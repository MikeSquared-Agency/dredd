package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/MikeSquared-Agency/dredd/internal/extractor"
)

// WriteStyle persists a writing style extraction.
func (s *Store) WriteStyle(ctx context.Context, ownerUUID uuid.UUID, sessionRef, source string, style extractor.WritingStyle) error {
	samples, _ := json.Marshal(style.Samples)
	traits, _ := json.Marshal(style.Traits)
	vocab, _ := json.Marshal(style.Vocabulary)
	patterns, _ := json.Marshal(style.Patterns)
	avoids, _ := json.Marshal(style.Avoids)

	_, err := s.pool.Exec(ctx, `
		INSERT INTO writing_styles (owner_id, session_ref, source, speaker, context, samples, traits, vocabulary, patterns, avoids, emoji_style, confidence)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		ownerUUID, sessionRef, source, style.Speaker, style.Context,
		samples, traits, vocab, patterns, avoids,
		style.EmojiStyle, style.Confidence,
	)
	if err != nil {
		return fmt.Errorf("insert writing_style: %w", err)
	}
	return nil
}
