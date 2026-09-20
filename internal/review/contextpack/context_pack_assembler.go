// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package contextpack

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// AssemblerInput holds all raw contextual inputs available for a pull request.
type AssemblerInput struct {
	DiffContent          string           `json:"diff_content"`
	Tickets              []TicketContext  `json:"tickets,omitempty"`
	TraceDecisions       []TraceDecision  `json:"trace_decisions,omitempty"`
	KnowledgeGraphPrompt string           `json:"knowledge_graph_prompt,omitempty"`
	RulesSummary         string           `json:"rules_summary,omitempty"`
	ConventionDocs       []string         `json:"convention_docs,omitempty"`
}

// ContextPackAssembler coordinates layer creation, token budgeting, and prompt assembly.
type ContextPackAssembler struct {
	allocator *BudgetAllocator
}

// NewContextPackAssembler constructs an assembler.
func NewContextPackAssembler(allocator *BudgetAllocator) *ContextPackAssembler {
	if allocator == nil {
		allocator = NewBudgetAllocator()
	}
	return &ContextPackAssembler{allocator: allocator}
}

// Assemble constructs the multi-layer ContextPack subject to configured token limits.
func (a *ContextPackAssembler) Assemble(
	ctx context.Context,
	input AssemblerInput,
	config ContextBudgetConfig,
) (*ContextPack, error) {
	var layers []ContextLayer

	// 1. Layer: Unified Diff (Highest Priority = 100)
	if strings.TrimSpace(input.DiffContent) != "" {
		layers = append(layers, ContextLayer{
			Kind:     LayerDiff,
			Name:     "Unified Diff",
			Content:  input.DiffContent,
			Priority: 100,
		})
	}

	// 2. Layer: Trace Architectural Decisions (Priority = 90)
	if len(input.TraceDecisions) > 0 {
		var sb strings.Builder
		sb.WriteString("### 🏛️ ScanDrix Trace Architectural Decisions (ADRs)\n")
		sb.WriteString("The following architectural decisions have been recorded for the files touched in this PR:\n\n")
		for _, d := range input.TraceDecisions {
			sb.WriteString(fmt.Sprintf("#### [%s] %s\n", d.DecisionKey, d.Title))
			sb.WriteString(fmt.Sprintf("- **Summary**: %s\n", d.Summary))
			sb.WriteString(fmt.Sprintf("- **Rationale**: %s\n", d.Rationale))
			if len(d.Constraints) > 0 {
				sb.WriteString(fmt.Sprintf("- **Constraints**: %s\n", strings.Join(d.Constraints, "; ")))
			}
			sb.WriteString(fmt.Sprintf("- **Applies to Files**: `%s`\n\n", strings.Join(d.Files, "`, `")))
		}
		layers = append(layers, ContextLayer{
			Kind:     LayerTraceDecision,
			Name:     "Trace Architectural Decisions",
			Content:  sb.String(),
			Priority: 90,
		})
	}

	// 3. Layer: External Ticket Requirements (Priority = 80)
	if len(input.Tickets) > 0 {
		var sb strings.Builder
		sb.WriteString("### 🎫 Linked Ticket Requirements & Acceptance Criteria\n")
		for _, t := range input.Tickets {
			sb.WriteString(fmt.Sprintf("#### [%s] %s (%s)\n", t.IssueKey, t.Title, strings.ToUpper(t.Provider)))
			if t.Description != "" {
				sb.WriteString(fmt.Sprintf("%s\n", t.Description))
			}
			if len(t.AcceptanceCriteria) > 0 {
				sb.WriteString("\n**Acceptance Criteria**:\n")
				for _, ac := range t.AcceptanceCriteria {
					sb.WriteString(fmt.Sprintf("- [ ] %s\n", ac))
				}
			}
			sb.WriteString("\n")
		}
		layers = append(layers, ContextLayer{
			Kind:     LayerTicket,
			Name:     "Ticket Requirements",
			Content:  sb.String(),
			Priority: 80,
		})
	}

	// 4. Layer: Knowledge Graph & Blast Radius (Priority = 70)
	if strings.TrimSpace(input.KnowledgeGraphPrompt) != "" {
		layers = append(layers, ContextLayer{
			Kind:     LayerKnowledgeGraph,
			Name:     "Knowledge Graph",
			Content:  input.KnowledgeGraphPrompt,
			Priority: 70,
		})
	}

	// 5. Layer: Drixy Custom Rules (Priority = 60)
	if strings.TrimSpace(input.RulesSummary) != "" {
		layers = append(layers, ContextLayer{
			Kind:     LayerRules,
			Name:     "Custom Rules",
			Content:  input.RulesSummary,
			Priority: 60,
		})
	}

	// 6. Layer: Repository Conventions (Priority = 50)
	if len(input.ConventionDocs) > 0 {
		var sb strings.Builder
		sb.WriteString("### 📐 Repository Conventions & Guidelines\n\n")
		for _, doc := range input.ConventionDocs {
			sb.WriteString(doc + "\n\n")
		}
		layers = append(layers, ContextLayer{
			Kind:     LayerConventions,
			Name:     "Conventions",
			Content:  sb.String(),
			Priority: 50,
		})
	}

	// Allocate budgets across all layers
	allocatedLayers, totalTokens := a.allocator.Allocate(layers, config)

	// Assemble final prompt string
	var promptSB strings.Builder
	droppedCount := 0

	for _, l := range allocatedLayers {
		if l.DroppedForBudget {
			droppedCount++
			continue
		}
		promptSB.WriteString(fmt.Sprintf("=== CONTEXT LAYER: %s ===\n", strings.ToUpper(l.Name)))
		promptSB.WriteString(l.Content)
		promptSB.WriteString("\n\n")
	}

	return &ContextPack{
		Layers:             allocatedLayers,
		TotalTokens:        totalTokens,
		DroppedLayersCount: droppedCount,
		AssembledPrompt:    promptSB.String(),
		CreatedAt:          time.Now().UTC(),
	}, nil
}
