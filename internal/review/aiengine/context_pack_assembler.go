// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package aiengine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/internal/review/priority"
)

// ContextLayerType identifies the purpose of an information layer in the review prompt.
type ContextLayerType string

const (
	LayerSystemPersona   ContextLayerType = "SYSTEM_PERSONA"
	LayerArchDecisions   ContextLayerType = "ARCH_DECISIONS"
	LayerExternalTicket  ContextLayerType = "EXTERNAL_TICKET"
	LayerActiveRules     ContextLayerType = "ACTIVE_RULES"
	LayerDiffHunks       ContextLayerType = "DIFF_HUNKS"
	LayerSymbolIndex     ContextLayerType = "SYMBOL_INDEX"
)

// ContextLayer represents a distinct modular block of review context.
type ContextLayer struct {
	Type        ContextLayerType `json:"type"`
	Title       string           `json:"title"`
	Content     string           `json:"content"`
	TokenWeight int              `json:"token_weight"`
	Priority    int              `json:"priority"` // Higher priority = kept during trimming (0-100)
}

// ContextPack represents the assembled and bounded prompt envelope for model evaluation.
type ContextPack struct {
	DigestToken     string         `json:"digest_token"`
	TotalTokens     int            `json:"total_tokens"`
	WasTrimmed      bool           `json:"was_trimmed"`
	TrimmedBytes    int            `json:"trimmed_bytes,omitempty"`
	Layers          []ContextLayer `json:"layers"`
	GeneratedAt     time.Time      `json:"generated_at"`
}

// ContextPackAssembler coordinates multi-layer context compilation and adaptive token trimming.
type ContextPackAssembler struct {
	maxTokenBudget  int
	charsPerToken   int
	refDetector     *ReferenceDetector
	riskClassifier  *priority.SecurityRiskClassifier
}

// NewContextPackAssembler constructs a context pack assembler with specified token budget.
func NewContextPackAssembler(maxTokens int) *ContextPackAssembler {
	if maxTokens <= 0 {
		maxTokens = 64000 // 64k token default window
	}
	return &ContextPackAssembler{
		maxTokenBudget: maxTokens,
		charsPerToken:  4,
		refDetector:    NewReferenceDetector(),
		riskClassifier: priority.NewSecurityRiskClassifier(),
	}
}

