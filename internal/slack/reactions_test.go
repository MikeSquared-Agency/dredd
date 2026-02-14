package slack

import (
	"encoding/json"
	"io"
	"log/slog"
	"testing"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestParseReaction(t *testing.T) {
	tests := []struct {
		name     string
		reaction string
		want     ReviewVerdict
	}{
		{"thumbsup", "+1", VerdictConfirmed},
		{"thumbsup alt", "thumbsup", VerdictConfirmed},
		{"thumbsdown", "-1", VerdictRejected},
		{"thumbsdown alt", "thumbsdown", VerdictRejected},
		{"shrug", "shrug", VerdictSkipped},
		{"unknown reaction", "heart", VerdictUnknown},
		{"empty", "", VerdictUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseReaction(tt.reaction)
			if got != tt.want {
				t.Errorf("ParseReaction(%q) = %q, want %q", tt.reaction, got, tt.want)
			}
		})
	}
}

func TestParseReactionEvent(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]string
		wantReac string
		wantUser string
		wantChan string
	}{
		{
			name: "standard reaction",
			metadata: map[string]string{
				"text":       ":+1:",
				"user_id":    "U123",
				"channel_id": "C456",
				"message_ts": "1234567890.123456",
			},
			wantReac: "+1",
			wantUser: "U123",
			wantChan: "C456",
		},
		{
			name: "no colons",
			metadata: map[string]string{
				"text":       "thumbsup",
				"user_id":    "U789",
				"channel_id": "C012",
				"message_ts": "9999999.000",
			},
			wantReac: "thumbsup",
			wantUser: "U789",
			wantChan: "C012",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, _ := json.Marshal(map[string]any{
				"metadata": tt.metadata,
			})

			evt, err := ParseReactionEvent(payload, discardLogger())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if evt.Reaction != tt.wantReac {
				t.Errorf("Reaction = %q, want %q", evt.Reaction, tt.wantReac)
			}
			if evt.UserID != tt.wantUser {
				t.Errorf("UserID = %q, want %q", evt.UserID, tt.wantUser)
			}
			if evt.Channel != tt.wantChan {
				t.Errorf("Channel = %q, want %q", evt.Channel, tt.wantChan)
			}
		})
	}
}

func TestParseReactionEvent_InvalidJSON(t *testing.T) {
	_, err := ParseReactionEvent([]byte("not json"), discardLogger())
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
