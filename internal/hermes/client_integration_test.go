//go:build integration

package hermes

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"
)

func skipWithoutNATS(t *testing.T) string {
	t.Helper()
	url := os.Getenv("NATS_URL")
	if url == "" {
		t.Skip("NATS_URL not set, skipping integration test")
	}
	return url
}

func TestIntegration_PubSub(t *testing.T) {
	natsURL := skipWithoutNATS(t)
	ctx := context.Background()
	logger := slog.Default()

	client, err := NewClient(ctx, natsURL, logger)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Close()

	received := make(chan map[string]string, 1)

	err = client.Subscribe("swarm.dredd.test.>", func(subject string, data []byte) {
		var msg map[string]string
		json.Unmarshal(data, &msg)
		received <- msg
	})
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	// Give subscription time to propagate
	time.Sleep(100 * time.Millisecond)

	err = client.Publish("swarm.dredd.test.ping", map[string]string{
		"message": "hello from integration test",
	})
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	select {
	case msg := <-received:
		if msg["message"] != "hello from integration test" {
			t.Errorf("expected hello message, got %v", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for message")
	}
}
