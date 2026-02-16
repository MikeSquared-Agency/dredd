package backfill

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/MikeSquared-Agency/dredd/internal/extractor"
	"github.com/MikeSquared-Agency/dredd/internal/slack"
	"github.com/MikeSquared-Agency/dredd/internal/store"
)

// Config holds the backfill command configuration.
type Config struct {
	CCDir          string
	GatewayDir     string
	Since          time.Time
	Until          time.Time
	DryRun         bool
	BatchSize      int
	MinMessages    int
	OwnerUUID      uuid.UUID
	SingleFile     string // process a single file only
	Source         string // source label for persisted records (default: "backfill")
	SkipSubagents  bool   // skip conversations with no human messages (default: true)
	SlackToken     string // optional: Slack bot token for posting summaries
	SlackChannel   string // optional: Slack channel for summaries
}

// Runner orchestrates the backfill process.
type Runner struct {
	cfg       Config
	store     *store.Store
	extractor *extractor.Extractor
	slack     *slack.Poster
	logger    *slog.Logger
}

// NewRunner creates a backfill runner.
func NewRunner(cfg Config, s *store.Store, ext *extractor.Extractor, logger *slog.Logger) *Runner {
	r := &Runner{
		cfg:       cfg,
		store:     s,
		extractor: ext,
		logger:    logger,
	}

	// Set up optional Slack poster for daily summaries.
	if cfg.SlackToken != "" && cfg.SlackChannel != "" {
		r.slack = slack.NewPoster(cfg.SlackToken, cfg.SlackChannel, logger)
	}

	return r
}

// sourceLabel returns the source string to use for persisted records.
func (r *Runner) sourceLabel() string {
	if r.cfg.Source != "" {
		return r.cfg.Source
	}
	return "backfill"
}

