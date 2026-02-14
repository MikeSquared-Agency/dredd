package backfill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseCCFile_BasicConversation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	lines := []string{
		`{"type":"user","uuid":"aaa","parentUuid":null,"sessionId":"s1","timestamp":"2026-02-11T10:00:00Z","message":{"role":"user","content":"Hello, deploy the service"}}`,
		`{"type":"assistant","uuid":"bbb","parentUuid":"aaa","sessionId":"s1","timestamp":"2026-02-11T10:00:05Z","message":{"role":"assistant","content":[{"type":"text","text":"I'll deploy the service now."}]}}`,
		`{"type":"user","uuid":"ccc","parentUuid":"bbb","sessionId":"s1","timestamp":"2026-02-11T10:00:10Z","message":{"role":"user","content":"Great, thanks"}}`,
	}

	writeLines(t, path, lines)

	msgs, err := ParseCCFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}

	if msgs[0].Role != "user" || msgs[0].Text != "Hello, deploy the service" {
		t.Errorf("msg[0] = %q %q, want user 'Hello, deploy the service'", msgs[0].Role, msgs[0].Text)
	}
	if msgs[1].Role != "assistant" || msgs[1].Text != "I'll deploy the service now." {
		t.Errorf("msg[1] = %q %q", msgs[1].Role, msgs[1].Text)
	}
	if msgs[2].Role != "user" || msgs[2].Text != "Great, thanks" {
		t.Errorf("msg[2] = %q %q", msgs[2].Role, msgs[2].Text)
	}
}

func TestParseCCFile_SkipsToolResults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	lines := []string{
		`{"type":"user","uuid":"aaa","parentUuid":null,"sessionId":"s1","timestamp":"2026-02-11T10:00:00Z","message":{"role":"user","content":"List files"}}`,
		`{"type":"assistant","uuid":"bbb","parentUuid":"aaa","sessionId":"s1","timestamp":"2026-02-11T10:00:01Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"ls"}}]}}`,
		`{"type":"user","uuid":"ccc","parentUuid":"bbb","sessionId":"s1","timestamp":"2026-02-11T10:00:02Z","message":{"role":"user","content":[{"tool_use_id":"toolu_1","type":"tool_result","content":"file1\nfile2","is_error":false}]}}`,
		`{"type":"assistant","uuid":"ddd","parentUuid":"ccc","sessionId":"s1","timestamp":"2026-02-11T10:00:03Z","message":{"role":"assistant","content":[{"type":"text","text":"I found file1 and file2."}]}}`,
	}

	writeLines(t, path, lines)

	msgs, err := ParseCCFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have: user "List files", assistant "I found..." — skipping tool_use and tool_result
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (skipping tool_use + tool_result), got %d", len(msgs))
	}
	if msgs[0].Text != "List files" {
		t.Errorf("msg[0] text = %q, want 'List files'", msgs[0].Text)
	}
	if msgs[1].Text != "I found file1 and file2." {
		t.Errorf("msg[1] text = %q", msgs[1].Text)
	}
}

func TestParseCCFile_SkipsThinkingBlocks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	lines := []string{
		`{"type":"user","uuid":"aaa","parentUuid":null,"sessionId":"s1","timestamp":"2026-02-11T10:00:00Z","message":{"role":"user","content":"What should I do?"}}`,
		`{"type":"assistant","uuid":"bbb","parentUuid":"aaa","sessionId":"s1","timestamp":"2026-02-11T10:00:05Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"Let me consider the options..."},{"type":"text","text":"I recommend option A."}]}}`,
	}

	writeLines(t, path, lines)

	msgs, err := ParseCCFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[1].Text != "I recommend option A." {
		t.Errorf("expected only text block, got %q", msgs[1].Text)
	}
}

func TestParseCCFile_SkipsProgressEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	lines := []string{
		`{"type":"user","uuid":"aaa","parentUuid":null,"sessionId":"s1","timestamp":"2026-02-11T10:00:00Z","message":{"role":"user","content":"Hello"}}`,
		`{"type":"progress","uuid":"ppp","parentUuid":"aaa","sessionId":"s1","timestamp":"2026-02-11T10:00:01Z","data":{"type":"hook_progress"}}`,
		`{"type":"assistant","uuid":"bbb","parentUuid":"aaa","sessionId":"s1","timestamp":"2026-02-11T10:00:02Z","message":{"role":"assistant","content":[{"type":"text","text":"Hi there"}]}}`,
	}

	writeLines(t, path, lines)

	msgs, err := ParseCCFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (progress skipped), got %d", len(msgs))
	}
}

func TestParseCCFile_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	os.WriteFile(path, []byte(""), 0o644)

	msgs, err := ParseCCFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestParseCCFile_NotFound(t *testing.T) {
	_, err := ParseCCFile("/nonexistent/file.jsonl")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func writeLines(t *testing.T, path string, lines []string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, line := range lines {
		f.WriteString(line + "\n")
	}
}
