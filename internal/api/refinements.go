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

// RefinementServer extends the base server with refinement functionality
type RefinementServer struct {
	*Server
	hermes *hermes.Client
}

// ScanRequest represents the request payload for refinement scans
type ScanRequest struct {
	Since     *string `json:"since,omitempty"`     // ISO timestamp
	SOULSlug  *string `json:"soul_slug,omitempty"` // Target SOUL slug
	Threshold *float64 `json:"threshold,omitempty"` // Similarity threshold
	DryRun    bool     `json:"dry_run"`            // Don't publish, just return results
}

// ScanResponse represents the response from refinement scans
type ScanResponse struct {
	Clusters []refinement.PatternCluster `json:"clusters"`
	Count    int                         `json:"count"`
	DryRun   bool                        `json:"dry_run"`
}

// NewRefinementServer creates a server with refinement capabilities
func NewRefinementServer(port int, apiToken string, db *store.Store, hermes *hermes.Client) *RefinementServer {
	base := NewServer(port, apiToken, db)
	rs := &RefinementServer{
		Server: base,
		hermes: hermes,
	}
	
	// Add refinement routes
	base.router.Route("/api/v1/refinements", func(r chi.Router) {
		r.Use(BearerAuthMiddleware(apiToken))
		r.Post("/scan", rs.scanRefinements)
		r.Get("/scan", rs.scanRefinementsDryRun)
	})
	
	return rs
}

// scanRefinements handles POST /api/v1/refinements/scan
func (rs *RefinementServer) scanRefinements(w http.ResponseWriter, r *http.Request) {
	var req ScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"invalid JSON: %v"}`, err), http.StatusBadRequest)
		return
	}
	
	clusters, err := rs.performScan(r.Context(), &req)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"scan failed: %v"}`, err), http.StatusInternalServerError)
		return
	}
	
	// If not dry run, publish refinement proposals
	if !req.DryRun && len(clusters) > 0 {
		publisher := refinement.NewPublisher(rs.hermes)
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
func (rs *RefinementServer) scanRefinementsDryRun(w http.ResponseWriter, r *http.Request) {
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
	
	clusters, err := rs.performScan(r.Context(), &req)
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
func (rs *RefinementServer) performScan(ctx context.Context, req *ScanRequest) ([]refinement.PatternCluster, error) {
	detector := refinement.NewDetector(rs.store)
	
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