// Run executes the backfill process.
func (r *Runner) Run(ctx context.Context) error {
	state, err := LoadState()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	// Discover files.
	ccFiles, gwFiles, err := r.discoverFiles()
	if err != nil {
		return fmt.Errorf("discover files: %w", err)
	}

	r.logger.Info("files discovered",
		"cc_files", len(ccFiles),
		"gateway_files", len(gwFiles),
	)

	// Parse all files to get conversations + fingerprints for dedup.
	type parsedFile struct {
		path   string
		source FileSource
		msgs   []ConversationMessage
		fp     fileFingerprint
	}

	var ccParsed, gwParsed []parsedFile

	// Parse CC files.
	for _, path := range ccFiles {
		if state.IsProcessed(path) {
			continue
		}
		msgs, err := ParseCCFile(path)
		if err != nil {
			r.logger.Warn("failed to parse CC file", "path", path, "error", err)
			state.AddError(fmt.Sprintf("parse %s: %v", path, err))
			continue
		}
		if len(msgs) < r.cfg.MinMessages {
			continue
		}
		if r.cfg.SkipSubagents && !r.hasHumanMessages(msgs) {
			continue
		}
		if !r.inDateRange(msgs) {
			continue
		}
		fp := BuildFingerprint(path, SourceCC, msgs)
		ccParsed = append(ccParsed, parsedFile{path: path, source: SourceCC, msgs: msgs, fp: fp})
	}

	// Parse Gateway files.
	for _, path := range gwFiles {
		if state.IsProcessed(path) {
			continue
		}
		msgs, err := ParseGatewayFile(path)
		if err != nil {
			r.logger.Warn("failed to parse gateway file", "path", path, "error", err)
			state.AddError(fmt.Sprintf("parse %s: %v", path, err))
			continue
		}
		if len(msgs) < r.cfg.MinMessages {
			continue
		}
		if r.cfg.SkipSubagents && !r.hasHumanMessages(msgs) {
			continue
		}
		if !r.inDateRange(msgs) {
			continue
		}
		fp := BuildFingerprint(path, SourceGateway, msgs)
		gwParsed = append(gwParsed, parsedFile{path: path, source: SourceGateway, msgs: msgs, fp: fp})
	}

	// Deduplicate: find gateway files that overlap with CC files.
	var ccFPs, gwFPs []fileFingerprint
	for _, p := range ccParsed {
		ccFPs = append(ccFPs, p.fp)
	}
	for _, p := range gwParsed {
		gwFPs = append(gwFPs, p.fp)
	}
	duplicates := FindDuplicates(ccFPs, gwFPs)

	// Build final file list: CC first, then non-duplicate gateway.
	var allFiles []parsedFile
	allFiles = append(allFiles, ccParsed...)
	for _, gw := range gwParsed {
		if duplicates[gw.path] {
			r.logger.Info("skipping duplicate gateway file", "path", gw.path)
			continue
		}
		allFiles = append(allFiles, gw)
	}

	state.FilesRemaining = len(allFiles)
	r.logger.Info("files to process",
		"total", len(allFiles),
		"cc", len(ccParsed),
		"gateway_unique", len(allFiles)-len(ccParsed),
		"gateway_skipped", len(duplicates),
	)

	// Process each file.
	totalDecisions := 0
	totalPatterns := 0
	totalChunks := 0
	chunksInBatch := 0

	// Summary accumulator for Slack daily summaries.
	var fileSummaries []FileSummary

	for _, pf := range allFiles {
		select {
		case <-ctx.Done():
			r.logger.Info("backfill interrupted, saving state")
			_ = state.Save()
			r.postBatchSummary(ctx, fileSummaries)
			return ctx.Err()
		default:
		}

		r.logger.Info("processing file", "path", pf.path, "messages", len(pf.msgs), "source", sourceStr(pf.source))

		// Track per-file summary.
		fs := FileSummary{
			Path:   pf.path,
			Source: sourceStr(pf.source),
		}
		if len(pf.msgs) > 0 && !pf.msgs[0].Timestamp.IsZero() {
			fs.Date = pf.msgs[0].Timestamp.Format("2006-01-02")
		}

		// Chunk the conversation.
		chunks := ChunkConversation(pf.msgs, pf.path, pf.source)
		if len(chunks) == 0 {
			state.MarkProcessed(pf.path)
			continue
		}

		for _, chunk := range chunks {
			select {
			case <-ctx.Done():
				_ = state.Save()
				return ctx.Err()
			default:
			}

			transcript := FormatTranscript(chunk)
			if len(strings.TrimSpace(transcript)) == 0 {
				continue
			}

			r.logger.Info("extracting chunk",
				"session_ref", chunk.SessionRef,
				"messages", len(chunk.Messages),
			)

			result, err := r.extractor.Extract(ctx, chunk.SessionRef, r.cfg.OwnerUUID, transcript)
			if err != nil {
				r.logger.Error("extraction failed", "session_ref", chunk.SessionRef, "error", err)
				state.AddError(fmt.Sprintf("extract %s: %v", chunk.SessionRef, err))
				fs.Errors++
				continue
			}

			decisions := len(result.Decisions)
			patterns := len(result.Patterns)

			if !r.cfg.DryRun {
				if err := r.persist(ctx, result); err != nil {
					r.logger.Error("persist failed", "session_ref", chunk.SessionRef, "error", err)
					state.AddError(fmt.Sprintf("persist %s: %v", chunk.SessionRef, err))
					fs.Errors++
					continue
				}
			}

			totalDecisions += decisions
			totalPatterns += patterns
			totalChunks++
			chunksInBatch++
			fs.Decisions += decisions
			fs.Patterns += patterns
			fs.Chunks++

			r.logger.Info("chunk processed",
				"session_ref", chunk.SessionRef,
				"decisions", decisions,
				"patterns", patterns,
				"dry_run", r.cfg.DryRun,
			)

			state.ChunksProcessed++
			state.DecisionsFound += decisions
			state.PatternsFound += patterns

			// Rate limiting: pause after batch-size chunks.
			if chunksInBatch >= r.cfg.BatchSize {
				r.logger.Info("batch complete, saving state and pausing",
					"chunks_in_batch", chunksInBatch,
					"total_chunks", totalChunks,
				)
				_ = state.Save()
				chunksInBatch = 0

				// Post batch summary to Slack.
				r.postBatchSummary(ctx, fileSummaries)
				fileSummaries = nil

				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(30 * time.Second):
				}
			}
		}

		fileSummaries = append(fileSummaries, fs)
		state.MarkProcessed(pf.path)
		state.FilesRemaining--
		_ = state.Save()
	}

	// Final save.
	_ = state.Save()

	// Post final summary.
	r.postBatchSummary(ctx, fileSummaries)

	r.logger.Info("backfill complete",
		"files_processed", len(allFiles),
		"chunks_processed", totalChunks,
		"decisions_found", totalDecisions,
		"patterns_found", totalPatterns,
		"dry_run", r.cfg.DryRun,
	)

	fmt.Printf("\n=== Backfill Summary ===\n")
	fmt.Printf("Files processed: %d\n", len(allFiles))
	fmt.Printf("Chunks processed: %d\n", totalChunks)
	fmt.Printf("Decisions found: %d\n", totalDecisions)
	fmt.Printf("Patterns found: %d\n", totalPatterns)
	fmt.Printf("Errors: %d\n", len(state.Errors))
	if r.cfg.DryRun {
		fmt.Printf("Mode: DRY RUN (no DB writes)\n")
	}
	fmt.Printf("State file: %s\n", expandHome(defaultStatePath))

	return nil
}

