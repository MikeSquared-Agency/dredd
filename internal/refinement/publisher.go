package refinement

import (
	"fmt"
	"time"

	"github.com/MikeSquared-Agency/dredd/internal/hermes"
)

// RefinementEvent represents a pattern refinement proposal
type RefinementEvent struct {
	Patterns        []PatternProposal `json:"patterns"`
	TargetSOULSlug  string            `json:"target_soul_slug"`
	TargetSection   string            `json:"target_section"`
	ProposedChange  string            `json:"proposed_change"`
	ClusterSize     int               `json:"cluster_size"`
	Timestamp       time.Time         `json:"timestamp"`
}

// PatternProposal represents a pattern within a refinement proposal
type PatternProposal struct {
	ID          string  `json:"id"`
	Summary     string  `json:"summary"`
	PatternType string  `json:"pattern_type"`
	Confidence  float64 `json:"confidence"`
}

// Publisher publishes pattern refinement events to NATS
type Publisher struct {
	hermes *hermes.Client
}

// NewPublisher creates a new refinement event publisher
func NewPublisher(hermes *hermes.Client) *Publisher {
	return &Publisher{hermes: hermes}
}

// PublishRefinementProposal publishes a pattern refinement proposal to NATS
func (p *Publisher) PublishRefinementProposal(cluster PatternCluster, targetSOULSlug string) error {
	// Convert cluster patterns to proposals
	proposals := make([]PatternProposal, len(cluster.Patterns))
	for i, pattern := range cluster.Patterns {
		proposals[i] = PatternProposal{
			ID:          pattern.ID,
			Summary:     pattern.Summary,
			PatternType: cluster.PatternType,
			Confidence:  pattern.Confidence,
		}
	}

	// Create refinement event
	event := RefinementEvent{
		Patterns:       proposals,
		TargetSOULSlug: targetSOULSlug,
		TargetSection:  cluster.SOULSection,
		ProposedChange: generateProposedChange(cluster),
		ClusterSize:    cluster.Count,
		Timestamp:      time.Now().UTC(),
	}

	// Publish to NATS
	subject := "pattern.refinement.proposed"
	return p.hermes.Publish(subject, event)
}

// generateProposedChange creates a human-readable description of the proposed change
func generateProposedChange(cluster PatternCluster) string {
	switch cluster.PatternType {
	case "correction":
		return fmt.Sprintf("Update %s based on %d correction patterns: %s", 
			cluster.SOULSection, cluster.Count, cluster.Summary)
	case "pushback":
		return fmt.Sprintf("Review %s for potential overreach based on %d pushback patterns: %s",
			cluster.SOULSection, cluster.Count, cluster.Summary)
	case "reframing":
		return fmt.Sprintf("Enhance %s with reframing insights from %d patterns: %s",
			cluster.SOULSection, cluster.Count, cluster.Summary)
	case "philosophy":
		return fmt.Sprintf("Refine philosophical stance in %s based on %d patterns: %s",
			cluster.SOULSection, cluster.Count, cluster.Summary)
	case "direction":
		return fmt.Sprintf("Adjust interaction modes in %s based on %d directional patterns: %s",
			cluster.SOULSection, cluster.Count, cluster.Summary)
	default:
		return fmt.Sprintf("Refine %s based on %d %s patterns: %s",
			cluster.SOULSection, cluster.Count, cluster.PatternType, cluster.Summary)
	}
}
