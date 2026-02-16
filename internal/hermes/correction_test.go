package hermes

import (
	"encoding/json"
	"testing"
)

func TestCorrectionSignalParsing(t *testing.T) {
	raw := `{
		"session_ref": "sess-001",
		"decision_id": "dec-abc",
		"agent_id": "developer",
		"model_id": "claude-sonnet-4-5-20250929",
		"model_tier": "standard",
		"correction_type": "rejected",
		"category": "architecture",
		"severity": "significant"
	}`

	var signal CorrectionSignal
	err := json.Unmarshal([]byte(raw), &signal)
	if err != nil {
		t.Fatalf("failed to parse CorrectionSignal: %v", err)
	}

	if signal.SessionRef != "sess-001" {
		t.Errorf("expected session_ref 'sess-001', got '%s'", signal.SessionRef)
	}
	if signal.AgentID != "developer" {
		t.Errorf("expected agent_id 'developer', got '%s'", signal.AgentID)
	}
	if signal.CorrectionType != "rejected" {
		t.Errorf("expected correction_type 'rejected', got '%s'", signal.CorrectionType)
	}
	if signal.Category != "architecture" {
		t.Errorf("expected category 'architecture', got '%s'", signal.Category)
	}
	if signal.ModelTier != "standard" {
		t.Errorf("expected model_tier 'standard', got '%s'", signal.ModelTier)
	}
	if signal.Severity != "significant" {
		t.Errorf("expected severity 'significant', got '%s'", signal.Severity)
	}
}

func TestCorrectionSignalRoundTrip(t *testing.T) {
	signal := CorrectionSignal{
		SessionRef:     "sess-rt",
		DecisionID:     "dec-rt",
		AgentID:        "architect",
		ModelID:        "claude-opus-4-6",
		ModelTier:      "premium",
		CorrectionType: "confirmed",
		Category:       "security",
		Severity:       "critical",
	}

	data, err := json.Marshal(signal)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var parsed CorrectionSignal
	err = json.Unmarshal(data, &parsed)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if parsed != signal {
		t.Errorf("round-trip mismatch: got %+v, want %+v", parsed, signal)
	}
}

func TestSubjectCorrectionConstant(t *testing.T) {
	if SubjectCorrection != "swarm.dredd.correction" {
		t.Errorf("expected SubjectCorrection 'swarm.dredd.correction', got '%s'", SubjectCorrection)
	}
}
