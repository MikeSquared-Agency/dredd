package refinement

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/MikeSquared-Agency/dredd/internal/store"
)

// PatternCluster represents a group of similar patterns
type PatternCluster struct {
	PatternType string            `json:"pattern_type"`
	Count       int               `json:"count"`
	Summary     string            `json:"summary"`
	SOULSection string            `json:"soul_section"`
	Patterns    []ClusterPattern  `json:"patterns"`
}

// ClusterPattern represents a single pattern within a cluster
type ClusterPattern struct {
	ID          string    `json:"id"`
	Summary     string    `json:"summary"`
	Confidence  float64   `json:"confidence"`
	CreatedAt   time.Time `json:"created_at"`
}

// patternRecord is an internal type for processing database records
type patternRecord struct {
	ID          string
	PatternType string
	Summary     string
	Confidence  float64
	CreatedAt   time.Time
	Embedding   []float64
}

// Detector finds and clusters confirmed reasoning patterns for SOUL refinement
type Detector struct {
	store *store.Store
}

// NewDetector creates a new pattern detector
func NewDetector(store *store.Store) *Detector {
	return &Detector{store: store}
}

// FindClusters finds confirmed patterns and groups them by embedding similarity
func (d *Detector) FindClusters(ctx context.Context, since *time.Time, threshold float64) ([]PatternCluster, error) {
	if threshold <= 0 || threshold > 1 {
		threshold = 0.85 // Default cosine similarity threshold
	}

	// Build query with optional time filter
	query := `
		SELECT 
			id::text,
			pattern_type,
			summary,
			dredd_confidence,
			created_at,
			arc_embedding
		FROM reasoning_patterns 
		WHERE pattern_type IN ('correction', 'pushback', 'reframing')
		AND review_status = 'confirmed'
		AND dredd_confidence > 0.8`
	
	args := []interface{}{}
	if since != nil {
		query += " AND created_at >= $1"
		args = append(args, *since)
	}
	
	query += " ORDER BY created_at DESC"

	rows, err := d.store.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query confirmed patterns: %w", err)
	}
	defer rows.Close()

	// Collect all patterns
	var patterns []patternRecord
	for rows.Next() {
		var p patternRecord
		var embeddingStr string
		
		err := rows.Scan(&p.ID, &p.PatternType, &p.Summary, &p.Confidence, &p.CreatedAt, &embeddingStr)
		if err != nil {
			return nil, fmt.Errorf("scan pattern row: %w", err)
		}
		
		// Parse embedding - simple parser for pgvector format [0.1,0.2,0.3]
		if embeddingStr != "" {
			embedding, parseErr := parsePgVector(embeddingStr)
			if parseErr != nil {
				// Skip patterns with invalid embeddings
				continue
			}
			p.Embedding = embedding
		}
		
		patterns = append(patterns, p)
	}
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pattern rows: %w", err)
	}

	// Group patterns by type and similarity
	clusters := d.clusterPatterns(patterns, threshold)
	
	// Map to SOUL sections
	mapper := NewMapper()
	for i := range clusters {
		sections := mapper.MapPatternToSections(clusters[i].PatternType)
		if len(sections) > 0 {
			clusters[i].SOULSection = sections[0] // Use first section for now
		}
	}

	return clusters, nil
}

// clusterPatterns groups patterns by type and embedding similarity
func (d *Detector) clusterPatterns(patterns []patternRecord, threshold float64) []PatternCluster {
	// Group by pattern type first
	typeGroups := make(map[string][]patternRecord)
	for _, p := range patterns {
		typeGroups[p.PatternType] = append(typeGroups[p.PatternType], p)
	}

	var clusters []PatternCluster
	
	for patternType, typePatterns := range typeGroups {
		// For each type, cluster by embedding similarity
		typeClusters := d.clusterByEmbedding(typePatterns, threshold)
		
		for _, cluster := range typeClusters {
			if len(cluster) == 0 {
				continue
			}
			
			// Use the first (most recent) pattern's summary as cluster summary
			clusterSummary := cluster[0].Summary
			
			// Convert to cluster format
			clusterPatterns := make([]ClusterPattern, len(cluster))
			for i, p := range cluster {
				clusterPatterns[i] = ClusterPattern{
					ID:         p.ID,
					Summary:    p.Summary,
					Confidence: p.Confidence,
					CreatedAt:  p.CreatedAt,
				}
			}
			
			clusters = append(clusters, PatternCluster{
				PatternType: patternType,
				Count:       len(cluster),
				Summary:     clusterSummary,
				Patterns:    clusterPatterns,
			})
		}
	}

	return clusters
}

// clusterByEmbedding clusters patterns by cosine similarity of embeddings
func (d *Detector) clusterByEmbedding(patterns []patternRecord, threshold float64) [][]patternRecord {
	if len(patterns) == 0 {
		return nil
	}
	
	// Simple clustering: for each pattern, find others within threshold
	// This is a basic approach - could be improved with proper clustering algorithms
	var clusters [][]patternRecord
	used := make(map[string]bool)
	
	for _, p := range patterns {
		if used[p.ID] {
			continue
		}
		
		cluster := []patternRecord{p}
		used[p.ID] = true
		
		// Find similar patterns
		for _, other := range patterns {
			if used[other.ID] || other.ID == p.ID {
				continue
			}
			
			similarity := cosineSimilarity(p.Embedding, other.Embedding)
			if similarity >= threshold {
				cluster = append(cluster, other)
				used[other.ID] = true
			}
		}
		
		clusters = append(clusters, cluster)
	}
	
	return clusters
}

// Utility functions

// parsePgVector parses a pgvector string like "[0.1,0.2,0.3]" into []float64
func parsePgVector(s string) ([]float64, error) {
	if len(s) < 2 || s[0] != '[' || s[len(s)-1] != ']' {
		return nil, fmt.Errorf("invalid vector format")
	}
	
	s = s[1 : len(s)-1] // Remove brackets
	if s == "" {
		return []float64{}, nil
	}
	
	parts := strings.Split(s, ",")
	result := make([]float64, len(parts))
	
	for i, part := range parts {
		val, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return nil, fmt.Errorf("parse float %q: %w", part, err)
		}
		result[i] = val
	}
	
	return result, nil
}

// cosineSimilarity calculates cosine similarity between two vectors
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}
	
	var dotProduct, normA, normB float64
	
	for i := 0; i < len(a); i++ {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	
	if normA == 0.0 || normB == 0.0 {
		return 0.0
	}
	
	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}
