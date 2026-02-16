package dedup

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DeduResult represents the result of a deduplication operation.
type DeduResult struct {
	Table       string              `json:"table"`
	Threshold   float64            `json:"threshold"`
	Execute     bool               `json:"execute"`
	Clusters    int                `json:"clusters"`
	TotalItems  int                `json:"total_items"`
	Deduped     int                `json:"deduped"`
	Survivors   int                `json:"survivors"`
	Details     []ClusterDetail    `json:"details,omitempty"`
}

// ClusterDetail provides information about a specific duplicate cluster.
type ClusterDetail struct {
	SurvivorID uuid.UUID   `json:"survivor_id"`
	DedupedIDs []uuid.UUID `json:"deduped_ids"`
	Size       int         `json:"size"`
}

// Deduplicator orchestrates the deduplication process.
type Deduplicator struct {
	pool    *pgxpool.Pool
	scanner *Scanner
	ranker  *Ranker
	logger  *slog.Logger
}

// New creates a new deduplicator instance.
func New(pool *pgxpool.Pool, logger *slog.Logger) *Deduplicator {
	return &Deduplicator{
		pool:    pool,
		scanner: NewScanner(pool),
		ranker:  NewRanker(pool),
		logger:  logger,
	}
}

// DeduplicateReasoningPatterns performs deduplication on reasoning patterns.
func (d *Deduplicator) DeduplicateReasoningPatterns(ctx context.Context, threshold float64, execute bool) (*DeduResult, error) {
	d.logger.Info("starting reasoning patterns deduplication", "threshold", threshold, "execute", execute)

	// Find duplicate pairs
	pairs, err := d.scanner.FindReasoningPatternDuplicates(ctx, threshold)
	if err != nil {
		return nil, fmt.Errorf("find duplicates: %w", err)
	}

	d.logger.Info("found duplicate pairs", "count", len(pairs))

	if len(pairs) == 0 {
		return &DeduResult{
			Table:     "reasoning_patterns",
			Threshold: threshold,
			Execute:   execute,
			Clusters:  0,
			Survivors: 0,
			Deduped:   0,
		}, nil
	}

	// Cluster duplicates using union-find
	clusters := d.clusterPairs(pairs)
	d.logger.Info("clustered duplicates", "clusters", len(clusters))

	result := &DeduResult{
		Table:     "reasoning_patterns",
		Threshold: threshold,
		Execute:   execute,
		Clusters:  len(clusters),
	}

	var allSurvivors []uuid.UUID
	var allDeduped []uuid.UUID

	// Process each cluster
	for _, cluster := range clusters {
		result.TotalItems += len(cluster)

		// Rank to find survivor
		survivorID, err := d.ranker.RankReasoningPatterns(ctx, cluster)
		if err != nil {
			d.logger.Error("failed to rank cluster", "cluster", cluster, "error", err)
			continue
		}

		// Identify items to be deduped
		var dedupedIDs []uuid.UUID
		for _, id := range cluster {
			if id != survivorID {
				dedupedIDs = append(dedupedIDs, id)
			}
		}

		allSurvivors = append(allSurvivors, survivorID)
		allDeduped = append(allDeduped, dedupedIDs...)

		if execute {
			// Update deduped items
			if err := d.markReasoningPatternsAsDeduped(ctx, dedupedIDs, survivorID); err != nil {
				d.logger.Error("failed to mark items as deduped", "survivor", survivorID, "deduped", dedupedIDs, "error", err)
				continue
			}
		}

		// Add details
		result.Details = append(result.Details, ClusterDetail{
			SurvivorID: survivorID,
			DedupedIDs: dedupedIDs,
			Size:       len(cluster),
		})
	}

	result.Survivors = len(allSurvivors)
	result.Deduped = len(allDeduped)

	d.logger.Info("deduplication completed", "survivors", result.Survivors, "deduped", result.Deduped)
	return result, nil
}

