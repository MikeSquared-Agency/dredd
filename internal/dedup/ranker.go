package dedup

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReasoningPatternRecord represents a reasoning pattern for ranking.
type ReasoningPatternRecord struct {
	ID               uuid.UUID
	ReviewStatus     string
	DreddConfidence  float64
	CreatedAt        time.Time
}

// DecisionRecord represents a decision for ranking.
type DecisionRecord struct {
	ID           uuid.UUID
	ReviewStatus string
	Severity     string
	CreatedAt    time.Time
}

// Ranker picks survivors from clusters of duplicate records.
type Ranker struct {
	pool *pgxpool.Pool
}

// NewRanker creates a new ranker instance.
func NewRanker(pool *pgxpool.Pool) *Ranker {
	return &Ranker{pool: pool}
}

// RankReasoningPatterns picks the best record from a cluster of reasoning pattern IDs.
func (r *Ranker) RankReasoningPatterns(ctx context.Context, ids []uuid.UUID) (uuid.UUID, error) {
	if len(ids) == 0 {
		return uuid.Nil, fmt.Errorf("empty cluster")
	}
	if len(ids) == 1 {
		return ids[0], nil
	}

	// Fetch records for ranking
	query := `
		SELECT id, review_status, dredd_confidence, created_at
		FROM reasoning_patterns
		WHERE id = ANY($1)`

	rows, err := r.pool.Query(ctx, query, ids)
	if err != nil {
		return uuid.Nil, fmt.Errorf("fetch reasoning patterns: %w", err)
	}
	defer rows.Close()

	var records []ReasoningPatternRecord
	for rows.Next() {
		var record ReasoningPatternRecord
		if err := rows.Scan(&record.ID, &record.ReviewStatus, &record.DreddConfidence, &record.CreatedAt); err != nil {
			return uuid.Nil, fmt.Errorf("scan reasoning pattern: %w", err)
		}
		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		return uuid.Nil, fmt.Errorf("rows error: %w", err)
	}

	if len(records) == 0 {
		return uuid.Nil, fmt.Errorf("no records found")
	}

	// Rank records
	best := records[0]
	for _, record := range records[1:] {
		if isReasoningPatternBetter(record, best) {
			best = record
		}
	}

	return best.ID, nil
}

// RankDecisions picks the best record from a cluster of decision IDs.
func (r *Ranker) RankDecisions(ctx context.Context, ids []uuid.UUID) (uuid.UUID, error) {
	if len(ids) == 0 {
		return uuid.Nil, fmt.Errorf("empty cluster")
	}
	if len(ids) == 1 {
		return ids[0], nil
	}

	// Fetch records for ranking
	query := `
		SELECT id, review_status, severity, created_at
		FROM decisions
		WHERE id = ANY($1)`

	rows, err := r.pool.Query(ctx, query, ids)
	if err != nil {
		return uuid.Nil, fmt.Errorf("fetch decisions: %w", err)
	}
	defer rows.Close()

	var records []DecisionRecord
	for rows.Next() {
		var record DecisionRecord
		if err := rows.Scan(&record.ID, &record.ReviewStatus, &record.Severity, &record.CreatedAt); err != nil {
			return uuid.Nil, fmt.Errorf("scan decision: %w", err)
		}
		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		return uuid.Nil, fmt.Errorf("rows error: %w", err)
	}

	if len(records) == 0 {
		return uuid.Nil, fmt.Errorf("no records found")
	}

	// Rank records
	best := records[0]
	for _, record := range records[1:] {
		if isDecisionBetter(record, best) {
			best = record
		}
	}

	return best.ID, nil
}

// isReasoningPatternBetter determines if record a is better than record b for reasoning patterns.
func isReasoningPatternBetter(a, b ReasoningPatternRecord) bool {
	// 1. Review status: confirmed > pending > rejected
	aStatus := reviewStatusPriority(a.ReviewStatus)
	bStatus := reviewStatusPriority(b.ReviewStatus)
	if aStatus != bStatus {
		return aStatus > bStatus
	}

	// 2. Higher dredd_confidence breaks ties
	if a.DreddConfidence != b.DreddConfidence {
		return a.DreddConfidence > b.DreddConfidence
	}

	// 3. More recent created_at breaks further ties
	return a.CreatedAt.After(b.CreatedAt)
}

// isDecisionBetter determines if record a is better than record b for decisions.
func isDecisionBetter(a, b DecisionRecord) bool {
	// 1. Review status: confirmed > pending > rejected
	aStatus := reviewStatusPriority(a.ReviewStatus)
	bStatus := reviewStatusPriority(b.ReviewStatus)
	if aStatus != bStatus {
		return aStatus > bStatus
	}

	// 2. Higher severity wins ties (critical > high > medium > low)
	aSeverity := severityPriority(a.Severity)
	bSeverity := severityPriority(b.Severity)
	if aSeverity != bSeverity {
		return aSeverity > bSeverity
	}

	// 3. More recent created_at breaks further ties
	return a.CreatedAt.After(b.CreatedAt)
}

// reviewStatusPriority returns numeric priority for review status.
func reviewStatusPriority(status string) int {
	switch status {
	case "confirmed":
		return 3
	case "pending":
		return 2
	case "rejected":
		return 1
	default:
		return 0
	}
}

// severityPriority returns numeric priority for severity.
func severityPriority(severity string) int {
	switch severity {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}
