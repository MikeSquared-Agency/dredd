package hermes

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
)

// SubjectCorrection is the NATS subject for prompt-loop correction signals.
const SubjectCorrection = "swarm.dredd.correction"

// CorrectionSignal is emitted when a decision is confirmed or rejected,
// enabling downstream prompt optimisation loops to adjust extraction quality.
type CorrectionSignal struct {
	SessionRef     string `json:"session_ref"`
	DecisionID     string `json:"decision_id"`
	AgentID        string `json:"agent_id"`
	ModelID        string `json:"model_id"`
	ModelTier      string `json:"model_tier"`
	CorrectionType string `json:"correction_type"`
	Category       string `json:"category"`
	Severity       string `json:"severity"`
}

type Client struct {
	conn   *nats.Conn
	subs   []*nats.Subscription
	logger *slog.Logger
}

func NewClient(ctx context.Context, url, token string, logger *slog.Logger) (*Client, error) {
	opts := []nats.Option{
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(60),
		nats.ReconnectWait(2 * time.Second),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				logger.Warn("nats disconnected", "error", err)
			}
		}),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			logger.Info("nats reconnected")
		}),
	}
	if token != "" {
		opts = append(opts, nats.Token(token))
	}

	nc, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}

	return &Client{conn: nc, logger: logger}, nil
}

func (c *Client) Publish(subject string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	return c.conn.Publish(subject, payload)
}

func (c *Client) Subscribe(subject string, handler func(subject string, data []byte)) error {
	sub, err := c.conn.Subscribe(subject, func(msg *nats.Msg) {
		handler(msg.Subject, msg.Data)
	})
	if err != nil {
		return fmt.Errorf("subscribe %s: %w", subject, err)
	}
	c.subs = append(c.subs, sub)
	c.logger.Info("subscribed", "subject", subject)
	return nil
}

func (c *Client) Close() {
	for _, sub := range c.subs {
		_ = sub.Unsubscribe()
	}
	c.conn.Close()
}
