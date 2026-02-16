package extractor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/MikeSquared-Agency/dredd/internal/anthropic"
)

type Extractor struct {
	llm    *anthropic.Client
	logger *slog.Logger
}

func New(llm *anthropic.Client, logger *slog.Logger) *Extractor {
	return &Extractor{llm: llm, logger: logger}
}

type llmResponse struct {
	Decisions []DecisionEpisode  `json:"decisions"`
	Patterns  []ReasoningPattern `json:"patterns"`
	Styles    []WritingStyle     `json:"styles"`
}

// Extract processes a transcript and returns structured extractions.
func (e *Extractor) Extract(ctx context.Context, sessionRef string, ownerUUID uuid.UUID, transcript string) (*ExtractionResult, error) {
	prompt := fmt.Sprintf(extractionUserPrompt, sessionRef, ownerUUID.String(), transcript)

	messages := []anthropic.Message{
		{Role: "user", Content: prompt},
	}

	e.logger.Info("extracting from transcript",
		"session_ref", sessionRef,
		"owner", ownerUUID.String(),
		"transcript_len", len(transcript),
	)

	raw, err := e.llm.Complete(ctx, systemPrompt, messages, 8192)
	if err != nil {
		return nil, fmt.Errorf("llm extraction: %w", err)
	}

	var resp llmResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		e.logger.Error("failed to parse extraction response",
			"error", err,
			"raw", raw,
		)
		return nil, fmt.Errorf("parse extraction: %w", err)
	}

	e.logger.Info("extraction complete",
		"session_ref", sessionRef,
		"decisions", len(resp.Decisions),
		"patterns", len(resp.Patterns),
		"styles", len(resp.Styles),
	)

	return &ExtractionResult{
		SessionRef: sessionRef,
		OwnerUUID:  ownerUUID,
		Decisions:  resp.Decisions,
		Patterns:   resp.Patterns,
		Styles:     resp.Styles,
	}, nil
}
