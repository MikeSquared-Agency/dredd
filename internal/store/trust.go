package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

type TrustRecord struct {
	ID               uuid.UUID
	AgentID          string
	Category         string
	Severity         string
	TrustScore       float64
	TotalDecisions   int
	CorrectDecisions int
	CriticalFailures int
}

// GetTrust fetches the trust record for an agent/category/severity combination.
func (s *Store) GetTrust(ctx context.Context, agentID, category, severity string) (*TrustRecord, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, agent_id, category, severity, trust_score, total_decisions, correct_decisions, critical_failures
		FROM agent_trust
		WHERE agent_id = $1 AND category = $2 AND severity = $3`,
		agentID, category, severity,
	)

	var t TrustRecord
	err := row.Scan(&t.ID, &t.AgentID, &t.Category, &t.Severity, &t.TrustScore, &t.TotalDecisions, &t.CorrectDecisions, &t.CriticalFailures)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// UpsertTrust creates or updates a trust record for an agent/category/severity.
func (s *Store) UpsertTrust(ctx context.Context, agentID, category, severity string, score float64, total, correct, failures int) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO agent_trust (id, agent_id, category, severity, trust_score, total_decisions, correct_decisions, critical_failures, last_signal_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now(), now())
		ON CONFLICT (agent_id, category, severity)
		DO UPDATE SET
			trust_score = $5,
			total_decisions = $6,
			correct_decisions = $7,
			critical_failures = $8,
			last_signal_at = now(),
			updated_at = now()`,
		uuid.New(), agentID, category, severity, score, total, correct, failures,
	)
	if err != nil {
		return fmt.Errorf("upsert trust: %w", err)
	}
	return nil
}
