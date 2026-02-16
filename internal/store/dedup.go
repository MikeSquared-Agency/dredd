package store

import (
	"context"

	"github.com/MikeSquared-Agency/dredd/internal/dedup"
	"log/slog"
)

// GetDeduplicator returns a deduplicator instance using the store's connection pool.
func (s *Store) GetDeduplicator(logger *slog.Logger) *dedup.Deduplicator {
	return dedup.New(s.pool, logger)
}

// DeduplicateReasoningPatterns performs deduplication on reasoning patterns.
func (s *Store) DeduplicateReasoningPatterns(ctx context.Context, threshold float64, execute bool, logger *slog.Logger) (*dedup.DeduResult, error) {
	deduper := s.GetDeduplicator(logger)
	return deduper.DeduplicateReasoningPatterns(ctx, threshold, execute)
}

// DeduplicateDecisions performs deduplication on decisions.
func (s *Store) DeduplicateDecisions(ctx context.Context, threshold float64, execute bool, logger *slog.Logger) (*dedup.DeduResult, error) {
	deduper := s.GetDeduplicator(logger)
	return deduper.DeduplicateDecisions(ctx, threshold, execute)
}
