package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/MikeSquared-Agency/dredd/internal/extractor"
	"github.com/MikeSquared-Agency/dredd/internal/hermes"
	"github.com/MikeSquared-Agency/dredd/internal/slack"
	"github.com/MikeSquared-Agency/dredd/internal/store"
	"github.com/MikeSquared-Agency/dredd/internal/trust"
)

// Processor orchestrates Dredd's transcript processing pipeline.
type Processor struct {
	store        *store.Store
	extractor    *extractor.Extractor
	hermes       *hermes.Client
	slack        *slack.Poster
	logger       *slog.Logger
	chronicleURL string

	mu             sync.Mutex
	pendingReviews map[string]*pendingReview // keyed by header TS (for rejection thread replies)
	pendingItems   map[string]*pendingItem   // keyed by per-item TS (for per-item reactions)
}

// pendingItem maps a single Slack thread-reply TS to its stored ID and extraction data.
type pendingItem struct {
	SessionRef string
	OwnerUUID  uuid.UUID
	Kind       string    // "decision" or "pattern"
	Idx        int       // index into the original extraction result
	StoredID   uuid.UUID // the DB id (decision or pattern)
	Decision   *extractor.DecisionEpisode
	Pattern    *extractor.ReasoningPattern
}

// pendingReview tracks the header TS and all item-level TSes for a review thread.
type pendingReview struct {
	SessionRef  string
	OwnerUUID   uuid.UUID
	HeaderTS    string
	DecisionIDs []uuid.UUID
	PatternIDs  []uuid.UUID
	Decisions   []extractor.DecisionEpisode
	Patterns    []extractor.ReasoningPattern
}

func New(s *store.Store, ext *extractor.Extractor, h *hermes.Client, sl *slack.Poster, chronicleURL string, logger *slog.Logger) *Processor {
	return &Processor{
		store:          s,
		extractor:      ext,
		hermes:         h,
		slack:          sl,
		logger:         logger,
		chronicleURL:   chronicleURL,
		pendingReviews: make(map[string]*pendingReview),
		pendingItems:   make(map[string]*pendingItem),
	}
}

// HandleTranscriptStored is the NATS handler for swarm.chronicle.transcript.stored.
func (p *Processor) HandleTranscriptStored(subject string, data []byte) {
	ctx := context.Background()

	var evt extractor.TranscriptEvent
	if err := json.Unmarshal(data, &evt); err != nil {
		p.logger.Error("failed to parse transcript event", "error", err)
		return
	}

	ownerUUID, err := uuid.Parse(evt.OwnerUUID)
	if err != nil {
		p.logger.Error("invalid owner uuid", "owner_uuid", evt.OwnerUUID, "error", err)
		return
	}

	p.logger.Info("processing transcript",
		"session_id", evt.SessionID,
		"session_ref", evt.SessionRef,
		"owner", evt.OwnerUUID,
	)

	// Fetch transcript content from the event payload or Chronicle.
	transcript, err := p.fetchTranscript(ctx, evt)
	if err != nil {
		p.logger.Error("failed to fetch transcript", "session_id", evt.SessionID, "error", err)
		return
	}

	// Extract decisions and patterns.
	result, err := p.extractor.Extract(ctx, evt.SessionRef, ownerUUID, transcript)
	if err != nil {
		p.logger.Error("extraction failed", "session_ref", evt.SessionRef, "error", err)
		return
	}

	// Propagate model tracking fields from the transcript event to each decision.
	for i := range result.Decisions {
		result.Decisions[i].ModelID = evt.ModelID
		result.Decisions[i].ModelTier = evt.ModelTier
	}

	// Persist extractions.
	decisionIDs, patternIDs, err := p.persist(ctx, result)
	if err != nil {
		p.logger.Error("persistence failed", "session_ref", evt.SessionRef, "error", err)
		return
	}

	// Post per-item review thread to Slack.
	if p.slack != nil {
		thread, err := p.slack.PostReviewThread(ctx, result, evt.Title, evt.Surface, evt.Duration)
		if err != nil {
			p.logger.Error("slack post failed", "error", err)
		} else {
			p.mu.Lock()
			// Store header-level review for rejection thread replies.
			p.pendingReviews[thread.HeaderTS] = &pendingReview{
				SessionRef:  evt.SessionRef,
				OwnerUUID:   ownerUUID,
				HeaderTS:    thread.HeaderTS,
				DecisionIDs: decisionIDs,
				PatternIDs:  patternIDs,
				Decisions:   result.Decisions,
				Patterns:    result.Patterns,
			}
			// Store per-item TS mappings for per-item reactions.
			for _, item := range thread.Items {
				pi := &pendingItem{
					SessionRef: evt.SessionRef,
					OwnerUUID:  ownerUUID,
					Kind:       item.Kind,
					Idx:        item.Idx,
				}
				switch item.Kind {
				case "decision":
					if item.Idx < len(decisionIDs) {
						pi.StoredID = decisionIDs[item.Idx]
					}
					if item.Idx < len(result.Decisions) {
						dec := result.Decisions[item.Idx]
						pi.Decision = &dec
					}
				case "pattern":
					if item.Idx < len(patternIDs) {
						pi.StoredID = patternIDs[item.Idx]
					}
					if item.Idx < len(result.Patterns) {
						pat := result.Patterns[item.Idx]
						pi.Pattern = &pat
					}
				}
				p.pendingItems[item.TS] = pi
			}
			p.mu.Unlock()
		}
	}

	p.logger.Info("transcript processed",
		"session_ref", evt.SessionRef,
		"decisions", len(decisionIDs),
		"patterns", len(patternIDs),
	)
}

