package dedup

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DuplicatePair represents two potentially duplicate records.
type DuplicatePair struct {
	ID1        uuid.UUID
	ID2        uuid.UUID
	Similarity float64
}

// Scanner finds duplicate pairs using pgvector cosine similarity.
type Scanner struct {
	pool *pgxpool.Pool
}

// NewScanner creates a new scanner instance.
func NewScanner(pool *pgxpool.Pool) *Scanner {
	return &Scanner{pool: pool}
}

// FindReasoningPatternDuplicates finds duplicate reasoning patterns above the threshold.
func (s *Scanner) FindReasoningPatternDuplicates(ctx context.Context, threshold float64) ([]DuplicatePair, error) {
	query := `
		SELECT a.id, b.id, 1 - (a.arc_embedding <=> b.arc_embedding) AS similarity
		FROM reasoning_patterns a, reasoning_patterns b
		WHERE a.id < b.id
		  AND a.arc_embedding IS NOT NULL AND b.arc_embedding IS NOT NULL
		  AND a.deduped_at IS NULL AND b.deduped_at IS NULL
		  AND 1 - (a.arc_embedding <=> b.arc_embedding) > $1
		ORDER BY similarity DESC`

	rows, err := s.pool.Query(ctx, query, threshold)
	if err != nil {
		return nil, fmt.Errorf("query reasoning pattern duplicates: %w", err)
	}
	defer rows.Close()

	var pairs []DuplicatePair
	for rows.Next() {
		var pair DuplicatePair
		if err := rows.Scan(&pair.ID1, &pair.ID2, &pair.Similarity); err != nil {
			return nil, fmt.Errorf("scan duplicate pair: %w", err)
		}
		pairs = append(pairs, pair)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return pairs, nil
}

// FindDecisionDuplicates finds duplicate decisions above the threshold.
func (s *Scanner) FindDecisionDuplicates(ctx context.Context, threshold float64) ([]DuplicatePair, error) {
	query := `
		SELECT a.id, b.id, 1 - (a.embedding <=> b.embedding) AS similarity
		FROM decisions a, decisions b
		WHERE a.id < b.id
		  AND a.embedding IS NOT NULL AND b.embedding IS NOT NULL
		  AND a.deduped_at IS NULL AND b.deduped_at IS NULL
		  AND 1 - (a.embedding <=> b.embedding) > $1
		ORDER BY similarity DESC`

	rows, err := s.pool.Query(ctx, query, threshold)
	if err != nil {
		return nil, fmt.Errorf("query decision duplicates: %w", err)
	}
	defer rows.Close()

	var pairs []DuplicatePair
	for rows.Next() {
		var pair DuplicatePair
		if err := rows.Scan(&pair.ID1, &pair.ID2, &pair.Similarity); err != nil {
			return nil, fmt.Errorf("scan duplicate pair: %w", err)
		}
		pairs = append(pairs, pair)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return pairs, nil
}
