//go:build integration

package store

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/MikeSquared-Agency/dredd/internal/extractor"
)

func setupTestStore(t *testing.T) *Store {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()
	s, err := New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	t.Cleanup(func() {
		s.Close()
	})
	return s
}

func TestIntegration_WriteAndUpdatePattern(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	ownerUUID := uuid.New()
	sessionRef := "integration-test-" + uuid.New().String()[:8]

	// Write a reasoning pattern
	pat := extractor.ReasoningPattern{
		PatternType:     "pushback",
		Summary:         "Integration test pattern",
		ConversationArc: "Mike: Stop doing that\nAgent: OK",
		Tags:            []string{"test", "integration"},
		Confidence:      0.88,
	}

	id, err := s.WriteReasoningPattern(ctx, ownerUUID, sessionRef, pat)
	if err != nil {
		t.Fatalf("WriteReasoningPattern failed: %v", err)
	}
	if id == uuid.Nil {
		t.Fatal("expected non-nil pattern ID")
	}

	// Fetch it back
	row, err := s.GetPatternByID(ctx, id)
	if err != nil {
		t.Fatalf("GetPatternByID failed: %v", err)
	}
	if row.PatternType != "pushback" {
		t.Errorf("expected pattern_type pushback, got %q", row.PatternType)
	}
	if row.Summary != "Integration test pattern" {
		t.Errorf("expected summary, got %q", row.Summary)
	}
	if row.Confidence != 0.88 {
		t.Errorf("expected confidence 0.88, got %f", row.Confidence)
	}
	if row.ReviewStatus != "pending" {
		t.Errorf("expected review_status pending, got %q", row.ReviewStatus)
	}

	// Update review status
	err = s.UpdatePatternReviewStatus(ctx, id, "confirmed", "looks correct")
	if err != nil {
		t.Fatalf("UpdatePatternReviewStatus failed: %v", err)
	}

	// Verify update
	row, err = s.GetPatternByID(ctx, id)
	if err != nil {
		t.Fatalf("GetPatternByID after update failed: %v", err)
	}
	if row.ReviewStatus != "confirmed" {
		t.Errorf("expected review_status confirmed, got %q", row.ReviewStatus)
	}

	// Cleanup
	t.Cleanup(func() {
		s.pool.Exec(ctx, "DELETE FROM reasoning_patterns WHERE id = $1", id)
	})
}

func TestIntegration_WriteDecisionEpisode(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	ownerUUID := uuid.New()
	sessionRef := "integration-test-" + uuid.New().String()[:8]

	ep := extractor.DecisionEpisode{
		Domain:        "architecture",
		Category:      "gate_approval",
		Severity:      "routine",
		Summary:       "Integration test decision",
		SituationText: "Testing the decision engine write path",
		Options: []extractor.DecisionOption{
			{
				OptionKey:  "approve",
				ProSignals: []string{"tests pass"},
				ConSignals: []string{"no review"},
				WasChosen:  true,
			},
			{
				OptionKey:  "reject",
				ProSignals: []string{"more review"},
				ConSignals: []string{"delays"},
				WasChosen:  false,
			},
		},
		Reasoning: extractor.DecisionReasoning{
			Factors:       []string{"tests passing", "low risk"},
			Tradeoffs:     []string{"speed vs review"},
			ReasoningText: "Tests pass and this is low risk, approve",
		},
		Tags:       []string{"test", "integration"},
		Confidence: 0.95,
	}

	id, err := s.WriteDecisionEpisode(ctx, ownerUUID, sessionRef, "dredd", ep)
	if err != nil {
		t.Fatalf("WriteDecisionEpisode failed: %v", err)
	}
	if id == uuid.Nil {
		t.Fatal("expected non-nil decision ID")
	}

	// Verify decision was written
	var summary string
	err = s.pool.QueryRow(ctx, "SELECT summary FROM decisions WHERE id = $1", id).Scan(&summary)
	if err != nil {
		t.Fatalf("query decision failed: %v", err)
	}
	if summary != "Integration test decision" {
		t.Errorf("expected summary, got %q", summary)
	}

	// Verify options were written
	var optCount int
	err = s.pool.QueryRow(ctx, "SELECT count(*) FROM decision_options WHERE decision_id = $1", id).Scan(&optCount)
	if err != nil {
		t.Fatalf("query options failed: %v", err)
	}
	if optCount != 2 {
		t.Errorf("expected 2 options, got %d", optCount)
	}

	// Verify tags were written
	var tagCount int
	err = s.pool.QueryRow(ctx, "SELECT count(*) FROM decision_tags WHERE decision_id = $1", id).Scan(&tagCount)
	if err != nil {
		t.Fatalf("query tags failed: %v", err)
	}
	if tagCount != 2 {
		t.Errorf("expected 2 tags, got %d", tagCount)
	}

	// Cleanup
	t.Cleanup(func() {
		s.pool.Exec(ctx, "DELETE FROM decisions WHERE id = $1", id)
	})
}

func TestIntegration_UpsertTrust(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	agentID := "test-agent-" + uuid.New().String()[:8]

	// Create trust record
	err := s.UpsertTrust(ctx, agentID, "gate_approval", "routine", 0.5, 10, 8, 0)
	if err != nil {
		t.Fatalf("UpsertTrust (create) failed: %v", err)
	}

	// Read it back
	rec, err := s.GetTrust(ctx, agentID, "gate_approval", "routine")
	if err != nil {
		t.Fatalf("GetTrust failed: %v", err)
	}
	if rec.TrustScore != 0.5 {
		t.Errorf("expected score 0.5, got %f", rec.TrustScore)
	}
	if rec.TotalDecisions != 10 {
		t.Errorf("expected 10 total, got %d", rec.TotalDecisions)
	}

	// Update (upsert)
	err = s.UpsertTrust(ctx, agentID, "gate_approval", "routine", 0.55, 11, 9, 0)
	if err != nil {
		t.Fatalf("UpsertTrust (update) failed: %v", err)
	}

	rec, err = s.GetTrust(ctx, agentID, "gate_approval", "routine")
	if err != nil {
		t.Fatalf("GetTrust after update failed: %v", err)
	}
	if rec.TrustScore != 0.55 {
		t.Errorf("expected score 0.55, got %f", rec.TrustScore)
	}
	if rec.TotalDecisions != 11 {
		t.Errorf("expected 11 total, got %d", rec.TotalDecisions)
	}

	// Cleanup
	t.Cleanup(func() {
		s.pool.Exec(ctx, "DELETE FROM agent_trust WHERE agent_id = $1", agentID)
	})
}
