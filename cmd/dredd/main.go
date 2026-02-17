package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/MikeSquared-Agency/dredd/internal/anthropic"
	"github.com/MikeSquared-Agency/dredd/internal/api"
	"github.com/MikeSquared-Agency/dredd/internal/backfill"
	"github.com/MikeSquared-Agency/dredd/internal/config"
	"github.com/MikeSquared-Agency/dredd/internal/extractor"
	"github.com/MikeSquared-Agency/dredd/internal/hermes"
	"github.com/MikeSquared-Agency/dredd/internal/processor"
	"github.com/MikeSquared-Agency/dredd/internal/slack"
	"github.com/MikeSquared-Agency/dredd/internal/store"
)

func main() {
	// Route subcommands: "dredd" or "dredd serve" → service, "dredd backfill" → backfill, "dredd dedup" → dedup.
	if len(os.Args) > 1 && os.Args[1] == "backfill" {
		runBackfill(os.Args[2:])
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "dedup" {
		runDedup(os.Args[2:])
		return
	}

	// Strip "serve" if provided, then run the service.
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		os.Args = append(os.Args[:1], os.Args[2:]...)
	}

	runServe()
}

func runDedup(args []string) {
	fs := flag.NewFlagSet("dedup", flag.ExitOnError)
	threshold := fs.Float64("threshold", 0.92, "Similarity threshold (0.0-1.0)")
	execute := fs.Bool("execute", false, "Execute deduplication (default is dry-run)")
	table := fs.String("table", "all", "Table to deduplicate: patterns, decisions, or all")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "parse flags: %v\n", err)
		os.Exit(1)
	}

	setupLogging("info")

	envCfg := config.Load()
	if envCfg.DatabaseURL == "" {
		slog.Error("DATABASE_URL is required")
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Database
	db, err := store.New(ctx, envCfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	slog.Info("dedup starting",
		"threshold", *threshold,
		"execute", *execute,
		"table", *table,
	)

	// Validate threshold
	if *threshold < 0.0 || *threshold > 1.0 {
		slog.Error("threshold must be between 0.0 and 1.0", "threshold", *threshold)
		os.Exit(1)
	}

	// Validate table
	if *table != "patterns" && *table != "decisions" && *table != "all" {
		slog.Error("table must be 'patterns', 'decisions', or 'all'", "table", *table)
		os.Exit(1)
	}

	logger := slog.Default()

	// Execute deduplication
	if *table == "patterns" || *table == "all" {
		result, err := db.DeduplicateReasoningPatterns(ctx, *threshold, *execute, logger)
		if err != nil {
			slog.Error("failed to deduplicate reasoning patterns", "error", err)
			os.Exit(1)
		}

		// Output result as JSON
		output, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			slog.Error("failed to marshal result", "error", err)
			os.Exit(1)
		}
		fmt.Println(string(output))
	}

	if *table == "decisions" || *table == "all" {
		result, err := db.DeduplicateDecisions(ctx, *threshold, *execute, logger)
		if err != nil {
			slog.Error("failed to deduplicate decisions", "error", err)
			os.Exit(1)
		}

		// Output result as JSON
		output, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			slog.Error("failed to marshal result", "error", err)
			os.Exit(1)
		}
		fmt.Println(string(output))
	}

	slog.Info("dedup completed")
}

func runBackfill(args []string) {
	fs := flag.NewFlagSet("backfill", flag.ExitOnError)
	ccDir := fs.String("cc-dir", "~/.claude/projects", "CC JSONL transcript directory")
	gatewayDir := fs.String("gateway-dir", "~/.openclaw/agents/main/sessions", "Gateway session directory")
	since := fs.String("since", "", "Only process files with messages after this date (YYYY-MM-DD)")
	until := fs.String("until", "", "Only process files with messages before this date (YYYY-MM-DD)")
	dryRun := fs.Bool("dry-run", false, "Parse and extract but don't write to DB")
	batchSize := fs.Int("batch-size", 10, "Number of chunks to process before pausing")
	minMessages := fs.Int("min-messages", 5, "Minimum messages per conversation to process")
	owner := fs.String("owner", "9f6ed519-5763-4e30-9c2f-5580e0c57703", "Owner UUID for extracted records")
	singleFile := fs.String("file", "", "Process a single file instead of directories")
	source := fs.String("source", "backfill", "Source label for persisted records")
	skipSubagents := fs.Bool("skip-subagents", true, "Skip conversations with no human messages")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "parse flags: %v\n", err)
		os.Exit(1)
	}

	setupLogging("info")

	ownerUUID, err := uuid.Parse(*owner)
	if err != nil {
		slog.Error("invalid owner UUID", "owner", *owner, "error", err)
		os.Exit(1)
	}

	envCfg := config.Load()

	cfg := backfill.Config{
		CCDir:         *ccDir,
		GatewayDir:    *gatewayDir,
		DryRun:        *dryRun,
		BatchSize:     *batchSize,
		MinMessages:   *minMessages,
		OwnerUUID:     ownerUUID,
		SingleFile:    *singleFile,
		Source:        *source,
		SkipSubagents: *skipSubagents,
		SlackToken:    envCfg.SlackBotToken,
		SlackChannel:  envCfg.SlackChannel,
	}

	if *since != "" {
		t, err := time.Parse("2006-01-02", *since)
		if err != nil {
			slog.Error("invalid --since date", "since", *since, "error", err)
			os.Exit(1)
		}
		cfg.Since = t
	}
	if *until != "" {
		t, err := time.Parse("2006-01-02", *until)
		if err != nil {
			slog.Error("invalid --until date", "until", *until, "error", err)
			os.Exit(1)
		}
		cfg.Until = t.Add(24*time.Hour - time.Nanosecond) // end of day
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle SIGINT for graceful shutdown during backfill.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		slog.Info("interrupt received, finishing current batch...")
		cancel()
	}()

	// Anthropic client (always needed for extraction).
	if envCfg.AnthropicAPIKey == "" {
		slog.Error("ANTHROPIC_API_KEY is required")
		os.Exit(1)
	}
	llm := anthropic.NewClient(envCfg.AnthropicAPIKey, envCfg.AnthropicModel)
	ext := extractor.New(llm, slog.Default())

	// Database (not needed for dry-run, but connect anyway for simplicity).
	var db *store.Store
	if !cfg.DryRun {
		if envCfg.DatabaseURL == "" {
			slog.Error("DATABASE_URL is required (use --dry-run to skip DB)")
			os.Exit(1)
		}
		db, err = store.New(ctx, envCfg.DatabaseURL)
		if err != nil {
			slog.Error("failed to connect to database", "error", err)
			os.Exit(1)
		}
		defer db.Close()
	}

	slog.Info("backfill starting",
		"cc_dir", cfg.CCDir,
		"gateway_dir", cfg.GatewayDir,
		"dry_run", cfg.DryRun,
		"batch_size", cfg.BatchSize,
		"min_messages", cfg.MinMessages,
		"owner", cfg.OwnerUUID.String(),
		"source", cfg.Source,
		"skip_subagents", cfg.SkipSubagents,
	)

	runner := backfill.NewRunner(cfg, db, ext, slog.Default())
	if err := runner.Run(ctx); err != nil && err != context.Canceled {
		slog.Error("backfill failed", "error", err)
		os.Exit(1)
	}
}

