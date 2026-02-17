package processor

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/MikeSquared-Agency/dredd/internal/extractor"
)

// InteractionEvent matches the slack-gateway interaction event format.
type InteractionEvent struct {
	ActionID  string `json:"action_id"`
	Value     string `json:"value"`
	UserID    string `json:"user_id"`
	UserName  string `json:"user_name"`
	ChannelID string `json:"channel_id"`
	MessageTS string `json:"message_ts"`
	TriggerID string `json:"trigger_id"`
}

type gateMetadata struct {
	ItemID string `json:"item_id"`
	Stage  string `json:"stage"`
}

// HandleGateDecision processes gate approval/rejection interactions from Slack.
func (p *Processor) HandleGateDecision(subject string, data []byte) {
	var evt InteractionEvent
	if err := json.Unmarshal(data, &evt); err != nil {
		p.logger.Warn("failed to parse interaction event", "error", err)
		return
	}

	// Only process gate actions
	var decisionType string
	var itemID string
	switch {
	case strings.HasPrefix(evt.ActionID, "gate_approve:"):
		decisionType = "approved"
		itemID = strings.TrimPrefix(evt.ActionID, "gate_approve:")
	case strings.HasPrefix(evt.ActionID, "gate_changes:"):
		decisionType = "changes_requested"
		itemID = strings.TrimPrefix(evt.ActionID, "gate_changes:")
	case strings.HasPrefix(evt.ActionID, "gate_block:"):
		decisionType = "blocked"
		itemID = strings.TrimPrefix(evt.ActionID, "gate_block:")
	default:
		return // not a gate action, ignore
	}

	// Parse metadata from Value
	var meta gateMetadata
	if err := json.Unmarshal([]byte(evt.Value), &meta); err != nil {
		p.logger.Warn("failed to parse gate metadata", "error", err, "value", evt.Value)
		// Use itemID from action_id
		meta.ItemID = itemID
		meta.Stage = "unknown"
	}

	severity := "routine"
	if decisionType == "changes_requested" {
		severity = "significant"
	} else if decisionType == "blocked" {
		severity = "critical"
	}

	summary := "Gate " + decisionType + ": item " + itemID[:8] + " stage " + meta.Stage
	if evt.UserName != "" {
		summary += " by " + evt.UserName
	}

	ep := extractor.DecisionEpisode{
		Domain:        "gate",
		Category:      meta.Stage,
		Severity:      severity,
		Summary:       summary,
		SituationText: "Gate review for backlog item " + meta.ItemID + " at stage " + meta.Stage,
		Options: []extractor.DecisionOption{
			{OptionKey: "approve", ProSignals: []string{"evidence meets criteria"}, WasChosen: decisionType == "approved"},
			{OptionKey: "request_changes", ProSignals: []string{"evidence insufficient or incorrect"}, WasChosen: decisionType == "changes_requested"},
		},
		Reasoning: extractor.DecisionReasoning{
			Factors: []string{decisionType},
			ReasoningText: "Human reviewer " + evt.UserName + " decided: " + decisionType,
		},
		Tags:       []string{"gate", meta.Stage, decisionType},
		Confidence: 1.0, // human decision = full confidence
		SignalType: "gate_" + decisionType,
	}

	ctx := context.Background()
	ownerUUID := uuid.Nil // system-level decision

	id, err := p.store.WriteDecisionEpisode(ctx, ownerUUID, meta.ItemID, "slack-gateway", ep)
	if err != nil {
		p.logger.Error("failed to store gate decision",
			"error", err,
			"item_id", itemID,
			"stage", meta.Stage,
			"type", decisionType,
		)
		return
	}

	p.logger.Info("gate decision captured",
		"decision_id", id,
		"item_id", itemID[:8],
		"stage", meta.Stage,
		"type", decisionType,
		"user", evt.UserName,
	)
}


// GateEvidenceEvent matches the Dispatch gate evidence NATS event.
type GateEvidenceEvent struct {
	ItemID          string `json:"item_id"`
	ItemTitle       string `json:"item_title"`
	Stage           string `json:"stage"`
	Criterion       string `json:"criterion"`
	Evidence        string `json:"evidence"`
	SubmittedBy     string `json:"submitted_by"`
	AgentID         string `json:"agent_id"`
	PromptVersionID string `json:"prompt_version_id,omitempty"`
}

// HandleGateEvidence captures evidence submissions and logs prompt version attribution.
func (p *Processor) HandleGateEvidence(subject string, data []byte) {
	var evt GateEvidenceEvent
	if err := json.Unmarshal(data, &evt); err != nil {
		p.logger.Warn("failed to parse gate evidence event", "error", err)
		return
	}

	// Only log if we have version attribution
	if evt.PromptVersionID == "" {
		return
	}

	p.logger.Info("gate evidence with version attribution",
		"item_id", evt.ItemID[:8],
		"stage", evt.Stage,
		"criterion", evt.Criterion,
		"prompt_version_id", evt.PromptVersionID,
		"agent", evt.AgentID,
	)

	// Store as a decision episode tagged with the prompt version
	ep := extractor.DecisionEpisode{
		Domain:        "gate_evidence",
		Category:      evt.Stage,
		Severity:      "routine",
		Summary:       "Evidence submitted for " + evt.ItemID[:8] + " stage " + evt.Stage + " criterion " + evt.Criterion,
		SituationText: "Evidence: " + evt.Evidence,
		Reasoning: extractor.DecisionReasoning{
			ReasoningText: "Agent " + evt.AgentID + " submitted evidence using prompt version " + evt.PromptVersionID,
			Factors:       []string{"prompt_version:" + evt.PromptVersionID},
		},
		Tags:       []string{"gate_evidence", evt.Stage, "version:" + evt.PromptVersionID},
		Confidence: 1.0,
		ModelID:    evt.PromptVersionID,
	}

	ctx := context.Background()
	_, err := p.store.WriteDecisionEpisode(ctx, uuid.Nil, evt.ItemID, "dispatch", ep)
	if err != nil {
		p.logger.Error("failed to store versioned evidence",
			"error", err,
			"item_id", evt.ItemID[:8],
			"prompt_version_id", evt.PromptVersionID,
		)
	}
}
