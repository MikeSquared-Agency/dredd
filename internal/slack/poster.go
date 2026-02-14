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

// ReviewItem represents a single decision or pattern posted as a thread reply.
type ReviewItem struct {
	TS   string // Slack message timestamp of the thread reply
	Kind string // "decision" or "pattern"
	Idx  int    // index into the result's Decisions or Patterns slice
}

// ReviewThread holds all posted messages for a single extraction review.
type ReviewThread struct {
	HeaderTS string       // the parent summary message TS
	Items    []ReviewItem // per-item thread replies
}

// PostReviewThread posts a summary header followed by individual thread replies
// for each decision and pattern. Each reply gets its own reaction prompt so Mike
// can review per item. Returns the full ReviewThread for reaction tracking.
func (p *Poster) PostReviewThread(ctx context.Context, result *extractor.ExtractionResult, sessionTitle, surface, duration string) (*ReviewThread, error) {
	// 1. Post the summary header.
	headerText := formatHeaderMessage(result, sessionTitle, surface, duration)
	headerTS, err := p.postMessage(ctx, headerText, "")
	if err != nil {
		return nil, fmt.Errorf("post header: %w", err)
	}

	thread := &ReviewThread{HeaderTS: headerTS}

	// 2. Post each decision as a thread reply.
	for i, d := range result.Decisions {
		text := formatDecisionItem(i+1, d)
		ts, err := p.postMessage(ctx, text, headerTS)
		if err != nil {
			p.logger.Error("failed to post decision item", "index", i, "error", err)
			continue
		}
		thread.Items = append(thread.Items, ReviewItem{TS: ts, Kind: "decision", Idx: i})
	}

	// 3. Post each pattern as a thread reply.
	for i, pat := range result.Patterns {
		text := formatPatternItem(i+1, pat)
		ts, err := p.postMessage(ctx, text, headerTS)
		if err != nil {
			p.logger.Error("failed to post pattern item", "index", i, "error", err)
			continue
		}
		thread.Items = append(thread.Items, ReviewItem{TS: ts, Kind: "pattern", Idx: i})
	}

	p.logger.Info("posted review thread to slack",
		"header_ts", headerTS,
		"items", len(thread.Items),
		"session_ref", result.SessionRef,
	)
	return thread, nil
}

// PostReviewSummary posts the extraction summary to Slack for human review.
// Returns the message timestamp (ts) which is used for tracking reactions.
// Deprecated: Use PostReviewThread for per-item review.
func (p *Poster) PostReviewSummary(ctx context.Context, result *extractor.ExtractionResult, sessionTitle, surface, duration string) (string, error) {
	text := formatReviewMessage(result, sessionTitle, surface, duration)
	ts, err := p.postMessage(ctx, text, "")
	if err != nil {
		return "", err
	}
	p.logger.Info("posted review to slack", "ts", ts, "session_ref", result.SessionRef)
	return ts, nil
}

// postMessage sends a single Slack message, optionally as a thread reply.
// Returns the message TS.
func (p *Poster) postMessage(ctx context.Context, text, threadTS string) (string, error) {
	payload := map[string]any{
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
	}
	if threadTS != "" {
		payload["thread_ts"] = threadTS
	}

	body, err := json.Marshal(payload)
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
		OK    bool   `json:"ok"`
		TS    string `json:"ts"`
		Error string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(respBody, &slackResp); err != nil {
		return "", fmt.Errorf("parse slack response: %w", err)
	}
	if !slackResp.OK {
		return "", fmt.Errorf("slack error: %s", slackResp.Error)
	}

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

// formatHeaderMessage creates the summary header for a review thread.
func formatHeaderMessage(result *extractor.ExtractionResult, title, surface, duration string) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "*Session:* %s (%s, %s)\n", title, surface, duration)
	fmt.Fprintf(&sb, "*Owner:* %s\n\n", result.OwnerUUID.String())

	if len(result.Decisions) == 0 && len(result.Patterns) == 0 {
		sb.WriteString("_No decisions or patterns extracted from this session._")
	} else {
		fmt.Fprintf(&sb, "Extracted *%d* decisions, *%d* patterns. Review each item in the thread below.", len(result.Decisions), len(result.Patterns))
	}

	return sb.String()
}

// formatDecisionItem creates the Slack message for a single decision.
func formatDecisionItem(num int, d extractor.DecisionEpisode) string {
	tags := strings.Join(d.Tags, ", ")
	return fmt.Sprintf("*Decision %d:* %s\nTags: %s | Severity: %s | Confidence: %.2f", num, d.Summary, tags, d.Severity, d.Confidence)
}

// formatPatternItem creates the Slack message for a single pattern.
func formatPatternItem(num int, p extractor.ReasoningPattern) string {
	return fmt.Sprintf("*Pattern %d:* [%s] %s\nConfidence: %.2f", num, p.PatternType, p.Summary, p.Confidence)
}

// formatReviewMessage creates the legacy single-message summary.
// Retained for backward compatibility.
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
