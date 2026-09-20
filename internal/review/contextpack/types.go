// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package contextpack

import (
	"time"

	"github.com/google/uuid"
)

// ContextLayerKind classifies the role and source of a context layer.
type ContextLayerKind string

const (
	LayerDiff           ContextLayerKind = "diff"
	LayerTraceDecision  ContextLayerKind = "trace_decision"
	LayerTicket         ContextLayerKind = "ticket"
	LayerKnowledgeGraph ContextLayerKind = "knowledge_graph"
	LayerRules          ContextLayerKind = "rules"
	LayerConventions    ContextLayerKind = "conventions"
)

// ContextLayer represents a distinct, prioritized slice of contextual information.
type ContextLayer struct {
	Kind             ContextLayerKind `json:"kind"`
	Name             string           `json:"name"`
	Content          string           `json:"content"`
	EstimatedTokens  int              `json:"estimated_tokens"`
	Priority         int              `json:"priority"` // Higher value = higher priority (retained first)
	DroppedForBudget bool             `json:"dropped_for_budget"`
	Truncated        bool             `json:"truncated"`
}

// TicketContext holds project management ticket requirements (Jira, Linear, GitHub).
type TicketContext struct {
	IssueKey           string   `json:"issue_key"`
	Provider           string   `json:"provider"` // "jira", "linear", "github"
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	AcceptanceCriteria []string `json:"acceptance_criteria,omitempty"`
	Status             string   `json:"status"`
	Priority           string   `json:"priority,omitempty"`
}

// TraceDecision represents an Architectural Decision Record (ADR) tracked by ScanDrix Trace.
type TraceDecision struct {
	ID          uuid.UUID `json:"id"`
	DecisionKey string    `json:"decision_key"` // e.g. "ADR-0042"
	Title       string    `json:"title"`
	Summary     string    `json:"summary"`
	Rationale   string    `json:"rationale"`
	Constraints []string  `json:"constraints,omitempty"`
	Files       []string  `json:"files"`
	Tags        []string  `json:"tags,omitempty"`
}

// ContextBudgetConfig defines token budget allocation per context layer.
type ContextBudgetConfig struct {
	TotalBudgetTokens int                         `json:"total_budget_tokens"`
	LayerBudgets      map[ContextLayerKind]int    `json:"layer_budgets"`
}

// DefaultContextBudgetConfig returns standard production token budget allocations.
func DefaultContextBudgetConfig() ContextBudgetConfig {
	return ContextBudgetConfig{
		TotalBudgetTokens: 48000,
		LayerBudgets: map[ContextLayerKind]int{
			LayerDiff:           30000,
			LayerTraceDecision:  6000,
			LayerTicket:         4000,
			LayerKnowledgeGraph: 5000,
			LayerRules:          5000,
			LayerConventions:    3000,
		},
	}
}

// ContextPack represents the assembled and budget-constrained context payload.
type ContextPack struct {
	Layers             []ContextLayer `json:"layers"`
	TotalTokens        int            `json:"total_tokens"`
	DroppedLayersCount int            `json:"dropped_layers_count"`
	AssembledPrompt    string         `json:"assembled_prompt"`
	CreatedAt          time.Time      `json:"created_at"`
}
