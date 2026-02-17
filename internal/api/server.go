package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/MikeSquared-Agency/dredd/internal/store"
)

type Server struct {
	httpServer *http.Server
	router     *chi.Mux
	port       int
	store      *store.Store
}

type DedupRequest struct {
	Threshold float64 `json:"threshold"`
	Execute   bool    `json:"execute"`
	Table     string  `json:"table"`
}

func NewServer(port int, apiToken string, db *store.Store) *Server {
	router := chi.NewRouter()
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)

	s := &Server{
		router: router,
		port:   port,
		store:  db,
	}

	// Health endpoint — unauthenticated (used by load balancers).
	router.Get("/health", s.health)

	// API routes — protected by Bearer token auth.
	router.Route("/api/v1", func(r chi.Router) {
		r.Use(BearerAuthMiddleware(apiToken))
		r.Get("/dredd/status", s.status)
		r.Post("/dedup", s.dedup)
	})

	return s
}

// BearerAuthMiddleware validates the Authorization header against a static
// Bearer token. If the token is empty, all requests are allowed (so the
// service can run without auth during development).
func BearerAuthMiddleware(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" {
				// No token configured — allow all requests.
				next.ServeHTTP(w, r)
				return
			}
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") || auth[7:] != token {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.port)
	slog.Info("API server starting", "addr", addr)
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: s.router,
	}
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully drains in-flight requests within the given context deadline.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"agent":  "dredd",
		"status": "shadow", // Phase 1: shadow mode
	})
}

func (s *Server) dedup(w http.ResponseWriter, r *http.Request) {
	var req DedupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"invalid JSON: %v"}`, err), http.StatusBadRequest)
		return
	}

	// Set defaults
	if req.Threshold == 0 {
		req.Threshold = 0.92
	}
	if req.Table == "" {
		req.Table = "all"
	}

	// Validate threshold
	if req.Threshold < 0.0 || req.Threshold > 1.0 {
		http.Error(w, `{"error":"threshold must be between 0.0 and 1.0"}`, http.StatusBadRequest)
		return
	}

	// Validate table
	if req.Table != "patterns" && req.Table != "decisions" && req.Table != "all" {
		http.Error(w, `{"error":"table must be 'patterns', 'decisions', or 'all'"}`, http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	logger := slog.Default()

	// Collect results
	var results []interface{}

	// Execute deduplication
	if req.Table == "patterns" || req.Table == "all" {
		result, err := s.store.DeduplicateReasoningPatterns(ctx, req.Threshold, req.Execute, logger)
		if err != nil {
			slog.Error("failed to deduplicate reasoning patterns", "error", err)
			http.Error(w, fmt.Sprintf(`{"error":"failed to deduplicate reasoning patterns: %v"}`, err), http.StatusInternalServerError)
			return
		}
		results = append(results, result)
	}

	if req.Table == "decisions" || req.Table == "all" {
		result, err := s.store.DeduplicateDecisions(ctx, req.Threshold, req.Execute, logger)
		if err != nil {
			slog.Error("failed to deduplicate decisions", "error", err)
			http.Error(w, fmt.Sprintf(`{"error":"failed to deduplicate decisions: %v"}`, err), http.StatusInternalServerError)
			return
		}
		results = append(results, result)
	}

	// Return results
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	
	// If single table, return the result directly; if all, return array
	if len(results) == 1 {
		json.NewEncoder(w).Encode(results[0])
	} else {
		json.NewEncoder(w).Encode(results)
	}
}

// Router returns the internal router for adding additional routes
func (s *Server) Router() *chi.Mux {
	return s.router
}
