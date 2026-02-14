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
)

type Server struct {
	httpServer *http.Server
	router     *chi.Mux
	port       int
}

func NewServer(port int, apiToken string) *Server {
	router := chi.NewRouter()
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)

	s := &Server{
		router: router,
		port:   port,
	}

	// Health endpoint — unauthenticated (used by load balancers).
	router.Get("/health", s.health)

	// API routes — protected by Bearer token auth.
	router.Route("/api/v1", func(r chi.Router) {
		r.Use(BearerAuthMiddleware(apiToken))
		r.Get("/dredd/status", s.status)
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
