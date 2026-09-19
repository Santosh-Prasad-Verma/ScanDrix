// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package orchestrator

import (
	"fmt"
	"math"
	"strings"

	"github.com/scandrix/backend/pkg/models"
)

// SemanticDedupConfig configures thresholds for finding deduplication.
type SemanticDedupConfig struct {
	ContentThreshold float64 // Jaccard similarity threshold (e.g. 0.65)
	EmbeddingLow     float64 // Low embedding cosine similarity threshold (e.g. 0.82)
	EmbeddingHigh    float64 // High embedding cosine similarity threshold (e.g. 0.92)
	LineDistanceMax  int     // Max line distance to consider two findings overlapping
}

// DefaultSemanticDedupConfig returns default production thresholds.
func DefaultSemanticDedupConfig() SemanticDedupConfig {
	return SemanticDedupConfig{
		ContentThreshold: 0.65,
		EmbeddingLow:     0.82,
		EmbeddingHigh:    0.92,
		LineDistanceMax:  10,
	}
}

// Embedder computes vector representations for semantic text comparison.
type Embedder interface {
	Embed(text string) ([]float64, error)
}

// SemanticDeduplicator identifies, merges, and resolves duplicate findings across specialists.
type SemanticDeduplicator struct {
	config   SemanticDedupConfig
	embedder Embedder
}

// NewSemanticDeduplicator constructs a deduplicator with optional custom embedder.
func NewSemanticDeduplicator(embedder Embedder, cfg ...SemanticDedupConfig) *SemanticDeduplicator {
	c := DefaultSemanticDedupConfig()
	if len(cfg) > 0 {
		c = cfg[0]
	}
	return &SemanticDeduplicator{
		config:   c,
		embedder: embedder,
	}
}

// DeduplicateFindings executes the multi-pass deduplication pipeline.
func (d *SemanticDeduplicator) DeduplicateFindings(findings []AgentFinding) ([]AgentFinding, int) {
	if len(findings) <= 1 {
		return findings, 0
	}

	initialCount := len(findings)
	var deduplicated []AgentFinding

	// Group findings by FilePath first
	byFile := make(map[string][]AgentFinding)
	for _, f := range findings {
		byFile[f.FilePath] = append(byFile[f.FilePath], f)
	}

	for _, fileFindings := range byFile {
		merged := d.dedupSingleFile(fileFindings)
		deduplicated = append(deduplicated, merged...)
	}

	droppedCount := initialCount - len(deduplicated)
	return deduplicated, droppedCount
}

func (d *SemanticDeduplicator) dedupSingleFile(findings []AgentFinding) []AgentFinding {
	if len(findings) <= 1 {
		return findings
	}

	merged := make([]bool, len(findings))
	var result []AgentFinding

	for i := 0; i < len(findings); i++ {
		if merged[i] {
			continue
		}

		current := findings[i]
		if len(current.ContributingAgents) == 0 && current.AgentName != "" {
			current.ContributingAgents = []string{current.AgentName}
		}

		for j := i + 1; j < len(findings); j++ {
			if merged[j] {
				continue
			}

			candidate := findings[j]
			if d.isDuplicate(current, candidate) {
				merged[j] = true
				current = d.mergeFindings(current, candidate)
			}
		}

		result = append(result, current)
	}

	return result
}

func (d *SemanticDeduplicator) isDuplicate(a, b AgentFinding) bool {
	// Must be in the same file
	if a.FilePath != b.FilePath {
		return false
	}

	// 1. Line distance check
	linesOverlap := linesAreClose(a.StartLine, a.EndLine, b.StartLine, b.EndLine, d.config.LineDistanceMax)
	if !linesOverlap {
		return false
	}

	// 2. Exact fingerprint check
	if a.Fingerprint != "" && a.Fingerprint == b.Fingerprint {
		return true
	}

	// 3. Line overlap ratio and token similarity check
	textA := fmt.Sprintf("%s %s %s", a.Title, a.Description, a.Remediation)
	textB := fmt.Sprintf("%s %s %s", b.Title, b.Description, b.Remediation)
	jaccard := computeJaccardSimilarity(textA, textB)

	overlapLen := min(a.EndLine, b.EndLine) - max(a.StartLine, b.StartLine) + 1
	if overlapLen > 0 {
		minSpan := min(a.EndLine-a.StartLine+1, b.EndLine-b.StartLine+1)
		overlapRatio := float64(overlapLen) / float64(minSpan)
		if overlapRatio >= 0.50 && jaccard >= 0.20 {
			return true
		}
	}

	if jaccard >= d.config.ContentThreshold {
		return true
	}

	// 4. Embedding cosine similarity check if embedder is present
	if d.embedder != nil {
		vecA, errA := d.embedder.Embed(textA)
		vecB, errB := d.embedder.Embed(textB)
		if errA == nil && errB == nil {
			cosine := computeCosineSimilarity(vecA, vecB)
			if cosine >= d.config.EmbeddingHigh {
				return true
			}
			if cosine >= d.config.EmbeddingLow && jaccard >= 0.40 {
				return true
			}
		}
	}

	return false
}

