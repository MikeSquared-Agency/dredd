package slack

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/MikeSquared-Agency/dredd/internal/extractor"
)

func TestFormatReviewMessage_WithDecisionsAndPatterns(t *testing.T) {
	result := &extractor.ExtractionResult{
		SessionRef: "test-session",
		OwnerUUID:  uuid.MustParse("9f6ed519-0000-0000-0000-000000000000"),
		Decisions: []extractor.DecisionEpisode{
			{
				Summary:    "Rejected timestamp correlation for identity",
				Tags:       []string{"architecture", "correction"},
				Confidence: 0.92,
			},
		},
		Patterns: []extractor.ReasoningPattern{
			{
				PatternType: "pushback",
				Summary:     "Prefers architectural solutions over quick fixes",
				Confidence:  0.95,
			},
		},
	}

	msg := formatReviewMessage(result, "Fix auth bug", "cc", "5m42s")

	if msg == "" {
		t.Fatal("expected non-empty message")
	}

	// Check key content is present
	checks := []string{
		"Fix auth bug",
		"cc",
		"5m42s",
		"Decisions found: 1",
		"Rejected timestamp correlation",
		"architecture, correction",
		"0.92",
		"Patterns found: 1",
		"pushback",
		"Prefers architectural solutions",
		"0.95",
	}
	for _, check := range checks {
		if !containsStr(msg, check) {
			t.Errorf("expected message to contain %q", check)
		}
	}
}

func TestFormatReviewMessage_Empty(t *testing.T) {
	result := &extractor.ExtractionResult{
		SessionRef: "empty-session",
		OwnerUUID:  uuid.New(),
		Decisions:  nil,
		Patterns:   nil,
	}

	msg := formatReviewMessage(result, "Empty", "cc", "0s")

	if !containsStr(msg, "No decisions or patterns") {
		t.Errorf("expected empty message, got %q", msg)
	}
}

func TestPostReviewSummary_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer xoxb-test" {
			t.Errorf("expected Bearer xoxb-test, got %q", r.Header.Get("Authorization"))
		}

		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		json.Unmarshal(body, &payload)

		if payload["channel"] != "C123" {
			t.Errorf("expected channel C123, got %v", payload["channel"])
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"ts": "1234567890.123456",
		})
	}))
	defer server.Close()

	p := NewPoster("xoxb-test", "C123", discardLogger())
	p.apiURL = server.URL

	result := &extractor.ExtractionResult{
		SessionRef: "test",
		OwnerUUID:  uuid.New(),
		Decisions:  []extractor.DecisionEpisode{{Summary: "test decision", Tags: []string{"test"}, Confidence: 0.9}},
	}

	ts, err := p.PostReviewSummary(context.Background(), result, "Test Session", "cc", "1m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ts != "1234567890.123456" {
		t.Errorf("expected ts 1234567890.123456, got %q", ts)
	}
}

func TestPostReviewSummary_SlackError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"ok":    false,
			"error": "channel_not_found",
		})
	}))
	defer server.Close()

	p := NewPoster("xoxb-test", "C123", discardLogger())
	p.apiURL = server.URL

	result := &extractor.ExtractionResult{
		SessionRef: "test",
		OwnerUUID:  uuid.New(),
	}

	_, err := p.PostReviewSummary(context.Background(), result, "Test", "cc", "1m")
	if err == nil {
		t.Fatal("expected error for slack error response")
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
