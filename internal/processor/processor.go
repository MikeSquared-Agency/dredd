package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/MikeSquared-Agency/dredd/internal/extractor"
	"github.com/MikeSquared-Agency/dredd/internal/hermes"
	"github.com/MikeSquared-Agency/dredd/internal/slack"
	"github.com/MikeSquared-Agency/dredd/internal/store"
	"github.com/MikeSquared-Agency/dredd/internal/trust"
)

// Processor orchestrates Dredd's transcript processing pipeline.
type Processor struct {
	store     *store.Store
	extractor *extractor.Extractor
	hermes    *hermes.Client
	slack     *slack.Poster
	logger    *slog.Logger

	// pendingReviews maps Slack message TS → extraction metadata for reaction handling.
	pendingReviews map[string]*pendingReview
}

type pendingReview struct {
	SessionRef  string
	OwnerUUID   uuid.UUID
	DecisionIDs []uuid.UUID
	PatternIDs  []uuid.UUID
	Decisions   []extractor.DecisionEpisode
	Patterns    []extractor.ReasoningPattern
}

func New(s *store.Store, ext *extractor.Extractor, h *hermes.Client, sl *slack.Poster, logger *slog.Logger) *Processor {
	return &Processor{
		store:          s,
		extractor:      ext,
		hermes:         h,
		slack:          sl,
		logger:         logger,
		pendingReviews: make(map[string]*pendingReview),
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

	// Fetch transcript content from the event payload.
	// The transcript text is expected in the event or fetched from Chronicle.
	transcript, err := p.fetchTranscript(ctx, evt.SessionID)
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

	// Persist extractions.
	decisionIDs, patternIDs, err := p.persist(ctx, result)
	if err != nil {
		p.logger.Error("persistence failed", "session_ref", evt.SessionRef, "error", err)
		return
	}

	// Post review summary to Slack.
	if p.slack != nil {
		ts, err := p.slack.PostReviewSummary(ctx, result, evt.Title, evt.Surface, evt.Duration)
		if err != nil {
			p.logger.Error("slack post failed", "error", err)
		} else {
			p.pendingReviews[ts] = &pendingReview{
				SessionRef:  evt.SessionRef,
				OwnerUUID:   ownerUUID,
				DecisionIDs: decisionIDs,
				PatternIDs:  patternIDs,
				Decisions:   result.Decisions,
				Patterns:    result.Patterns,
			}
		}
	}

	p.logger.Info("transcript processed",
		"session_ref", evt.SessionRef,
		"decisions", len(decisionIDs),
		"patterns", len(patternIDs),
	)
}

// HandleReaction processes Slack reaction feedback from slack-forwarder via NATS.
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

	review, ok := p.pendingReviews[evt.MessageTS]
	if !ok {
		return // not a message we're tracking
	}

	p.logger.Info("processing review reaction",
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

		// Emit trust/feedback signals for confirmed/rejected decisions.
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

	// Emit pattern confirmation events.
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

	// On rejection, ask for correction in thread.
	if verdict == slack.VerdictRejected && p.slack != nil {
		if err := p.slack.PostThread(ctx, evt.MessageTS, "What did I get wrong? Your correction is the highest-value training signal."); err != nil {
			p.logger.Error("failed to post correction thread", "error", err)
		}
	}

	// Clean up.
	delete(p.pendingReviews, evt.MessageTS)
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
			score := trust.UpdateScore(0.0, dec.Severity, correct)
			total, correctCount := 1, 0
			if correct {
				correctCount = 1
			}
			if err := p.store.UpsertTrust(ctx, dec.AgentID, dec.Category, dec.Severity, score, total, correctCount, 0); err != nil {
				p.logger.Error("failed to create trust record", "error", err)
			}
		} else {
			newScore := trust.UpdateScore(rec.TrustScore, dec.Severity, correct)
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

func (p *Processor) fetchTranscript(ctx context.Context, sessionID string) (string, error) {
	// TODO: Fetch from Chronicle API when transcript endpoints are available.
	// For now, the transcript content should be included in the NATS event payload
	// or fetched from a known storage location.
	return "", fmt.Errorf("transcript fetch not yet implemented for session %s — Chronicle transcript API needed", sessionID)
}

func outcomeStr(correct bool) string {
	if correct {
		return "correct"
	}
	return "incorrect"
}
