package backfill

import (
	"strings"
	"testing"
	"time"
)

func TestChunkConversation_UnderLimit(t *testing.T) {
	msgs := makeMessages(5, time.Second)
	chunks := ChunkConversation(msgs, "test.jsonl", SourceCC)

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if len(chunks[0].Messages) != 5 {
		t.Errorf("expected 5 messages in chunk, got %d", len(chunks[0].Messages))
	}
	if chunks[0].SessionRef != "test.jsonl#chunk-0" {
		t.Errorf("session_ref = %q", chunks[0].SessionRef)
	}
}

func TestChunkConversation_SplitsOnMessageCount(t *testing.T) {
	msgs := makeMessages(45, time.Second) // 45 messages, should split at 20-message boundaries
	chunks := ChunkConversation(msgs, "test.jsonl", SourceCC)

	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks for 45 messages, got %d", len(chunks))
	}
	if len(chunks[0].Messages) != 20 {
		t.Errorf("chunk 0: expected 20 messages, got %d", len(chunks[0].Messages))
	}
	if len(chunks[1].Messages) != 20 {
		t.Errorf("chunk 1: expected 20 messages, got %d", len(chunks[1].Messages))
	}
	if len(chunks[2].Messages) != 5 {
		t.Errorf("chunk 2: expected 5 messages, got %d", len(chunks[2].Messages))
	}
}

func TestChunkConversation_SplitsOnTimeGap_CC(t *testing.T) {
	base := time.Date(2026, 2, 11, 10, 0, 0, 0, time.UTC)
	msgs := []ConversationMessage{
		{Role: "user", Text: "Hello", Timestamp: base},
		{Role: "assistant", Text: "Hi", Timestamp: base.Add(1 * time.Second)},
		// 10 minute gap — should split for CC (5 min threshold)
		{Role: "user", Text: "New topic", Timestamp: base.Add(10 * time.Minute)},
		{Role: "assistant", Text: "Sure", Timestamp: base.Add(10*time.Minute + time.Second)},
	}

	chunks := ChunkConversation(msgs, "test.jsonl", SourceCC)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks (split on 10min gap for CC), got %d", len(chunks))
	}
	if len(chunks[0].Messages) != 2 {
		t.Errorf("chunk 0: expected 2 messages, got %d", len(chunks[0].Messages))
	}
	if len(chunks[1].Messages) != 2 {
		t.Errorf("chunk 1: expected 2 messages, got %d", len(chunks[1].Messages))
	}
}

func TestChunkConversation_NoSplitOnSmallGap_Gateway(t *testing.T) {
	base := time.Date(2026, 2, 11, 10, 0, 0, 0, time.UTC)
	msgs := []ConversationMessage{
		{Role: "user", Text: "Hello", Timestamp: base},
		{Role: "assistant", Text: "Hi", Timestamp: base.Add(1 * time.Second)},
		// 7 minute gap — should NOT split for Gateway (10 min threshold)
		{Role: "user", Text: "Continue", Timestamp: base.Add(7 * time.Minute)},
		{Role: "assistant", Text: "OK", Timestamp: base.Add(7*time.Minute + time.Second)},
	}

	chunks := ChunkConversation(msgs, "test.jsonl", SourceGateway)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk (7min gap < 10min gateway threshold), got %d", len(chunks))
	}
}

func TestChunkConversation_SplitsOnTimeGap_Gateway(t *testing.T) {
	base := time.Date(2026, 2, 11, 10, 0, 0, 0, time.UTC)
	msgs := []ConversationMessage{
		{Role: "user", Text: "Hello", Timestamp: base},
		{Role: "assistant", Text: "Hi", Timestamp: base.Add(1 * time.Second)},
		// 15 minute gap — should split for Gateway
		{Role: "user", Text: "New topic", Timestamp: base.Add(15 * time.Minute)},
	}

	chunks := ChunkConversation(msgs, "test.jsonl", SourceGateway)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks (15min gap > 10min gateway threshold), got %d", len(chunks))
	}
}

func TestChunkConversation_Empty(t *testing.T) {
	chunks := ChunkConversation(nil, "test.jsonl", SourceCC)
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for nil messages, got %d", len(chunks))
	}
}

func TestFormatTranscript(t *testing.T) {
	chunk := Chunk{
		Messages: []ConversationMessage{
			{Role: "user", Text: "Deploy auth service"},
			{Role: "assistant", Text: "Deploying now."},
			{Role: "user", Text: "Thanks"},
		},
	}

	result := FormatTranscript(chunk)

	if !strings.Contains(result, "Human: Deploy auth service") {
		t.Errorf("expected 'Human: Deploy auth service', got:\n%s", result)
	}
	if !strings.Contains(result, "Assistant: Deploying now.") {
		t.Errorf("expected 'Assistant: Deploying now.', got:\n%s", result)
	}
	if !strings.Contains(result, "Human: Thanks") {
		t.Errorf("expected 'Human: Thanks', got:\n%s", result)
	}
}

func TestChunkConversation_TimestampsSet(t *testing.T) {
	base := time.Date(2026, 2, 11, 10, 0, 0, 0, time.UTC)
	msgs := []ConversationMessage{
		{Role: "user", Text: "A", Timestamp: base},
		{Role: "assistant", Text: "B", Timestamp: base.Add(5 * time.Second)},
		{Role: "user", Text: "C", Timestamp: base.Add(10 * time.Second)},
	}

	chunks := ChunkConversation(msgs, "test.jsonl", SourceCC)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}

	if !chunks[0].StartTime.Equal(base) {
		t.Errorf("StartTime = %v, want %v", chunks[0].StartTime, base)
	}
	if !chunks[0].EndTime.Equal(base.Add(10 * time.Second)) {
		t.Errorf("EndTime = %v, want %v", chunks[0].EndTime, base.Add(10*time.Second))
	}
}

func makeMessages(n int, gap time.Duration) []ConversationMessage {
	base := time.Date(2026, 2, 11, 10, 0, 0, 0, time.UTC)
	msgs := make([]ConversationMessage, n)
	for i := 0; i < n; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		msgs[i] = ConversationMessage{
			Role:      role,
			Text:      "message",
			Timestamp: base.Add(time.Duration(i) * gap),
		}
	}
	return msgs
}
