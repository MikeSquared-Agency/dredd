package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestComplete_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("expected x-api-key test-key, got %q", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("expected anthropic-version 2023-06-01, got %q", r.Header.Get("anthropic-version"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", r.Header.Get("Content-Type"))
		}

		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.Model != "test-model" {
			t.Errorf("expected model test-model, got %q", req.Model)
		}
		if req.System != "you are a test" {
			t.Errorf("expected system prompt, got %q", req.System)
		}
		if len(req.Messages) != 1 || req.Messages[0].Content != "hello" {
			t.Errorf("unexpected messages: %+v", req.Messages)
		}
		if req.MaxTokens != 100 {
			t.Errorf("expected max_tokens 100, got %d", req.MaxTokens)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response{
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{
				{Type: "text", Text: "world"},
			},
			StopReason: "end_turn",
		})
	}))
	defer server.Close()

	c := NewClient("test-key", "test-model")
	c.SetTestTransport(server.URL)

	result, err := c.Complete(context.Background(), "you are a test", []Message{{Role: "user", Content: "hello"}}, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "world" {
		t.Errorf("expected 'world', got %q", result)
	}
}

func TestComplete_OAuthToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// OAuth tokens must use Bearer auth, not x-api-key.
		if auth := r.Header.Get("Authorization"); auth != "Bearer sk-ant-oat01-testtoken" {
			t.Errorf("expected Authorization Bearer, got %q", auth)
		}
		if r.Header.Get("x-api-key") != "" {
			t.Error("x-api-key should not be set for OAuth tokens")
		}
		if beta := r.Header.Get("anthropic-beta"); beta != "oauth-2025-04-20" {
			t.Errorf("expected anthropic-beta oauth-2025-04-20, got %q", beta)
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("expected anthropic-version 2023-06-01, got %q", r.Header.Get("anthropic-version"))
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response{
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{
				{Type: "text", Text: "oauth works"},
			},
			StopReason: "end_turn",
		})
	}))
	defer server.Close()

	c := NewClient("sk-ant-oat01-testtoken", "test-model")
	c.SetTestTransport(server.URL)

	result, err := c.Complete(context.Background(), "", []Message{{Role: "user", Content: "hello"}}, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "oauth works" {
		t.Errorf("expected 'oauth works', got %q", result)
	}
}

func TestComplete_StandardAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Standard API keys must use x-api-key, not Bearer auth.
		if r.Header.Get("x-api-key") != "sk-ant-api03-testkey" {
			t.Errorf("expected x-api-key, got %q", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("Authorization should not be set for standard API keys")
		}
		if r.Header.Get("anthropic-beta") != "" {
			t.Error("anthropic-beta should not be set for standard API keys")
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response{
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{
				{Type: "text", Text: "apikey works"},
			},
			StopReason: "end_turn",
		})
	}))
	defer server.Close()

	c := NewClient("sk-ant-api03-testkey", "test-model")
	c.SetTestTransport(server.URL)

	result, err := c.Complete(context.Background(), "", []Message{{Role: "user", Content: "hello"}}, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "apikey works" {
		t.Errorf("expected 'apikey works', got %q", result)
	}
}

func TestComplete_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"type":    "invalid_request_error",
				"message": "max_tokens is too large",
			},
		})
	}))
	defer server.Close()

	c := NewClient("test-key", "test-model")
	c.SetTestTransport(server.URL)

	_, err := c.Complete(context.Background(), "", []Message{{Role: "user", Content: "hi"}}, 100)
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}

func TestComplete_EmptyContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response{
			Content:    nil,
			StopReason: "end_turn",
		})
	}))
	defer server.Close()

	c := NewClient("test-key", "test-model")
	c.SetTestTransport(server.URL)

	_, err := c.Complete(context.Background(), "", []Message{{Role: "user", Content: "hi"}}, 100)
	if err == nil {
		t.Fatal("expected error for empty content response")
	}
}
