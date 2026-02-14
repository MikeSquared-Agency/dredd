package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/MikeSquared-Agency/dredd/internal/extractor"
)

const defaultPostMessageURL = "https://slack.com/api/chat.postMessage"

type Poster struct {
	token   string
	channel string
	client  *http.Client
	logger  *slog.Logger
	apiURL  string
}

func NewPoster(token, channel string, logger *slog.Logger) *Poster {
	return &Poster{
		token:   token,
		channel: channel,
		client:  &http.Client{Timeout: 10 * time.Second},
		apiURL:  defaultPostMessageURL,
		logger:  logger,
	}
}

// PostReviewSummary posts the extraction summary to Slack for human review.
// Returns the message timestamp (ts) which is used for tracking reactions.
func (p *Poster) PostReviewSummary(ctx context.Context, result *extractor.ExtractionResult, sessionTitle, surface, duration string) (string, error) {
	text := formatReviewMessage(result, sessionTitle, surface, duration)

	body, err := json.Marshal(map[string]any{
		"channel": p.channel,
		"text":    text,
		"blocks": []map[string]any{
			{
				"type": "section",
				"text": map[string]any{
					"type": "mrkdwn",
					"text": text,
				},
			},
			{
				"type": "context",
				"elements": []map[string]any{
					{
						"type": "mrkdwn",
						"text": "React: :+1: correct | :-1: wrong | :shrug: skip",
					},
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("marshal slack payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+p.token)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("slack post: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	var slackResp struct {
		OK bool   `json:"ok"`
		TS string `json:"ts"`
		Error string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(respBody, &slackResp); err != nil {
		return "", fmt.Errorf("parse slack response: %w", err)
	}
	if !slackResp.OK {
		return "", fmt.Errorf("slack error: %s", slackResp.Error)
	}

	p.logger.Info("posted review to slack", "ts", slackResp.TS, "session_ref", result.SessionRef)
	return slackResp.TS, nil
}

// PostThread posts a threaded reply to a message.
func (p *Poster) PostThread(ctx context.Context, threadTS, text string) error {
	body, err := json.Marshal(map[string]any{
		"channel":   p.channel,
		"thread_ts": threadTS,
		"text":      text,
	})
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+p.token)

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func formatReviewMessage(result *extractor.ExtractionResult, title, surface, duration string) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "*Session:* %s (%s, %s)\n", title, surface, duration)
	fmt.Fprintf(&sb, "*Owner:* %s\n\n", result.OwnerUUID.String())

	if len(result.Decisions) > 0 {
		fmt.Fprintf(&sb, "*Decisions found: %d*\n", len(result.Decisions))
		for i, d := range result.Decisions {
			tags := strings.Join(d.Tags, ", ")
			fmt.Fprintf(&sb, "%d. %s\n   Tags: %s | Confidence: %.2f\n", i+1, d.Summary, tags, d.Confidence)
		}
		sb.WriteString("\n")
	}

	if len(result.Patterns) > 0 {
		fmt.Fprintf(&sb, "*Patterns found: %d*\n", len(result.Patterns))
		for i, p := range result.Patterns {
			fmt.Fprintf(&sb, "%d. [%s] %s\n   Confidence: %.2f\n", i+1, p.PatternType, p.Summary, p.Confidence)
		}
	}

	if len(result.Decisions) == 0 && len(result.Patterns) == 0 {
		sb.WriteString("_No decisions or patterns extracted from this session._")
	}

	return sb.String()
}
