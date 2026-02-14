package extractor

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/MikeSquared-Agency/dredd/internal/anthropic"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestExtract_Success(t *testing.T) {
	extractionJSON := llmResponse{
		Decisions: []DecisionEpisode{
			{
				Domain:   "architecture",
				Category: "gate_approval",
				Severity: "routine",
				Summary:  "Approved deployment of auth service",
				Tags:     []string{"architecture", "approval"},
				Confidence: 0.92,
			},
		},
		Patterns: []ReasoningPattern{
			{
				PatternType:     "pushback",
				Summary:         "Prefers architectural solutions over quick fixes",
				ConversationArc: "Mike: Stop picking the quick fix every time",
				Tags:            []string{"architecture", "anti-pattern"},
				Confidence:      0.95,
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respJSON, _ := json.Marshal(extractionJSON)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": string(respJSON)},
			},
			"stop_reason": "end_turn",
		})
	}))
	defer server.Close()

	llm := anthropic.NewClient("test-key", "test-model")
	llm.SetTestTransport(server.URL)

	ext := New(llm, discardLogger())
	ownerUUID := uuid.New()

	result, err := ext.Extract(context.Background(), "test-session-1", ownerUUID, "Mike: Deploy the auth service\nAgent: Deployed.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(result.Decisions))
	}
	if result.Decisions[0].Domain != "architecture" {
		t.Errorf("expected domain architecture, got %q", result.Decisions[0].Domain)
	}
	if result.Decisions[0].Confidence != 0.92 {
		t.Errorf("expected confidence 0.92, got %f", result.Decisions[0].Confidence)
	}

	if len(result.Patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(result.Patterns))
	}
	if result.Patterns[0].PatternType != "pushback" {
		t.Errorf("expected pattern type pushback, got %q", result.Patterns[0].PatternType)
	}

	if result.SessionRef != "test-session-1" {
		t.Errorf("expected session ref test-session-1, got %q", result.SessionRef)
	}
	if result.OwnerUUID != ownerUUID {
		t.Errorf("expected owner uuid %s, got %s", ownerUUID, result.OwnerUUID)
	}
}

func TestExtract_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "this is not json"},
			},
			"stop_reason": "end_turn",
		})
	}))
	defer server.Close()

	llm := anthropic.NewClient("test-key", "test-model")
	llm.SetTestTransport(server.URL)

	ext := New(llm, discardLogger())

	_, err := ext.Extract(context.Background(), "test-session", uuid.New(), "some transcript")
	if err == nil {
		t.Fatal("expected error for invalid JSON response")
	}
}

func TestExtract_EmptyTranscript(t *testing.T) {
	extractionJSON := llmResponse{
		Decisions: []DecisionEpisode{},
		Patterns:  []ReasoningPattern{},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respJSON, _ := json.Marshal(extractionJSON)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": string(respJSON)},
			},
			"stop_reason": "end_turn",
		})
	}))
	defer server.Close()

	llm := anthropic.NewClient("test-key", "test-model")
	llm.SetTestTransport(server.URL)

	ext := New(llm, discardLogger())

	result, err := ext.Extract(context.Background(), "empty-session", uuid.New(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Decisions) != 0 {
		t.Errorf("expected 0 decisions, got %d", len(result.Decisions))
	}
	if len(result.Patterns) != 0 {
		t.Errorf("expected 0 patterns, got %d", len(result.Patterns))
	}
}
