package backfill

import (
	"fmt"
	"strings"
	"time"
)

const (
	maxChunkMessages  = 20
	ccTimeGap         = 5 * time.Minute
	gatewayTimeGap    = 10 * time.Minute
)

// ChunkConversation splits a conversation into segments suitable for LLM extraction.
// It breaks on time gaps and message count boundaries.
func ChunkConversation(msgs []ConversationMessage, sessionRef string, source FileSource) []Chunk {
	if len(msgs) == 0 {
		return nil
	}

	timeGap := ccTimeGap
	if source == SourceGateway {
		timeGap = gatewayTimeGap
	}

	var chunks []Chunk
	var current []ConversationMessage
	chunkIdx := 0

	for _, msg := range msgs {
		// Break on time gap (if we have messages and timestamps are valid).
		if len(current) > 0 && !msg.Timestamp.IsZero() {
			prev := current[len(current)-1]
			if !prev.Timestamp.IsZero() && msg.Timestamp.Sub(prev.Timestamp) > timeGap {
				chunks = append(chunks, buildChunk(current, sessionRef, chunkIdx))
				current = nil
				chunkIdx++
			}
		}

		// Break on message count boundary.
		if len(current) >= maxChunkMessages {
			chunks = append(chunks, buildChunk(current, sessionRef, chunkIdx))
			current = nil
			chunkIdx++
		}

		current = append(current, msg)
	}

	// Flush remaining.
	if len(current) > 0 {
		chunks = append(chunks, buildChunk(current, sessionRef, chunkIdx))
	}

	return chunks
}

func buildChunk(msgs []ConversationMessage, sessionRef string, idx int) Chunk {
	ref := fmt.Sprintf("%s#chunk-%d", sessionRef, idx)
	c := Chunk{
		Messages:   make([]ConversationMessage, len(msgs)),
		SessionRef: ref,
	}
	copy(c.Messages, msgs)

	if !msgs[0].Timestamp.IsZero() {
		c.StartTime = msgs[0].Timestamp
	}
	if !msgs[len(msgs)-1].Timestamp.IsZero() {
		c.EndTime = msgs[len(msgs)-1].Timestamp
	}

	return c
}

// FormatTranscript renders a chunk's messages as a Human:/Assistant: transcript string
// suitable for the extractor.
func FormatTranscript(chunk Chunk) string {
	var sb strings.Builder
	for _, msg := range chunk.Messages {
		switch msg.Role {
		case "user":
			sb.WriteString("Human: ")
		case "assistant":
			sb.WriteString("Assistant: ")
		default:
			sb.WriteString(msg.Role + ": ")
		}
		sb.WriteString(msg.Text)
		sb.WriteString("\n\n")
	}
	return sb.String()
}