func (d *SemanticDeduplicator) mergeFindings(primary, secondary AgentFinding) AgentFinding {
	merged := primary

	// 1. Preserve highest severity
	if severityWeight(secondary.Severity) > severityWeight(primary.Severity) {
		merged.Severity = secondary.Severity
		merged.Blocking = secondary.Blocking
	} else if severityWeight(secondary.Severity) == severityWeight(primary.Severity) {
		if secondary.Blocking {
			merged.Blocking = true
		}
	}

	// 2. Preserve highest confidence
	if confidenceWeight(secondary.Confidence) > confidenceWeight(primary.Confidence) {
		merged.Confidence = secondary.Confidence
	}

	// 3. Prefer longer/more detailed description and remediation
	if len(secondary.Description) > len(merged.Description) {
		merged.Description = secondary.Description
	}
	if len(secondary.Remediation) > len(merged.Remediation) {
		merged.Remediation = secondary.Remediation
		merged.ImprovedCode = secondary.ImprovedCode
	}

	// 4. Combine contributing agents
	agentMap := make(map[string]struct{})
	for _, a := range merged.ContributingAgents {
		agentMap[a] = struct{}{}
	}
	for _, a := range secondary.ContributingAgents {
		agentMap[a] = struct{}{}
	}
	if secondary.AgentName != "" {
		agentMap[secondary.AgentName] = struct{}{}
	}

	var combined []string
	for a := range agentMap {
		combined = append(combined, a)
	}
	merged.ContributingAgents = combined

	// 5. Expand line range to cover both
	if secondary.StartLine < merged.StartLine {
		merged.StartLine = secondary.StartLine
	}
	if secondary.EndLine > merged.EndLine {
		merged.EndLine = secondary.EndLine
	}

	return merged
}

func linesAreClose(s1, e1, s2, e2, maxDist int) bool {
	// Overlapping
	if s1 <= e2 && s2 <= e1 {
		return true
	}
	// Nearby
	if intAbs(s1-s2) <= maxDist || intAbs(e1-e2) <= maxDist {
		return true
	}
	return false
}

func computeJaccardSimilarity(textA, textB string) float64 {
	tokensA := tokenize(textA)
	tokensB := tokenize(textB)

	if len(tokensA) == 0 && len(tokensB) == 0 {
		return 1.0
	}
	if len(tokensA) == 0 || len(tokensB) == 0 {
		return 0.0
	}

	setA := make(map[string]struct{}, len(tokensA))
	for _, t := range tokensA {
		setA[t] = struct{}{}
	}

	setB := make(map[string]struct{}, len(tokensB))
	for _, t := range tokensB {
		setB[t] = struct{}{}
	}

	intersection := 0
	for k := range setA {
		if _, exists := setB[k]; exists {
			intersection++
		}
	}

	union := len(setA) + len(setB) - intersection
	if union <= 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

func computeCosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA <= 0 || normB <= 0 {
		return 0.0
	}

	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func tokenize(text string) []string {
	clean := strings.ToLower(text)
	words := strings.FieldsFunc(clean, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_'
	})
	var result []string
	for _, w := range words {
		if len(w) > 2 { // filter 1-2 char noise
			result = append(result, w)
		}
	}
	return result
}

func severityWeight(s models.FindingSeverity) int {
	switch s {
	case models.SeverityCritical:
		return 5
	case models.SeverityHigh:
		return 4
	case models.SeverityMedium:
		return 3
	case models.SeverityLow:
		return 2
	case models.SeverityInfo:
		return 1
	default:
		return 0
	}
}

func confidenceWeight(c string) int {
	switch strings.ToUpper(c) {
	case "HIGH":
		return 3
	case "MEDIUM":
		return 2
	case "LOW":
		return 1
	default:
		return 0
	}
}

func intAbs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
