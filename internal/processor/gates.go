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
