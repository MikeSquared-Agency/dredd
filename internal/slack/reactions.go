package slack

import (
	"encoding/json"
	"fmt"
	"log/slog"
)

// ReactionEvent is the structure received from slack-forwarder via NATS.
type ReactionEvent struct {
	Reaction  string `json:"reaction"`
	UserID    string `json:"user_id"`
	Channel   string `json:"channel"`
	MessageTS string `json:"message_ts"`
}

// ReviewVerdict maps a Slack reaction to a review status.
type ReviewVerdict string

const (
	VerdictConfirmed ReviewVerdict = "confirmed"
	VerdictRejected  ReviewVerdict = "rejected"
	VerdictSkipped   ReviewVerdict = "skipped"
	VerdictUnknown   ReviewVerdict = "unknown"
)

// ParseReaction converts a Slack reaction emoji name to a review verdict.
func ParseReaction(reaction string) ReviewVerdict {
	switch reaction {
	case "+1", "thumbsup":
		return VerdictConfirmed
	case "-1", "thumbsdown":
		return VerdictRejected
	case "shrug":
		return VerdictSkipped
	default:
		return VerdictUnknown
	}
}

// ParseReactionEvent parses a NATS message payload from slack-forwarder into a ReactionEvent.
func ParseReactionEvent(data []byte, logger *slog.Logger) (*ReactionEvent, error) {
	// The slack-forwarder publishes events with metadata in a wrapper.
	var wrapper struct {
		Metadata map[string]string `json:"metadata"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, fmt.Errorf("parse reaction wrapper: %w", err)
	}

	evt := &ReactionEvent{
		Reaction:  wrapper.Metadata["text"],
		UserID:    wrapper.Metadata["user_id"],
		Channel:   wrapper.Metadata["channel_id"],
		MessageTS: wrapper.Metadata["message_ts"],
	}

	// Clean reaction text (remove colons if present)
	if len(evt.Reaction) > 2 && evt.Reaction[0] == ':' && evt.Reaction[len(evt.Reaction)-1] == ':' {
		evt.Reaction = evt.Reaction[1 : len(evt.Reaction)-1]
	}

	return evt, nil
}