func (r *Runner) persist(ctx context.Context, result *extractor.ExtractionResult) error {
	src := r.sourceLabel()
	for _, d := range result.Decisions {
		if _, err := r.store.WriteDecisionEpisode(ctx, result.OwnerUUID, result.SessionRef, src, d); err != nil {
			return fmt.Errorf("write decision: %w", err)
		}
	}
	for _, p := range result.Patterns {
		if _, err := r.store.WriteReasoningPattern(ctx, result.OwnerUUID, result.SessionRef, p); err != nil {
			return fmt.Errorf("write pattern: %w", err)
		}
	}
	for _, s := range result.Styles {
		if err := r.store.WriteStyle(ctx, result.OwnerUUID, result.SessionRef, src, s); err != nil {
			return fmt.Errorf("write style: %w", err)
		}
	}
	return nil
}

// postBatchSummary posts a daily summary of backfill results to Slack, grouped by date.
// If Slack is not configured, it logs the summary instead.
func (r *Runner) postBatchSummary(ctx context.Context, summaries []FileSummary) {
	if len(summaries) == 0 {
		return
	}

	text := FormatDailySummary(summaries)

	if r.slack == nil {
		r.logger.Info("backfill batch summary (no Slack configured)",
			"summary", text,
		)
		return
	}

	// Post as a standalone message (not a thread reply).
	if err := r.slack.PostThread(ctx, "", text); err != nil {
		r.logger.Warn("failed to post batch summary to Slack, logging instead",
			"error", err,
			"summary", text,
		)
	}
}

// FormatDailySummary formats file summaries grouped by date.
func FormatDailySummary(summaries []FileSummary) string {
	// Group by date.
	byDate := make(map[string][]FileSummary)
	for _, s := range summaries {
		date := s.Date
		if date == "" {
			date = "unknown"
		}
		byDate[date] = append(byDate[date], s)
	}

	// Sort dates.
	dates := make([]string, 0, len(byDate))
	for d := range byDate {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	var sb strings.Builder
	sb.WriteString("*Backfill Batch Summary*\n")

	for _, date := range dates {
		files := byDate[date]
		totalDec, totalPat := 0, 0
		for _, f := range files {
			totalDec += f.Decisions
			totalPat += f.Patterns
		}
		fmt.Fprintf(&sb, "\n*%s* (%d files, %d decisions, %d patterns)\n", date, len(files), totalDec, totalPat)
		for _, f := range files {
			name := filepath.Base(f.Path)
			fmt.Fprintf(&sb, "  - %s [%s]: %d dec, %d pat", name, f.Source, f.Decisions, f.Patterns)
			if f.Errors > 0 {
				fmt.Fprintf(&sb, " (%d errors)", f.Errors)
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func (r *Runner) discoverFiles() (ccFiles, gwFiles []string, err error) {
	if r.cfg.SingleFile != "" {
		path := expandHome(r.cfg.SingleFile)
		if _, err := os.Stat(path); err != nil {
			return nil, nil, fmt.Errorf("single file not found: %s", path)
		}
		// Determine source type from path.
		if strings.Contains(path, ".openclaw") {
			return nil, []string{path}, nil
		}
		return []string{path}, nil, nil
	}

	ccDir := expandHome(r.cfg.CCDir)
	gwDir := expandHome(r.cfg.GatewayDir)

	// Discover CC JSONL files.
	if info, err := os.Stat(ccDir); err == nil && info.IsDir() {
		err = filepath.Walk(ccDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // skip errors
			}
			if !info.IsDir() && strings.HasSuffix(info.Name(), ".jsonl") {
				ccFiles = append(ccFiles, path)
			}
			return nil
		})
		if err != nil {
			r.logger.Warn("error walking CC dir", "dir", ccDir, "error", err)
		}
	}

	// Discover Gateway JSONL files.
	if info, err := os.Stat(gwDir); err == nil && info.IsDir() {
		err = filepath.Walk(gwDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if !info.IsDir() && strings.HasSuffix(info.Name(), ".jsonl") {
				gwFiles = append(gwFiles, path)
			}
			return nil
		})
		if err != nil {
			r.logger.Warn("error walking gateway dir", "dir", gwDir, "error", err)
		}
	}

	return ccFiles, gwFiles, nil
}

// hasHumanMessages checks that the conversation has at least one non-cron user message.
func (r *Runner) hasHumanMessages(msgs []ConversationMessage) bool {
	for _, m := range msgs {
		if m.Role == "user" && !strings.HasPrefix(m.Text, "[cron:") {
			return true
		}
	}
	return false
}

// inDateRange checks if any message falls within the configured since/until range.
func (r *Runner) inDateRange(msgs []ConversationMessage) bool {
	if r.cfg.Since.IsZero() && r.cfg.Until.IsZero() {
		return true
	}

	for _, m := range msgs {
		if m.Timestamp.IsZero() {
			continue
		}
		if !r.cfg.Since.IsZero() && m.Timestamp.Before(r.cfg.Since) {
			continue
		}
		if !r.cfg.Until.IsZero() && m.Timestamp.After(r.cfg.Until) {
			continue
		}
		return true
	}
	return false
}

func sourceStr(s FileSource) string {
	if s == SourceCC {
		return "cc"
	}
	return "gateway"
}
