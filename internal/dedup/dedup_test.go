package dedup

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// Test union-find clustering logic
func TestClusterPairs(t *testing.T) {
	d := &Deduplicator{}
	
	// Test empty pairs
	clusters := d.clusterPairs([]DuplicatePair{})
	if len(clusters) != 0 {
		t.Errorf("expected 0 clusters for empty pairs, got %d", len(clusters))
	}

	// Test single pair
	id1, id2 := uuid.New(), uuid.New()
	pairs := []DuplicatePair{
		{ID1: id1, ID2: id2, Similarity: 0.95},
	}
	clusters = d.clusterPairs(pairs)
	if len(clusters) != 1 {
		t.Errorf("expected 1 cluster, got %d", len(clusters))
	}
	if len(clusters[0]) != 2 {
		t.Errorf("expected cluster size 2, got %d", len(clusters[0]))
	}

	// Test connected components
	id3, id4, id5 := uuid.New(), uuid.New(), uuid.New()
	pairs = []DuplicatePair{
		{ID1: id1, ID2: id2, Similarity: 0.95},
		{ID1: id2, ID2: id3, Similarity: 0.93},  // connects id1-id2-id3
		{ID1: id4, ID2: id5, Similarity: 0.94},  // separate cluster
	}
	clusters = d.clusterPairs(pairs)
	if len(clusters) != 2 {
		t.Errorf("expected 2 clusters, got %d", len(clusters))
	}

	// Find the cluster sizes
	var sizes []int
	for _, cluster := range clusters {
		sizes = append(sizes, len(cluster))
	}
	
	// Should have one cluster of size 3 and one of size 2
	found3, found2 := false, false
	for _, size := range sizes {
		if size == 3 {
			found3 = true
		} else if size == 2 {
			found2 = true
		}
	}
	if !found3 || !found2 {
		t.Errorf("expected clusters of size 3 and 2, got sizes %v", sizes)
	}
}

// Test reasoning pattern ranking logic
func TestIsReasoningPatternBetter(t *testing.T) {
	baseTime := time.Now()
	
	tests := []struct {
		name     string
		a        ReasoningPatternRecord
		b        ReasoningPatternRecord
		expected bool
	}{
		{
			name: "confirmed beats pending",
			a: ReasoningPatternRecord{
				ReviewStatus: "confirmed",
				DreddConfidence: 0.5,
				CreatedAt: baseTime,
			},
			b: ReasoningPatternRecord{
				ReviewStatus: "pending",
				DreddConfidence: 0.9,
				CreatedAt: baseTime.Add(time.Hour),
			},
			expected: true,
		},
		{
			name: "pending beats rejected",
			a: ReasoningPatternRecord{
				ReviewStatus: "pending",
				DreddConfidence: 0.5,
				CreatedAt: baseTime,
			},
			b: ReasoningPatternRecord{
				ReviewStatus: "rejected",
				DreddConfidence: 0.9,
				CreatedAt: baseTime.Add(time.Hour),
			},
			expected: true,
		},
		{
			name: "higher confidence wins when status equal",
			a: ReasoningPatternRecord{
				ReviewStatus: "pending",
				DreddConfidence: 0.9,
				CreatedAt: baseTime,
			},
			b: ReasoningPatternRecord{
				ReviewStatus: "pending",
				DreddConfidence: 0.7,
				CreatedAt: baseTime.Add(time.Hour),
			},
			expected: true,
		},
		{
			name: "more recent wins when status and confidence equal",
			a: ReasoningPatternRecord{
				ReviewStatus: "pending",
				DreddConfidence: 0.8,
				CreatedAt: baseTime.Add(time.Hour),
			},
			b: ReasoningPatternRecord{
				ReviewStatus: "pending",
				DreddConfidence: 0.8,
				CreatedAt: baseTime,
			},
			expected: true,
		},
		{
			name: "lower priority loses",
			a: ReasoningPatternRecord{
				ReviewStatus: "rejected",
				DreddConfidence: 0.9,
				CreatedAt: baseTime.Add(time.Hour),
			},
			b: ReasoningPatternRecord{
				ReviewStatus: "confirmed",
				DreddConfidence: 0.5,
				CreatedAt: baseTime,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isReasoningPatternBetter(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("isReasoningPatternBetter() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// Test decision ranking logic
func TestIsDecisionBetter(t *testing.T) {
	baseTime := time.Now()
	
	tests := []struct {
		name     string
		a        DecisionRecord
		b        DecisionRecord
		expected bool
	}{
		{
			name: "confirmed beats pending",
			a: DecisionRecord{
				ReviewStatus: "confirmed",
				Severity: "low",
				CreatedAt: baseTime,
			},
			b: DecisionRecord{
				ReviewStatus: "pending",
				Severity: "critical",
				CreatedAt: baseTime.Add(time.Hour),
			},
			expected: true,
		},
		{
			name: "higher severity wins when status equal",
			a: DecisionRecord{
				ReviewStatus: "pending",
				Severity: "critical",
				CreatedAt: baseTime,
			},
			b: DecisionRecord{
				ReviewStatus: "pending",
				Severity: "high",
				CreatedAt: baseTime.Add(time.Hour),
			},
			expected: true,
		},
		{
			name: "more recent wins when status and severity equal",
			a: DecisionRecord{
				ReviewStatus: "pending",
				Severity: "medium",
				CreatedAt: baseTime.Add(time.Hour),
			},
			b: DecisionRecord{
				ReviewStatus: "pending",
				Severity: "medium",
				CreatedAt: baseTime,
			},
			expected: true,
		},
		{
			name: "severity priority order",
			a: DecisionRecord{
				ReviewStatus: "pending",
				Severity: "high",
				CreatedAt: baseTime,
			},
			b: DecisionRecord{
				ReviewStatus: "pending",
				Severity: "medium",
				CreatedAt: baseTime.Add(time.Hour),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isDecisionBetter(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("isDecisionBetter() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// Test priority functions
func TestReviewStatusPriority(t *testing.T) {
	tests := map[string]int{
		"confirmed": 3,
		"pending":   2,
		"rejected":  1,
		"unknown":   0,
	}

	for status, expected := range tests {
		result := reviewStatusPriority(status)
		if result != expected {
			t.Errorf("reviewStatusPriority(%q) = %d, expected %d", status, result, expected)
		}
	}
}

func TestSeverityPriority(t *testing.T) {
	tests := map[string]int{
		"critical": 4,
		"high":     3,
		"medium":   2,
		"low":      1,
		"unknown":  0,
	}

	for severity, expected := range tests {
		result := severityPriority(severity)
		if result != expected {
			t.Errorf("severityPriority(%q) = %d, expected %d", severity, result, expected)
		}
	}
}
