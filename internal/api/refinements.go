package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/MikeSquared-Agency/dredd/internal/hermes"
	"github.com/MikeSquared-Agency/dredd/internal/refinement"
	"github.com/MikeSquared-Agency/dredd/internal/store"
)

// ScanRequest represents the request payload for refinement scans
type ScanRequest struct {
	Since     *string  `json:"since,omitempty"`     // ISO timestamp
	SOULSlug  *string  `json:"soul_slug,omitempty"` // Target SOUL slug
	Threshold *float64 `json:"threshold,omitempty"` // Similarity threshold
	DryRun    bool     `json:"dry_run"`             // Don't publish, just return results
}

// ScanResponse represents the response from refinement scans
type ScanResponse struct {
	Clusters []refinement.PatternCluster `json:"clusters"`
	Count    int                         `json:"count"`
	DryRun   bool                        `json:"dry_run"`
}

// AddRefinementRoutes adds refinement endpoints to an existing router
func AddRefinementRoutes(router chi.Router, apiToken string, store *store.Store, hermes *hermes.Client) {
	router.Route("/api/v1/refinements", func(r chi.Router) {
		r.Use(BearerAuthMiddleware(apiToken))
		
		// Create handlers with dependencies
		handler := &refinementHandler{
			store:  store,
			hermes: hermes,
		}
		
		r.Post("/scan", handler.scanRefinements)
		r.Get("/scan", handler.scanRefinementsDryRun)
	})
}

// refinementHandler holds dependencies for refinement endpoints
type refinementHandler struct {
	store  *store.Store
	hermes *hermes.Client
}

// scanRefinements handles POST /api/v1/refinements/scan
func (h *refinementHandler) scanRefinements(w http.ResponseWriter, r *http.Request) {
	var req ScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"invalid JSON: %v"}`, err), http.StatusBadRequest)
		return
	}

	clusters, err := h.performScan(r.Context(), &req)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"scan failed: %v"}`, err), http.StatusInternalServerError)
		return
	}

	// If not dry run, publish refinement proposals
	if !req.DryRun && len(clusters) > 0 {
		publisher := refinement.NewPublisher(h.hermes)
		soulSlug := "kai-soul" // Default SOUL slug
		if req.SOULSlug != nil {
			soulSlug = *req.SOULSlug
		}

		for _, cluster := range clusters {
			if err := publisher.PublishRefinementProposal(cluster, soulSlug); err != nil {
				// Log error but don't fail the request
				slog.Warn("failed to publish refinement proposal",
					"pattern_type", cluster.PatternType,
					"cluster_size", cluster.Count,
					"error", err)
			}
		}
	}

	response := ScanResponse{
		Clusters: clusters,
		Count:    len(clusters),
		DryRun:   req.DryRun,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// scanRefinementsDryRun handles GET /api/v1/refinements/scan
func (h *refinementHandler) scanRefinementsDryRun(w http.ResponseWriter, r *http.Request) {
	req := ScanRequest{DryRun: true}

	// Parse query parameters
	if since := r.URL.Query().Get("since"); since != "" {
		req.Since = &since
	}

	if soulSlug := r.URL.Query().Get("soul_slug"); soulSlug != "" {
		req.SOULSlug = &soulSlug
	}

	if thresholdStr := r.URL.Query().Get("threshold"); thresholdStr != "" {
		threshold, err := strconv.ParseFloat(thresholdStr, 64)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"invalid threshold: %v"}`, err), http.StatusBadRequest)
			return
		}
		req.Threshold = &threshold
	}

	clusters, err := h.performScan(r.Context(), &req)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"scan failed: %v"}`, err), http.StatusInternalServerError)
		return
	}

	response := ScanResponse{
		Clusters: clusters,
		Count:    len(clusters),
		DryRun:   true,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// performScan executes the pattern detection and clustering logic
func (h *refinementHandler) performScan(ctx context.Context, req *ScanRequest) ([]refinement.PatternCluster, error) {
	detector := refinement.NewDetector(h.store)

	var since *time.Time
	if req.Since != nil {
		t, err := time.Parse(time.RFC3339, *req.Since)
		if err != nil {
			return nil, fmt.Errorf("invalid since timestamp: %w", err)
		}
		since = &t
	}

	threshold := 0.85 // Default threshold
	if req.Threshold != nil {
		threshold = *req.Threshold
	}

	return detector.FindClusters(ctx, since, threshold)
}
