package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/MikeSquared-Agency/dredd/internal/extractor"
)

// WriteOpts holds optional parameters for store write operations.
type WriteOpts struct {
	// Embedding is an optional vector embedding. If non-nil, it will be included in the INSERT.
	Embedding []float64
}

// WriteDecisionEpisode writes a full decision episode across the Decision Engine tables.
// Tables: decisions, decision_context, decision_options, decision_reasoning, decision_tags.
func (s *Store) WriteDecisionEpisode(ctx context.Context, ownerUUID uuid.UUID, sessionRef, source string, ep extractor.DecisionEpisode, opts ...WriteOpts) (uuid.UUID, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Resolve optional write opts.
	var opt WriteOpts
	if len(opts) > 0 {
		opt = opts[0]
	}

	// 1. Insert decision
	decisionID := uuid.New()
	if opt.Embedding != nil {
		_, err = tx.Exec(ctx, `
			INSERT INTO decisions (id, domain, category, severity, source, source_channel, decided_by, summary, session_ref, embedding, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())`,
			decisionID, ep.Domain, ep.Category, ep.Severity, source, source, ownerUUID.String(), ep.Summary, sessionRef, pgVector(opt.Embedding),
		)
	} else {
		_, err = tx.Exec(ctx, `
			INSERT INTO decisions (id, domain, category, severity, source, source_channel, decided_by, summary, session_ref, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())`,
			decisionID, ep.Domain, ep.Category, ep.Severity, source, source, ownerUUID.String(), ep.Summary, sessionRef,
		)
	}
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

	// 3. Insert decision_options (pro_signals/con_signals are jsonb in live schema)
	for _, opt := range ep.Options {
		proJSON, _ := json.Marshal(opt.ProSignals)
		conJSON, _ := json.Marshal(opt.ConSignals)
		_, err = tx.Exec(ctx, `
			INSERT INTO decision_options (id, decision_id, option_key, option_label, pro_signals, con_signals, was_chosen)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			uuid.New(), decisionID, opt.OptionKey, opt.OptionKey, proJSON, conJSON, opt.WasChosen,
		)
		if err != nil {
			return uuid.Nil, fmt.Errorf("insert option: %w", err)
		}
	}

	// 4. Insert decision_reasoning (factors/tradeoffs are jsonb in live schema)
	factorsJSON, _ := json.Marshal(ep.Reasoning.Factors)
	tradeoffsJSON, _ := json.Marshal(ep.Reasoning.Tradeoffs)
	_, err = tx.Exec(ctx, `
		INSERT INTO decision_reasoning (id, decision_id, factors, tradeoffs, reasoning_text)
		VALUES ($1, $2, $3, $4, $5)`,
		uuid.New(), decisionID, factorsJSON, tradeoffsJSON, ep.Reasoning.ReasoningText,
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert reasoning: %w", err)
	}

	// 5. Insert decision_tags (composite PK on decision_id+tag, no id column)
	for _, tag := range ep.Tags {
		_, err = tx.Exec(ctx, `
			INSERT INTO decision_tags (decision_id, tag, added_by)
			VALUES ($1, $2, $3)
			ON CONFLICT (decision_id, tag) DO NOTHING`,
			decisionID, tag, source,
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
