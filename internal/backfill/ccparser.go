package backfill

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// ccLine represents a single line from a CC JSONL transcript.
type ccLine struct {
	Type       string     `json:"type"`
	UUID       string     `json:"uuid"`
	ParentUUID *string    `json:"parentUuid"`
	SessionID  string     `json:"sessionId"`
	Timestamp  string     `json:"timestamp"`
	Message    ccMessage  `json:"message"`
}

type ccMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// ParseCCFile parses a CC JSONL file into a conversation transcript.
func ParseCCFile(path string) ([]ConversationMessage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	// First pass: collect all lines keyed by UUID, track parent chains.
	byUUID := make(map[string]*ccLine)
	var roots []string // lines with no parent
	children := make(map[string]string) // parentUUID → childUUID (single chain)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB line buffer
	for scanner.Scan() {
		var line ccLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue // skip malformed lines
		}

		// Only care about user and assistant message types.
		if line.Type != "user" && line.Type != "assistant" {
			continue
		}

		byUUID[line.UUID] = &line

		if line.ParentUUID == nil || *line.ParentUUID == "" {
			roots = append(roots, line.UUID)
		} else {
			children[*line.ParentUUID] = line.UUID
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}

	if len(byUUID) == 0 {
		return nil, nil
	}

	// Walk the chain from root(s) following parentUuid → uuid links.
	var ordered []*ccLine
	for _, rootID := range roots {
		current := rootID
		for current != "" {
			if line, ok := byUUID[current]; ok {
				ordered = append(ordered, line)
			}
			current = children[current]
		}
	}

	// If chain walk missed some (e.g. orphans), fall back to adding unvisited by timestamp.
	visited := make(map[string]bool, len(ordered))
	for _, l := range ordered {
		visited[l.UUID] = true
	}
	if len(visited) < len(byUUID) {
		// Collect unvisited, they'll just be appended.
		for id, line := range byUUID {
			if !visited[id] {
				ordered = append(ordered, line)
			}
		}
	}

	// Convert to ConversationMessages, filtering out tool_result user messages.
	var msgs []ConversationMessage
	for _, line := range ordered {
		text, isToolResult := extractCCText(line)
		if isToolResult || text == "" {
			continue
		}

		ts, _ := time.Parse(time.RFC3339Nano, line.Timestamp)
		msgs = append(msgs, ConversationMessage{
			Role:      line.Type, // "user" or "assistant"
			Text:      text,
			Timestamp: ts,
		})
	}

	return msgs, nil
}

// extractCCText extracts the text content from a CC message.
// Returns the text and whether this was a tool_result message (to be skipped).
func extractCCText(line *ccLine) (string, bool) {
	if line.Message.Content == nil {
		return "", false
	}

	// Try as plain string first (some user messages).
	var plainStr string
	if err := json.Unmarshal(line.Message.Content, &plainStr); err == nil {
		return plainStr, false
	}

	// Parse as content block array.
	var blocks []ccContentBlock
	if err := json.Unmarshal(line.Message.Content, &blocks); err != nil {
		return "", false
	}

	// Check if this is a tool_result user message.
	for _, b := range blocks {
		if b.Type == "tool_result" {
			return "", true
		}
	}

	// Collect text blocks only (skip tool_use, thinking, etc.).
	var text string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			if text != "" {
				text += "\n"
			}
			text += b.Text
		}
	}

	return text, false
}

type ccContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}
