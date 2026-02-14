package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/MikeSquared-Agency/dredd/internal/extractor"
)

// WriteReasoningPattern inserts a reasoning pattern extraction.
func (s *Store) WriteReasoningPattern(ctx context.Context, ownerUUID uuid.UUID, sessionRef string, p extractor.ReasoningPattern, opts ...WriteOpts) (uuid.UUID, error) {
	var opt WriteOpts
	if len(opts) > 0 {
		opt = opts[0]
	}

	id := uuid.New()
	if opt.Embedding != nil {
		_, err := s.pool.Exec(ctx, `
			INSERT INTO reasoning_patterns (id, owner_uuid, session_ref, pattern_type, summary, conversation_arc, tags, dredd_confidence, embedding, review_status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'pending')`,
			id, ownerUUID, sessionRef, p.PatternType, p.Summary, p.ConversationArc, p.Tags, p.Confidence, pgVector(opt.Embedding),
		)
		if err != nil {
			return uuid.Nil, fmt.Errorf("insert reasoning pattern: %w", err)
		}
	} else {
		_, err := s.pool.Exec(ctx, `
			INSERT INTO reasoning_patterns (id, owner_uuid, session_ref, pattern_type, summary, conversation_arc, tags, dredd_confidence, review_status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'pending')`,
			id, ownerUUID, sessionRef, p.PatternType, p.Summary, p.ConversationArc, p.Tags, p.Confidence,
		)
		if err != nil {
			return uuid.Nil, fmt.Errorf("insert reasoning pattern: %w", err)
		}
	}
	return id, nil
}

// UpdatePatternReviewStatus updates the review status of a reasoning pattern.
func (s *Store) UpdatePatternReviewStatus(ctx context.Context, patternID uuid.UUID, status, note string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE reasoning_patterns SET review_status = $1, review_note = $2, reviewed_at = now()
		WHERE id = $3`,
		status, note, patternID,
	)
	return err
}

// GetPatternByID fetches a reasoning pattern by ID.
func (s *Store) GetPatternByID(ctx context.Context, id uuid.UUID) (*PatternRow, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, owner_uuid, session_ref, pattern_type, summary, conversation_arc, tags, dredd_confidence, review_status
		FROM reasoning_patterns WHERE id = $1`, id)

	var p PatternRow
	err := row.Scan(&p.ID, &p.OwnerUUID, &p.SessionRef, &p.PatternType, &p.Summary, &p.ConversationArc, &p.Tags, &p.Confidence, &p.ReviewStatus)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

type PatternRow struct {
	ID              uuid.UUID
	OwnerUUID       uuid.UUID
	SessionRef      string
	PatternType     string
	Summary         string
	ConversationArc string
	Tags            []string
	Confidence      float64
	ReviewStatus    string
}