// DeduplicateDecisions performs deduplication on decisions.
func (d *Deduplicator) DeduplicateDecisions(ctx context.Context, threshold float64, execute bool) (*DeduResult, error) {
	d.logger.Info("starting decisions deduplication", "threshold", threshold, "execute", execute)

	// Find duplicate pairs
	pairs, err := d.scanner.FindDecisionDuplicates(ctx, threshold)
	if err != nil {
		return nil, fmt.Errorf("find duplicates: %w", err)
	}

	d.logger.Info("found duplicate pairs", "count", len(pairs))

	if len(pairs) == 0 {
		return &DeduResult{
			Table:     "decisions",
			Threshold: threshold,
			Execute:   execute,
			Clusters:  0,
			Survivors: 0,
			Deduped:   0,
		}, nil
	}

	// Cluster duplicates using union-find
	clusters := d.clusterPairs(pairs)
	d.logger.Info("clustered duplicates", "clusters", len(clusters))

	result := &DeduResult{
		Table:     "decisions",
		Threshold: threshold,
		Execute:   execute,
		Clusters:  len(clusters),
	}

	var allSurvivors []uuid.UUID
	var allDeduped []uuid.UUID

	// Process each cluster
	for _, cluster := range clusters {
		result.TotalItems += len(cluster)

		// Rank to find survivor
		survivorID, err := d.ranker.RankDecisions(ctx, cluster)
		if err != nil {
			d.logger.Error("failed to rank cluster", "cluster", cluster, "error", err)
			continue
		}

		// Identify items to be deduped
		var dedupedIDs []uuid.UUID
		for _, id := range cluster {
			if id != survivorID {
				dedupedIDs = append(dedupedIDs, id)
			}
		}

		allSurvivors = append(allSurvivors, survivorID)
		allDeduped = append(allDeduped, dedupedIDs...)

		if execute {
			// Update deduped items
			if err := d.markDecisionsAsDeduped(ctx, dedupedIDs, survivorID); err != nil {
				d.logger.Error("failed to mark items as deduped", "survivor", survivorID, "deduped", dedupedIDs, "error", err)
				continue
			}
		}

		// Add details
		result.Details = append(result.Details, ClusterDetail{
			SurvivorID: survivorID,
			DedupedIDs: dedupedIDs,
			Size:       len(cluster),
		})
	}

	result.Survivors = len(allSurvivors)
	result.Deduped = len(allDeduped)

	d.logger.Info("deduplication completed", "survivors", result.Survivors, "deduped", result.Deduped)
	return result, nil
}

// clusterPairs groups duplicate pairs into connected components using union-find.
func (d *Deduplicator) clusterPairs(pairs []DuplicatePair) [][]uuid.UUID {
	if len(pairs) == 0 {
		return nil
	}

	// Build union-find data structure
	parent := make(map[uuid.UUID]uuid.UUID)
	
	// Initialize each ID as its own parent
	for _, pair := range pairs {
		if _, exists := parent[pair.ID1]; !exists {
			parent[pair.ID1] = pair.ID1
		}
		if _, exists := parent[pair.ID2]; !exists {
			parent[pair.ID2] = pair.ID2
		}
	}

	// Find function with path compression
	var find func(uuid.UUID) uuid.UUID
	find = func(id uuid.UUID) uuid.UUID {
		if parent[id] != id {
			parent[id] = find(parent[id]) // Path compression
		}
		return parent[id]
	}

	// Union function
	union := func(id1, id2 uuid.UUID) {
		root1 := find(id1)
		root2 := find(id2)
		if root1 != root2 {
			parent[root2] = root1
		}
	}

	// Union all pairs
	for _, pair := range pairs {
		union(pair.ID1, pair.ID2)
	}

	// Group IDs by their root
	groups := make(map[uuid.UUID][]uuid.UUID)
	for id := range parent {
		root := find(id)
		groups[root] = append(groups[root], id)
	}

	// Convert to slice of clusters
	var clusters [][]uuid.UUID
	for _, cluster := range groups {
		if len(cluster) > 1 { // Only include actual clusters, not singletons
			clusters = append(clusters, cluster)
		}
	}

	return clusters
}

// markReasoningPatternsAsDeduped updates reasoning patterns as deduped.
func (d *Deduplicator) markReasoningPatternsAsDeduped(ctx context.Context, dedupedIDs []uuid.UUID, survivorID uuid.UUID) error {
	if len(dedupedIDs) == 0 {
		return nil
	}

	query := `
		UPDATE reasoning_patterns 
		SET deduped_at = now(), dedup_survivor_id = $1 
		WHERE id = ANY($2)`

	_, err := d.pool.Exec(ctx, query, survivorID, dedupedIDs)
	if err != nil {
		return fmt.Errorf("update reasoning patterns: %w", err)
	}

	return nil
}

// markDecisionsAsDeduped updates decisions as deduped.
func (d *Deduplicator) markDecisionsAsDeduped(ctx context.Context, dedupedIDs []uuid.UUID, survivorID uuid.UUID) error {
	if len(dedupedIDs) == 0 {
		return nil
	}

	query := `
		UPDATE decisions 
		SET deduped_at = now(), dedup_survivor_id = $1 
		WHERE id = ANY($2)`

	_, err := d.pool.Exec(ctx, query, survivorID, dedupedIDs)
	if err != nil {
		return fmt.Errorf("update decisions: %w", err)
	}

	return nil
}