// HandleReaction processes Slack reaction feedback from slack-forwarder via NATS.
// Reactions on per-item thread replies are mapped to the specific decision or pattern.
func (p *Processor) HandleReaction(subject string, data []byte) {
	ctx := context.Background()

	evt, err := slack.ParseReactionEvent(data, p.logger)
	if err != nil {
		p.logger.Error("failed to parse reaction", "error", err)
		return
	}

	verdict := slack.ParseReaction(evt.Reaction)
	if verdict == slack.VerdictUnknown {
		return // not a review reaction
	}

	// Try per-item match first.
	p.mu.Lock()
	item, itemOK := p.pendingItems[evt.MessageTS]
	if itemOK {
		delete(p.pendingItems, evt.MessageTS)
	}
	p.mu.Unlock()

	if itemOK {
		p.handleItemReaction(ctx, item, verdict, evt.MessageTS)
		return
	}

	// Fall back to header-level reaction (applies to all items in the review).
	p.mu.Lock()
	review, ok := p.pendingReviews[evt.MessageTS]
	if !ok {
		p.mu.Unlock()
		return // not a message we're tracking
	}
	delete(p.pendingReviews, evt.MessageTS)
	p.mu.Unlock()

	p.logger.Info("processing header-level review reaction",
		"reaction", evt.Reaction,
		"verdict", string(verdict),
		"session_ref", review.SessionRef,
	)

	status := string(verdict)

	// Update all decisions in this review.
	for i, id := range review.DecisionIDs {
		if err := p.store.UpdateDecisionReviewStatus(ctx, id, status, ""); err != nil {
			p.logger.Error("failed to update decision review", "decision_id", id, "error", err)
		}
		if verdict == slack.VerdictConfirmed || verdict == slack.VerdictRejected {
			p.emitDecisionSignals(ctx, review, i, verdict)
		}
	}

	// Update all patterns in this review.
	for _, id := range review.PatternIDs {
		if err := p.store.UpdatePatternReviewStatus(ctx, id, status, ""); err != nil {
			p.logger.Error("failed to update pattern review", "pattern_id", id, "error", err)
		}
	}

	if verdict == slack.VerdictConfirmed {
		for _, pat := range review.Patterns {
			if err := p.hermes.Publish("swarm.dredd.pattern.confirmed", map[string]any{
				"pattern_type": pat.PatternType,
				"summary":      pat.Summary,
				"tags":         pat.Tags,
				"owner_uuid":   review.OwnerUUID.String(),
				"session_ref":  review.SessionRef,
			}); err != nil {
				p.logger.Error("failed to publish pattern confirmed", "error", err)
			}
		}
	}

	if verdict == slack.VerdictRejected && p.slack != nil {
		if err := p.slack.PostThread(ctx, evt.MessageTS, "What did I get wrong? Your correction is the highest-value training signal."); err != nil {
			p.logger.Error("failed to post correction thread", "error", err)
		}
	}
}

