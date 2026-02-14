package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	router *chi.Mux
	port   int
}

func NewServer(port int) *Server {
	router := chi.NewRouter()
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)

	s := &Server{
		router: router,
		port:   port,
	}

	router.Get("/health", s.health)
	router.Get("/api/v1/dredd/status", s.status)

	return s
}

func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.port)
	slog.Info("API server starting", "addr", addr)
	return http.ListenAndServe(addr, s.router)
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
