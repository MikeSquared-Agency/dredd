package backfill

import "time"

// ConversationMessage is a single turn in a conversation, shared across parsers.
type ConversationMessage struct {
	Role      string    // "user" or "assistant"
	Text      string
	Timestamp time.Time
}

// Chunk is a segment of conversation suitable for LLM extraction.
type Chunk struct {
	Messages   []ConversationMessage
	SessionRef string // source file + line range
	StartTime  time.Time
	EndTime    time.Time
}

// FileSource indicates which parser produced a conversation.
type FileSource int

const (
	SourceCC FileSource = iota
	SourceGateway
)
