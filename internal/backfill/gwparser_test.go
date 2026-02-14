package backfill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseGatewayFile_BasicConversation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	lines := []string{
		`{"type":"session","version":3,"id":"s1","timestamp":"2026-02-09T07:30:00Z"}`,
		`{"type":"message","id":"m1","parentId":"s1","timestamp":"2026-02-09T07:30:01Z","message":{"role":"user","content":[{"type":"text","text":"Run the morning briefing"}]}}`,
		`{"type":"message","id":"m2","parentId":"m1","timestamp":"2026-02-09T07:30:05Z","message":{"role":"assistant","content":[{"type":"text","text":"Good morning! Here is your briefing."}]}}`,
		`{"type":"message","id":"m3","parentId":"m2","timestamp":"2026-02-09T07:30:10Z","message":{"role":"user","content":[{"type":"text","text":"Thanks"}]}}`,
	}

	writeLines(t, path, lines)

	msgs, err := ParseGatewayFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}

	if msgs[0].Role != "user" || msgs[0].Text != "Run the morning briefing" {
		t.Errorf("msg[0] = %q %q", msgs[0].Role, msgs[0].Text)
	}
	if msgs[1].Role != "assistant" || msgs[1].Text != "Good morning! Here is your briefing." {
		t.Errorf("msg[1] = %q %q", msgs[1].Role, msgs[1].Text)
	}
}

func TestParseGatewayFile_SkipsToolResults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	lines := []string{
		`{"type":"message","id":"m1","parentId":null,"timestamp":"2026-02-09T07:30:01Z","message":{"role":"user","content":[{"type":"text","text":"Check status"}]}}`,
		`{"type":"message","id":"m2","parentId":"m1","timestamp":"2026-02-09T07:30:02Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"t1","name":"exec","arguments":{"command":"curl localhost"}}]}}`,
		`{"type":"message","id":"m3","parentId":"m2","timestamp":"2026-02-09T07:30:03Z","message":{"role":"toolResult","toolCallId":"t1","content":[{"type":"text","text":"OK"}]}}`,
		`{"type":"message","id":"m4","parentId":"m3","timestamp":"2026-02-09T07:30:04Z","message":{"role":"assistant","content":[{"type":"text","text":"Service is running."}]}}`,
	}

	writeLines(t, path, lines)

	msgs, err := ParseGatewayFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// user "Check status", assistant "Service is running" — toolCall has no text blocks, toolResult skipped
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Text != "Check status" {
		t.Errorf("msg[0] text = %q", msgs[0].Text)
	}
	if msgs[1].Text != "Service is running." {
		t.Errorf("msg[1] text = %q", msgs[1].Text)
	}
}

func TestParseGatewayFile_SkipsThinkingBlocks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	lines := []string{
		`{"type":"message","id":"m1","parentId":null,"timestamp":"2026-02-09T07:30:01Z","message":{"role":"user","content":[{"type":"text","text":"What do you think?"}]}}`,
		`{"type":"message","id":"m2","parentId":"m1","timestamp":"2026-02-09T07:30:05Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"Hmm let me consider"},{"type":"text","text":"I think we should proceed."}]}}`,
	}

	writeLines(t, path, lines)

	msgs, err := ParseGatewayFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[1].Text != "I think we should proceed." {
		t.Errorf("expected only text, got %q", msgs[1].Text)
	}
}

func TestParseGatewayFile_SkipsNonMessageEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	lines := []string{
		`{"type":"session","version":3,"id":"s1","timestamp":"2026-02-09T07:30:00Z"}`,
		`{"type":"model_change","id":"mc1","timestamp":"2026-02-09T07:30:00Z","modelId":"claude-opus-4-6"}`,
		`{"type":"thinking_level_change","id":"tlc1","timestamp":"2026-02-09T07:30:00Z","thinkingLevel":"low"}`,
		`{"type":"message","id":"m1","parentId":null,"timestamp":"2026-02-09T07:30:01Z","message":{"role":"user","content":[{"type":"text","text":"Hello"}]}}`,
		`{"type":"custom","customType":"model-snapshot","id":"cs1","timestamp":"2026-02-09T07:30:01Z"}`,
		`{"type":"message","id":"m2","parentId":"m1","timestamp":"2026-02-09T07:30:02Z","message":{"role":"assistant","content":[{"type":"text","text":"Hi"}]}}`,
	}

	writeLines(t, path, lines)

	msgs, err := ParseGatewayFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
}

func TestParseGatewayFile_OrdersByTimestamp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	// Write out-of-order
	lines := []string{
		`{"type":"message","id":"m2","parentId":"m1","timestamp":"2026-02-09T07:30:05Z","message":{"role":"assistant","content":[{"type":"text","text":"Second"}]}}`,
		`{"type":"message","id":"m1","parentId":null,"timestamp":"2026-02-09T07:30:01Z","message":{"role":"user","content":[{"type":"text","text":"First"}]}}`,
	}

	writeLines(t, path, lines)

	msgs, err := ParseGatewayFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Text != "First" {
		t.Errorf("expected 'First' first, got %q", msgs[0].Text)
	}
	if msgs[1].Text != "Second" {
		t.Errorf("expected 'Second' second, got %q", msgs[1].Text)
	}
}

func TestParseGatewayFile_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	os.WriteFile(path, []byte(""), 0o644)

	msgs, err := ParseGatewayFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestParseGatewayFile_PlainStringContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	lines := []string{
		`{"type":"message","id":"m1","parentId":null,"timestamp":"2026-02-09T07:30:01Z","message":{"role":"user","content":"plain string message"}}`,
		`{"type":"message","id":"m2","parentId":"m1","timestamp":"2026-02-09T07:30:02Z","message":{"role":"assistant","content":"plain string response"}}`,
	}

	writeLines(t, path, lines)

	msgs, err := ParseGatewayFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Text != "plain string message" {
		t.Errorf("msg[0] text = %q", msgs[0].Text)
	}
}
