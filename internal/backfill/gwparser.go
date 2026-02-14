package backfill

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)

// gwLine represents a single line from a Gateway session JSONL file.
type gwLine struct {
	Type      string    `json:"type"`
	ID        string    `json:"id"`
	ParentID  *string   `json:"parentId"`
	Timestamp string    `json:"timestamp"`
	Message   gwMessage `json:"message"`
}

type gwMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// ParseGatewayFile parses a Gateway session JSONL file into conversation messages.
func ParseGatewayFile(path string) ([]ConversationMessage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	type parsed struct {
		role string
		text string
		ts   time.Time
	}

	var items []parsed

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		var line gwLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}

		// Only process message events.
		if line.Type != "message" {
			continue
		}

		// Skip tool result messages.
		if line.Message.Role == "toolResult" {
			continue
		}

		// Only keep user and assistant.
		if line.Message.Role != "user" && line.Message.Role != "assistant" {
			continue
		}

		text := extractGatewayText(line.Message.Content)
		if text == "" {
			continue
		}

		ts, _ := time.Parse(time.RFC3339Nano, line.Timestamp)
		items = append(items, parsed{
			role: line.Message.Role,
			text: text,
			ts:   ts,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}

	// Order by timestamp.
	sort.Slice(items, func(i, j int) bool {
		return items[i].ts.Before(items[j].ts)
	})

	msgs := make([]ConversationMessage, len(items))
	for i, it := range items {
		msgs[i] = ConversationMessage{
			Role:      it.role,
			Text:      it.text,
			Timestamp: it.ts,
		}
	}

	return msgs, nil
}

// extractGatewayText extracts text content from gateway message content.
func extractGatewayText(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}

	// Try as plain string first.
	var plainStr string
	if err := json.Unmarshal(raw, &plainStr); err == nil {
		return plainStr
	}

	// Parse as content block array.
	var blocks []gwContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}

	// Collect text blocks only (skip thinking, toolCall, etc.).
	var text string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			if text != "" {
				text += "\n"
			}
			text += b.Text
		}
	}

	return text
}

type gwContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}