// AssemblePack builds the multi-layer context pack from the pipeline context and trims to budget.
func (a *ContextPackAssembler) AssemblePack(pCtx *pipeline.PipelineContext) ContextPack {
	var layers []ContextLayer

	// 1. Layer: System Persona & Standards (Highest Priority: 100)
	personaContent := `You are ScanDrix, an elite autonomous code review engine with deep expertise in security, concurrency, API design, and clean architecture.
Your mission is to identify real defects, vulnerabilities, and deviations from invariants. Do not produce noisy or pedantic nitpicks.`
	layers = append(layers, ContextLayer{
		Type:        LayerSystemPersona,
		Title:       "Review Persona & Guidelines",
		Content:     personaContent,
		TokenWeight: len(personaContent) / a.charsPerToken,
		Priority:    100,
	})

	// 2. Layer: Architectural Decisions (Priority: 90)
	if len(pCtx.TraceDecisions) > 0 {
		var sb strings.Builder
		sb.WriteString("### Architectural Decision Records (ADRs)\n")
		for _, td := range pCtx.TraceDecisions {
			sb.WriteString(fmt.Sprintf("- **[%s] %s**: %s\n", td.DecisionKey, td.Title, td.Summary))
		}
		adrText := sb.String()
		layers = append(layers, ContextLayer{
			Type:        LayerArchDecisions,
			Title:       "Architectural Decisions",
			Content:     adrText,
			TokenWeight: len(adrText) / a.charsPerToken,
			Priority:    90,
		})
	}

	// 3. Layer: External Ticket Context & Detected Citations (Priority: 85)
	var ticketText strings.Builder
	if pCtx.ExternalContext != nil {
		ticketText.WriteString(fmt.Sprintf("### Ticket [%s]: %s\n", pCtx.ExternalContext.IssueKey, pCtx.ExternalContext.Title))
		ticketText.WriteString(pCtx.ExternalContext.Description + "\n\n")
	}
	detectedRefs := a.refDetector.DetectReferences(pCtx.Title + "\n" + pCtx.Description)
	if len(detectedRefs) > 0 {
		ticketText.WriteString(a.refDetector.FormatContextMarkdown(detectedRefs))
	}
	if ticketText.Len() > 0 {
		tContent := ticketText.String()
		layers = append(layers, ContextLayer{
			Type:        LayerExternalTicket,
			Title:       "Ticket & External Context",
			Content:     tContent,
			TokenWeight: len(tContent) / a.charsPerToken,
			Priority:    85,
		})
	}

	// 4. Layer: Active Review Rules (Priority: 80)
	if len(pCtx.ActiveRules) > 0 {
		var rulesText strings.Builder
		rulesText.WriteString("### Mandatory Rule Specifications\n")
		for _, r := range pCtx.ActiveRules {
			rulesText.WriteString(fmt.Sprintf("- **[%s] %s**: %s (Severity: %s)\n", r.ID, r.Name, r.Description, r.Severity))
		}
		rContent := rulesText.String()
		layers = append(layers, ContextLayer{
			Type:        LayerActiveRules,
			Title:       "Custom Review Rules",
			Content:     rContent,
			TokenWeight: len(rContent) / a.charsPerToken,
			Priority:    80,
		})
	}

	// 5. Layer: Diff Hunks & Line Coordinates (Priority: 70)
	if len(pCtx.ChangedFiles) > 0 {
		var diffText strings.Builder
		diffText.WriteString("### Pull Request File Modifications\n")
		for _, f := range pCtx.ChangedFiles {
			diffText.WriteString(fmt.Sprintf("#### File: `%s` (+%d -%d)\n", f.Filename, f.Additions, f.Deletions))
			if f.Patch != "" {
				diffText.WriteString("```diff\n")
				diffText.WriteString(f.Patch)
				diffText.WriteString("\n```\n\n")
			}
		}
		dContent := diffText.String()
		layers = append(layers, ContextLayer{
			Type:        LayerDiffHunks,
			Title:       "Code Modifications",
			Content:     dContent,
			TokenWeight: len(dContent) / a.charsPerToken,
			Priority:    70,
		})
	}

	// Calculate total tokens
	totalTokens := 0
	for _, l := range layers {
		totalTokens += l.TokenWeight
	}

	wasTrimmed := false
	trimmedBytes := 0

	// Adaptive Trimming if exceeding token budget
	if totalTokens > a.maxTokenBudget {
		wasTrimmed = true
		layers, trimmedBytes = a.trimLayers(layers, a.maxTokenBudget)

		// Recalculate tokens after trimming
		totalTokens = 0
		for _, l := range layers {
			totalTokens += l.TokenWeight
		}
	}

	// Compute Digest Token
	digest := a.computePackDigest(layers)

	return ContextPack{
		DigestToken:  digest,
		TotalTokens:  totalTokens,
		WasTrimmed:   wasTrimmed,
		TrimmedBytes: trimmedBytes,
		Layers:       layers,
		GeneratedAt:  time.Now().UTC(),
	}
}

func (a *ContextPackAssembler) trimLayers(layers []ContextLayer, budgetTokens int) ([]ContextLayer, int) {
	// Sort layer indices by priority ascending (trim lowest priority first)
	type layerRef struct {
		idx      int
		priority int
	}

	refs := make([]layerRef, len(layers))
	for i, l := range layers {
		refs[i] = layerRef{idx: i, priority: l.Priority}
	}

	sort.Slice(refs, func(i, j int) bool {
		return refs[i].priority < refs[j].priority
	})

	totalTokens := 0
	for _, l := range layers {
		totalTokens += l.TokenWeight
	}

	trimmedBytes := 0
	for _, ref := range refs {
		if totalTokens <= budgetTokens {
			break
		}

		target := &layers[ref.idx]
		excessTokens := totalTokens - budgetTokens
		bytesToRemove := excessTokens * a.charsPerToken

		if bytesToRemove >= len(target.Content) {
			// Remove entire layer content
			trimmedBytes += len(target.Content)
			totalTokens -= target.TokenWeight
			target.Content = "[TRIMMED DUE TO CONTEXT BUDGET]"
			target.TokenWeight = len(target.Content) / a.charsPerToken
		} else {
			// Truncate layer content with ellipsis
			cutPoint := len(target.Content) - bytesToRemove
			if cutPoint < 200 {
				cutPoint = 200
			}
			trimmedBytes += len(target.Content) - cutPoint
			target.Content = target.Content[:cutPoint] + "\n... [TRUNCATED DUE TO CONTEXT BUDGET]"
			target.TokenWeight = len(target.Content) / a.charsPerToken
			totalTokens -= (bytesToRemove / a.charsPerToken)
		}
	}

	return layers, trimmedBytes
}

func (a *ContextPackAssembler) computePackDigest(layers []ContextLayer) string {
	h := sha256.New()
	for _, l := range layers {
		h.Write([]byte(string(l.Type)))
		h.Write([]byte(l.Content))
	}
	return hex.EncodeToString(h.Sum(nil))
}