// handleItemReaction processes a reaction on a single per-item thread reply.
func (p *Processor) handleItemReaction(ctx context.Context, item *pendingItem, verdict slack.ReviewVerdict, messageTS string) {
	p.logger.Info("processing per-item review reaction",
		"kind", item.Kind,
		"verdict", string(verdict),
		"stored_id", item.StoredID,
		"session_ref", item.SessionRef,
	)

	status := string(verdict)

	switch item.Kind {
	case "decision":
		if err := p.store.UpdateDecisionReviewStatus(ctx, item.StoredID, status, ""); err != nil {
			p.logger.Error("failed to update decision review", "decision_id", item.StoredID, "error", err)
		}
		if (verdict == slack.VerdictConfirmed || verdict == slack.VerdictRejected) && item.Decision != nil {
			correct := verdict == slack.VerdictConfirmed
			dec := item.Decision

			if dec.AgentID != "" {
				if err := p.hermes.Publish("swarm.dredd.trust.signal", map[string]any{
					"agent_id":    dec.AgentID,
					"category":    dec.Category,
					"outcome":     outcomeStr(correct),
					"severity":    dec.Severity,
					"session_ref": item.SessionRef,
				}); err != nil {
					p.logger.Error("failed to publish trust signal", "error", err)
				}

				rec, err := p.store.GetTrust(ctx, dec.AgentID, dec.Category, dec.Severity)
				if err != nil {
					score := trust.UpdateScoreWithSentiment(0.0, dec.Severity, correct, "")
					total, correctCount := 1, 0
					if correct {
						correctCount = 1
					}
					if err := p.store.UpsertTrust(ctx, dec.AgentID, dec.Category, dec.Severity, score, total, correctCount, 0); err != nil {
						p.logger.Error("failed to create trust record", "error", err)
					}
				} else {
					newScore := trust.UpdateScoreWithSentiment(rec.TrustScore, dec.Severity, correct, "")
					total := rec.TotalDecisions + 1
					correctCount := rec.CorrectDecisions
					if correct {
						correctCount++
					}
					if err := p.store.UpsertTrust(ctx, dec.AgentID, dec.Category, dec.Severity, newScore, total, correctCount, rec.CriticalFailures); err != nil {
						p.logger.Error("failed to update trust record", "error", err)
					}
				}
			}

			if dec.SignalType != "" {
				if err := p.hermes.Publish("swarm.dredd.assignment.signal", map[string]any{
					"signal_type": dec.SignalType,
					"agent_id":    dec.AgentID,
					"category":    dec.Category,
					"severity":    dec.Severity,
					"session_ref": item.SessionRef,
				}); err != nil {
					p.logger.Error("failed to publish assignment signal", "error", err)
				}
			}

			if !correct {
				if err := p.hermes.Publish("swarm.dredd.extraction.rejected", map[string]any{
					"session_ref": item.SessionRef,
					"decision":    dec.Summary,
					"category":    dec.Category,
				}); err != nil {
					p.logger.Error("failed to publish extraction rejected", "error", err)
				}
			}

			// Correction signal for prompt optimisation loop.
			correctionType := "confirmed"
			if !correct {
				correctionType = "rejected"
			}
			if p.hermes != nil {
				_ = p.hermes.Publish(hermes.SubjectCorrection, map[string]any{
					"session_ref":     item.SessionRef,
					"decision_id":     item.StoredID.String(),
					"agent_id":        dec.AgentID,
					"model_id":        dec.ModelID,
					"model_tier":      dec.ModelTier,
					"correction_type": correctionType,
					"category":        dec.Category,
					"severity":        dec.Severity,
				})
			}
		}

		if verdict == slack.VerdictRejected && p.slack != nil {
			if err := p.slack.PostThread(ctx, messageTS, "What did I get wrong? Your correction is the highest-value training signal."); err != nil {
				p.logger.Error("failed to post correction thread", "error", err)
			}
		}

	case "pattern":
		if err := p.store.UpdatePatternReviewStatus(ctx, item.StoredID, status, ""); err != nil {
			p.logger.Error("failed to update pattern review", "pattern_id", item.StoredID, "error", err)
		}

		if verdict == slack.VerdictConfirmed && item.Pattern != nil {
			if err := p.hermes.Publish("swarm.dredd.pattern.confirmed", map[string]any{
				"pattern_type": item.Pattern.PatternType,
				"summary":      item.Pattern.Summary,
				"tags":         item.Pattern.Tags,
				"owner_uuid":   item.OwnerUUID.String(),
				"session_ref":  item.SessionRef,
			}); err != nil {
				p.logger.Error("failed to publish pattern confirmed", "error", err)
			}
		}

		if verdict == slack.VerdictRejected && p.slack != nil {
			if err := p.slack.PostThread(ctx, messageTS, "What did I get wrong about this pattern?"); err != nil {
				p.logger.Error("failed to post correction thread", "error", err)
			}
		}
	}
}