func runServe() {
	cfg := config.Load()
	setupLogging(cfg.LogLevel)

	slog.Info("dredd starting", "port", cfg.Port)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Database
	if cfg.DatabaseURL == "" {
		slog.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	db, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	slog.Info("database connected")

	// Anthropic client
	if cfg.AnthropicAPIKey == "" {
		slog.Error("ANTHROPIC_API_KEY is required")
		os.Exit(1)
	}
	llm := anthropic.NewClient(cfg.AnthropicAPIKey, cfg.AnthropicModel)
	slog.Info("anthropic client ready", "model", cfg.AnthropicModel)

	// Extractor
	ext := extractor.New(llm, slog.Default())

	// NATS/Hermes
	hermesClient, err := hermes.NewClient(ctx, cfg.NatsURL, cfg.NatsToken, slog.Default())
	if err != nil {
		slog.Error("failed to connect to NATS", "error", err)
		os.Exit(1)
	}
	defer hermesClient.Close()
	slog.Info("NATS connected", "url", cfg.NatsURL)

	// Slack poster (optional — Dredd works without Slack, just no review loop)
	var slackPoster *slack.Poster
	if cfg.SlackBotToken != "" && cfg.SlackChannel != "" {
		slackPoster = slack.NewPoster(cfg.SlackBotToken, cfg.SlackChannel, slog.Default())
		slog.Info("slack poster ready", "channel", cfg.SlackChannel)
	} else {
		slog.Warn("slack not configured — running without review loop")
	}

	// Processor — the main pipeline
	proc := processor.New(db, ext, hermesClient, slackPoster, cfg.ChronicleURL, slog.Default())

	// Subscribe to transcript events
	if err := hermesClient.Subscribe("swarm.chronicle.transcript.stored", proc.HandleTranscriptStored); err != nil {
		slog.Error("failed to subscribe to transcript events", "error", err)
		os.Exit(1)
	}

	// Subscribe to Slack reactions for the review loop
	if err := hermesClient.Subscribe("swarm.slack.reaction", proc.HandleReaction); err != nil {
		slog.Error("failed to subscribe to slack reactions", "error", err)
		os.Exit(1)
	}

	// Subscribe to Slack interactions for gate decisions
	if err := hermesClient.Subscribe("swarm.slack.interaction", proc.HandleGateDecision); err != nil {
		slog.Error("failed to subscribe to gate decisions", "error", err)
	}

	// Subscribe to gate evidence for version attribution
	if err := hermesClient.Subscribe("swarm.dispatch.*.gate.evidence", proc.HandleGateEvidence); err != nil {
		slog.Error("failed to subscribe to gate evidence", "error", err)
	}

	// Subscribe to task picker decisions
	if err := hermesClient.Subscribe("swarm.slack.task.picked", proc.HandleTaskPicked); err != nil {
		slog.Error("failed to subscribe to task picked", "error", err)
	}
	if err := hermesClient.Subscribe("swarm.slack.task.regenerated", proc.HandleTaskRegenerate); err != nil {
		slog.Error("failed to subscribe to task regenerated", "error", err)
	}

	// HTTP API
	srv := api.NewServer(cfg.Port, cfg.APIToken, db)

	// Add refinement routes
	api.AddRefinementRoutes(srv.Router(), cfg.APIToken, db, hermesClient)
	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
		}
	}()

	// Announce registration
	if err := hermesClient.Publish("swarm.agent.dredd.registered", map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"port":      cfg.Port,
		"mode":      "shadow",
	}); err != nil {
		slog.Warn("failed to publish registration", "error", err)
	}

	slog.Info("dredd ready — shadow mode", "port", cfg.Port)

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	slog.Info("shutting down")
	cancel()

	// Give in-flight HTTP requests up to 5 seconds to complete.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}

	slog.Info("dredd stopped")
}

func setupLogging(level string) {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	slog.SetDefault(slog.New(handler))
}
