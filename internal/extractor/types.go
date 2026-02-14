package extractor

import "github.com/google/uuid"

// TranscriptEvent is the NATS event payload from Chronicle.
type TranscriptEvent struct {
	SessionID  string `json:"session_id"`
	OwnerUUID  string `json:"owner_uuid"`
	SessionRef string `json:"session_ref"`
	Title      string `json:"title"`
	Duration   string `json:"duration"`
	Surface    string `json:"surface"` // e.g. "cc", "slack", "web"
}

// ExtractionResult holds all extractions from a single transcript.
type ExtractionResult struct {
	SessionRef string
	OwnerUUID  uuid.UUID
	Decisions  []DecisionEpisode
	Patterns   []ReasoningPattern
}

// DecisionEpisode is a Type 1 extraction — a directive decision.
type DecisionEpisode struct {
	Domain       string          `json:"domain"`
	Category     string          `json:"category"`
	Severity     string          `json:"severity"` // routine | significant | critical
	Summary      string          `json:"summary"`
	SituationText string         `json:"situation_text"`
	Options      []DecisionOption `json:"options"`
	Reasoning    DecisionReasoning `json:"reasoning"`
	Tags         []string        `json:"tags"`
	Confidence   float64         `json:"confidence"`
	AgentID      string          `json:"agent_id,omitempty"` // if decision was about an agent's action
	SignalType   string          `json:"signal_type,omitempty"` // reassignment, budget_correction, etc.
}

// DecisionOption represents an alternative that was considered.
type DecisionOption struct {
	OptionKey  string   `json:"option_key"`
	ProSignals []string `json:"pro_signals"`
	ConSignals []string `json:"con_signals"`
	WasChosen  bool     `json:"was_chosen"`
}

// DecisionReasoning captures why a decision was made.
type DecisionReasoning struct {
	Factors       []string `json:"factors"`
	Tradeoffs     []string `json:"tradeoffs"`
	ReasoningText string   `json:"reasoning_text"`
}

// ReasoningPattern is a Type 2 extraction — a thinking pattern.
type ReasoningPattern struct {
	PatternType     string   `json:"pattern_type"` // reframing | correction | philosophy | direction | pushback
	Summary         string   `json:"summary"`
	ConversationArc string   `json:"conversation_arc"`
	Tags            []string `json:"tags"`
	Confidence      float64  `json:"confidence"`
}
