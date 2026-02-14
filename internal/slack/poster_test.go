package slack

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestPostReviewThread_PostsPerItem(t *testing.T) {
	var calls []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		json.Unmarshal(body, &payload)
		calls = append(calls, payload)

		ts := fmt.Sprintf("ts-%d", len(calls))
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"ts": ts,
		})
	}))
	defer server.Close()

	p := NewPoster("xoxb-test", "C123", discardLogger())
	p.apiURL = server.URL

	result := &extractor.ExtractionResult{
		SessionRef: "test",
		OwnerUUID:  uuid.New(),
		Decisions: []extractor.DecisionEpisode{
			{Summary: "dec1", Tags: []string{"a"}, Severity: "routine", Confidence: 0.9},
			{Summary: "dec2", Tags: []string{"b"}, Severity: "critical", Confidence: 0.8},
		},
		Patterns: []extractor.ReasoningPattern{
			{PatternType: "pushback", Summary: "pat1", Confidence: 0.95},
		},
	}

	thread, err := p.PostReviewThread(context.Background(), result, "Test Session", "cc", "1m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expect 4 calls: 1 header + 2 decisions + 1 pattern
	if len(calls) != 4 {
		t.Fatalf("expected 4 Slack API calls, got %d", len(calls))
	}

	// Header should not have thread_ts.
	if _, hasThread := calls[0]["thread_ts"]; hasThread {
		t.Error("header message should not have thread_ts")
	}

	// Replies should have thread_ts matching header.
	for i := 1; i < len(calls); i++ {
		if calls[i]["thread_ts"] != "ts-1" {
			t.Errorf("call %d: expected thread_ts=ts-1, got %v", i, calls[i]["thread_ts"])
		}
	}

	// Check thread structure.
	if thread.HeaderTS != "ts-1" {
		t.Errorf("expected header TS ts-1, got %q", thread.HeaderTS)
	}
	if len(thread.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(thread.Items))
	}
	if thread.Items[0].Kind != "decision" || thread.Items[0].Idx != 0 {
		t.Errorf("item 0: expected decision/0, got %s/%d", thread.Items[0].Kind, thread.Items[0].Idx)
	}
	if thread.Items[1].Kind != "decision" || thread.Items[1].Idx != 1 {
		t.Errorf("item 1: expected decision/1, got %s/%d", thread.Items[1].Kind, thread.Items[1].Idx)
	}
	if thread.Items[2].Kind != "pattern" || thread.Items[2].Idx != 0 {
		t.Errorf("item 2: expected pattern/0, got %s/%d", thread.Items[2].Kind, thread.Items[2].Idx)
	}
}

func TestFormatDecisionItem(t *testing.T) {
	d := extractor.DecisionEpisode{
		Summary:    "Use pgx instead of gorm",
		Tags:       []string{"architecture", "database"},
		Severity:   "significant",
		Confidence: 0.92,
	}
	msg := formatDecisionItem(1, d)
	if !containsStr(msg, "Decision 1") {
		t.Error("expected 'Decision 1' in output")
	}
	if !containsStr(msg, "pgx instead of gorm") {
		t.Error("expected summary in output")
	}
	if !containsStr(msg, "significant") {
		t.Error("expected severity in output")
	}
}

func TestFormatPatternItem(t *testing.T) {
	p := extractor.ReasoningPattern{
		PatternType: "pushback",
		Summary:     "Prefers clean architecture",
		Confidence:  0.88,
	}
	msg := formatPatternItem(1, p)
	if !containsStr(msg, "Pattern 1") {
		t.Error("expected 'Pattern 1' in output")
	}
	if !containsStr(msg, "pushback") {
		t.Error("expected pattern type in output")
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
