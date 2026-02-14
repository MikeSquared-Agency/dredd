package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/MikeSquared-Agency/dredd/internal/extractor"
)

// WriteDecisionEpisode writes a full decision episode across the Decision Engine tables.
// Tables: decisions, decision_context, decision_options, decision_reasoning, decision_tags.
func (s *Store) WriteDecisionEpisode(ctx context.Context, ownerUUID uuid.UUID, sessionRef string, ep extractor.DecisionEpisode) (uuid.UUID, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Insert decision
	decisionID := uuid.New()
	_, err = tx.Exec(ctx, `
		INSERT INTO decisions (id, domain, category, severity, source, decided_by, summary, session_ref, created_at)
		VALUES ($1, $2, $3, $4, 'dredd', $5, $6, $7, now())`,
		decisionID, ep.Domain, ep.Category, ep.Severity, ownerUUID, ep.Summary, sessionRef,
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert decision: %w", err)
	}

	// 2. Insert decision_context
	_, err = tx.Exec(ctx, `
		INSERT INTO decision_context (id, decision_id, situation_text)
		VALUES ($1, $2, $3)`,
		uuid.New(), decisionID, ep.SituationText,
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert context: %w", err)
	}

	// 3. Insert decision_options
	for _, opt := range ep.Options {
		_, err = tx.Exec(ctx, `
			INSERT INTO decision_options (id, decision_id, option_key, pro_signals, con_signals, was_chosen)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			uuid.New(), decisionID, opt.OptionKey, opt.ProSignals, opt.ConSignals, opt.WasChosen,
		)
		if err != nil {
			return uuid.Nil, fmt.Errorf("insert option: %w", err)
		}
	}

	// 4. Insert decision_reasoning
	_, err = tx.Exec(ctx, `
		INSERT INTO decision_reasoning (id, decision_id, factors, tradeoffs, reasoning_text)
		VALUES ($1, $2, $3, $4, $5)`,
		uuid.New(), decisionID, ep.Reasoning.Factors, ep.Reasoning.Tradeoffs, ep.Reasoning.ReasoningText,
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert reasoning: %w", err)
	}

	// 5. Insert decision_tags
	for _, tag := range ep.Tags {
		_, err = tx.Exec(ctx, `
			INSERT INTO decision_tags (id, decision_id, tag)
			VALUES ($1, $2, $3)`,
			uuid.New(), decisionID, tag,
		)
		if err != nil {
			return uuid.Nil, fmt.Errorf("insert tag: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("commit: %w", err)
	}

	return decisionID, nil
}

// UpdateDecisionReviewStatus updates the review status of a decision.
func (s *Store) UpdateDecisionReviewStatus(ctx context.Context, decisionID uuid.UUID, status, note string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE decisions SET review_status = $1, review_note = $2, reviewed_at = now()
		WHERE id = $3`,
		status, note, decisionID,
	)
	return err
}
