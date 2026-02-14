package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	srv := NewServer(8750, "test-token")

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %q", body["status"])
	}
}

func TestHealthEndpoint_NoAuthRequired(t *testing.T) {
	// Health must be accessible without any token, even when auth is configured.
	srv := NewServer(8750, "some-secret")

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 without auth header, got %d", w.Code)
	}
}

func TestStatusEndpoint_WithValidToken(t *testing.T) {
	srv := NewServer(8750, "test-token")

	req := httptest.NewRequest("GET", "/api/v1/dredd/status", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body["agent"] != "dredd" {
		t.Errorf("expected agent dredd, got %q", body["agent"])
	}
	if body["status"] != "shadow" {
		t.Errorf("expected status shadow, got %q", body["status"])
	}
}

func TestStatusEndpoint_Unauthorized(t *testing.T) {
	srv := NewServer(8750, "test-token")

	// No Authorization header.
	req := httptest.NewRequest("GET", "/api/v1/dredd/status", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestStatusEndpoint_WrongToken(t *testing.T) {
	srv := NewServer(8750, "test-token")

	req := httptest.NewRequest("GET", "/api/v1/dredd/status", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestStatusEndpoint_NoTokenConfigured(t *testing.T) {
	// When no API token is configured, all requests are allowed.
	srv := NewServer(8750, "")

	req := httptest.NewRequest("GET", "/api/v1/dredd/status", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 when no token configured, got %d", w.Code)
	}
}

func TestNotFoundEndpoint(t *testing.T) {
	srv := NewServer(8750, "")

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