func (p *Processor) emitDecisionSignals(ctx context.Context, review *pendingReview, idx int, verdict slack.ReviewVerdict) {
	if idx >= len(review.Decisions) {
		return
	}
	dec := review.Decisions[idx]
	correct := verdict == slack.VerdictConfirmed

	// Trust signal.
	if dec.AgentID != "" {
		if err := p.hermes.Publish("swarm.dredd.trust.signal", map[string]any{
			"agent_id":    dec.AgentID,
			"category":    dec.Category,
			"outcome":     outcomeStr(correct),
			"severity":    dec.Severity,
			"session_ref": review.SessionRef,
		}); err != nil {
			p.logger.Error("failed to publish trust signal", "error", err)
		}

		// Update trust score in DB.
		rec, err := p.store.GetTrust(ctx, dec.AgentID, dec.Category, dec.Severity)
		if err != nil {
			// First signal for this combination — create new record.
			// Sentiment defaults to empty (modifier=1.0) until sentiment detection is wired in.
			score := trust.UpdateScoreWithSentiment(0.0, dec.Severity, correct, "")
			total, correctCount := 1, 0
			if correct {
				correctCount = 1
			}
			if err := p.store.UpsertTrust(ctx, dec.AgentID, dec.Category, dec.Severity, score, total, correctCount, 0); err != nil {
				p.logger.Error("failed to create trust record", "error", err)
			}
		} else {
			newScore := trust.UpdateScoreWithSentiment(rec.TrustScore, dec.Severity, correct, "")
			total := rec.TotalDecisions + 1
			correctCount := rec.CorrectDecisions
			if correct {
				correctCount++
			}
			if err := p.store.UpsertTrust(ctx, dec.AgentID, dec.Category, dec.Severity, newScore, total, correctCount, rec.CriticalFailures); err != nil {
				p.logger.Error("failed to update trust record", "error", err)
			}
		}
	}

	// Assignment signal (reassignment, budget correction, etc.).
	if dec.SignalType != "" {
		if err := p.hermes.Publish("swarm.dredd.assignment.signal", map[string]any{
			"signal_type": dec.SignalType,
			"agent_id":    dec.AgentID,
			"category":    dec.Category,
			"severity":    dec.Severity,
			"session_ref": review.SessionRef,
		}); err != nil {
			p.logger.Error("failed to publish assignment signal", "error", err)
		}
	}

	// Self-training on rejections.
	if !correct {
		if err := p.hermes.Publish("swarm.dredd.extraction.rejected", map[string]any{
			"session_ref": review.SessionRef,
			"decision":    dec.Summary,
			"category":    dec.Category,
		}); err != nil {
			p.logger.Error("failed to publish extraction rejected", "error", err)
		}
	}

	// Correction signal for prompt optimisation loop.
	correctionType := "confirmed"
	if !correct {
		correctionType = "rejected"
	}
	if p.hermes != nil {
		_ = p.hermes.Publish(hermes.SubjectCorrection, map[string]any{
			"session_ref":     review.SessionRef,
			"decision_id":     review.DecisionIDs[idx].String(),
			"agent_id":        dec.AgentID,
			"model_id":        dec.ModelID,
			"model_tier":      dec.ModelTier,
			"correction_type": correctionType,
			"category":        dec.Category,
			"severity":        dec.Severity,
		})
	}
}

func (p *Processor) persist(ctx context.Context, result *extractor.ExtractionResult) ([]uuid.UUID, []uuid.UUID, error) {
	var decisionIDs []uuid.UUID
	for _, d := range result.Decisions {
		id, err := p.store.WriteDecisionEpisode(ctx, result.OwnerUUID, result.SessionRef, "dredd", d)
		if err != nil {
			return nil, nil, fmt.Errorf("write decision: %w", err)
		}
		decisionIDs = append(decisionIDs, id)
	}

	var patternIDs []uuid.UUID
	for _, pat := range result.Patterns {
		id, err := p.store.WriteReasoningPattern(ctx, result.OwnerUUID, result.SessionRef, pat)
		if err != nil {
			return nil, nil, fmt.Errorf("write pattern: %w", err)
		}
		patternIDs = append(patternIDs, id)
	}

	return decisionIDs, patternIDs, nil
}

func (p *Processor) fetchTranscript(ctx context.Context, evt extractor.TranscriptEvent) (string, error) {
	// Prefer transcript embedded in the event payload.
	if evt.Transcript != "" {
		return evt.Transcript, nil
	}

	// Fall back to Chronicle HTTP API.
	if p.chronicleURL == "" {
		return "", fmt.Errorf("no transcript in event payload and CHRONICLE_URL not configured for session %s", evt.SessionID)
	}

	url := fmt.Sprintf("%s/api/v1/events?trace_id=%s", p.chronicleURL, evt.SessionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build chronicle request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("chronicle request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("chronicle returned %d for session %s", resp.StatusCode, evt.SessionID)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read chronicle response: %w", err)
	}

	// Parse events and reconstruct transcript text.
	var events []struct {
		Metadata json.RawMessage `json:"metadata"`
	}
	if err := json.Unmarshal(body, &events); err != nil {
		return "", fmt.Errorf("parse chronicle events: %w", err)
	}

	if len(events) == 0 {
		return "", fmt.Errorf("no events found in chronicle for session %s", evt.SessionID)
	}

	// Return the raw events JSON as transcript input for the extractor.
	return string(body), nil
}

func outcomeStr(correct bool) string {
	if correct {
		return "correct"
	}
	return "incorrect"
}
