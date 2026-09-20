// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package feedback

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/llm/embedding"
	"github.com/scandrix/backend/pkg/models"
)

// DefaultSemanticDistanceThreshold defines the maximum cosine distance
// to qualify as a semantic duplicate (0.20 distance <=> 80% cosine similarity).
const DefaultSemanticDistanceThreshold = 0.20

// SecurityMemoryStore abstracts pgvector and fingerprint operations for security memory.
type SecurityMemoryStore interface {
	UpsertSecurityMemoryWithEmbedding(ctx context.Context, wsID uuid.UUID, mem *database.SecurityMemoryRecord, emb []float32) error
	SearchSimilarSecurityFindings(ctx context.Context, wsID uuid.UUID, emb []float32, category string, limit int, maxDistance float64) ([]database.SecurityMemoryRecord, error)
	GetSecurityMemoryByFingerprint(ctx context.Context, wsID uuid.UUID, fingerprint string) (*database.SecurityMemoryRecord, error)
}

// SemanticFeedbackService orchestrates vector learning from dismissed findings
// and evaluates incoming candidate findings for automated suppression.
type SemanticFeedbackService struct {
	store             SecurityMemoryStore
	embedder          embedding.Embedder
	distanceThreshold float64
}

// NewSemanticFeedbackService constructs a feedback learning engine.
func NewSemanticFeedbackService(store SecurityMemoryStore, embedder embedding.Embedder) *SemanticFeedbackService {
	if embedder == nil {
		embedder = embedding.NewDeterministicSemanticEmbedder()
	}
	return &SemanticFeedbackService{
		store:             store,
		embedder:          embedder,
		distanceThreshold: DefaultSemanticDistanceThreshold,
	}
}

// SetDistanceThreshold sets custom cosine distance gate (e.g. 0.15 for tighter match, 0.25 for looser).
func (s *SemanticFeedbackService) SetDistanceThreshold(threshold float64) {
	if threshold > 0 && threshold < 1.0 {
		s.distanceThreshold = threshold
	}
}

// RecordDismissal vectorizes and persists a dismissed finding into security memory.
func (s *SemanticFeedbackService) RecordDismissal(
	ctx context.Context,
	wsID uuid.UUID,
	finding *models.CodeFinding,
	reason string,
	actorEmail string,
) error {
	if s.store == nil || finding == nil {
		return nil
	}

	if reason == "" {
		reason = "FALSE_POSITIVE"
	}

	codeSnippet := finding.SuggestedDiff
	if codeSnippet == "" {
		codeSnippet = finding.Remediation
	}

	// Prepare semantic text for vectorization
	textToEmbed := FormatFindingForEmbedding(finding.Title, finding.Category, codeSnippet, finding.Description)
	vec, err := s.embedder.EmbedText(ctx, textToEmbed)
	if err != nil {
		slog.Warn("Failed generating embedding for dismissed finding; falling back to zero vector", "error", err, "title", finding.Title)
		vec = nil
	}

	rec := &database.SecurityMemoryRecord{
		ID:                 uuid.New(),
		WorkspaceID:        wsID,
		FindingFingerprint: finding.Fingerprint,
		Category:           finding.Category,
		RuleID:             finding.Title,
		CodeSnippet:        codeSnippet,
		Justification:      finding.Description,
		DismissalReason:    reason,
		CreatedAt:          time.Now().UTC(),
	}

	if err := s.store.UpsertSecurityMemoryWithEmbedding(ctx, wsID, rec, vec); err != nil {
		return fmt.Errorf("failed persisting security memory for dismissed finding: %w", err)
	}

	slog.Info("Successfully learned dismissed finding into security memory",
		"workspace_id", wsID,
		"rule", finding.Title,
		"fingerprint", finding.Fingerprint,
		"reason", reason,
		"actor", actorEmail,
	)

	return nil
}

// SuppressionDecision captures the evaluation result of a candidate finding.
type SuppressionDecision struct {
	ShouldSuppress   bool                            `json:"should_suppress"`
	IsExactMatch     bool                            `json:"is_exact_match"`
	SimilarityScore  float64                         `json:"similarity_score"`
	CosineDistance   float64                         `json:"cosine_distance"`
	HistoricalMemory *database.SecurityMemoryRecord `json:"historical_memory,omitempty"`
	Reason           string                          `json:"reason,omitempty"`
}

// CheckSuppression evaluates a candidate finding against historical security memory.
func (s *SemanticFeedbackService) CheckSuppression(
	ctx context.Context,
	wsID uuid.UUID,
	finding *models.CodeFinding,
) SuppressionDecision {
	if s.store == nil || finding == nil {
		return SuppressionDecision{ShouldSuppress: false}
	}

	// 1. Tier 1: Instant Exact Cryptographic Fingerprint Check
	if finding.Fingerprint != "" {
		if exactMem, err := s.store.GetSecurityMemoryByFingerprint(ctx, wsID, finding.Fingerprint); err == nil && exactMem != nil {
			return SuppressionDecision{
				ShouldSuppress:   isDismissalSuppressable(exactMem.DismissalReason),
				IsExactMatch:     true,
				SimilarityScore:  1.0,
				CosineDistance:   0.0,
				HistoricalMemory: exactMem,
				Reason:           fmt.Sprintf("Exact fingerprint match with historical dismissal (%s)", exactMem.DismissalReason),
			}
		}
	}

	// 2. Tier 2: Semantic Vector Cosine Similarity Search
	codeSnippet := finding.SuggestedDiff
	if codeSnippet == "" {
		codeSnippet = finding.Remediation
	}

	textToEmbed := FormatFindingForEmbedding(finding.Title, finding.Category, codeSnippet, finding.Description)
	vec, err := s.embedder.EmbedText(ctx, textToEmbed)
	if err != nil || len(vec) == 0 {
		return SuppressionDecision{ShouldSuppress: false}
	}

	matches, err := s.store.SearchSimilarSecurityFindings(ctx, wsID, vec, finding.Category, 1, s.distanceThreshold)
	if err != nil || len(matches) == 0 {
		return SuppressionDecision{ShouldSuppress: false}
	}

	bestMatch := matches[0]
	if isDismissalSuppressable(bestMatch.DismissalReason) {
		similarity := bestMatch.SimilarityScore
		dist := 1.0 - similarity
		return SuppressionDecision{
			ShouldSuppress:   true,
			IsExactMatch:     false,
			SimilarityScore:  similarity,
			CosineDistance:   dist,
			HistoricalMemory: &bestMatch,
			Reason: fmt.Sprintf("Semantic vector similarity (%.1f%%) matches historical dismissal (%s)",
				similarity*100.0, bestMatch.DismissalReason),
		}
	}

	return SuppressionDecision{ShouldSuppress: false}
}

// FormatFindingForEmbedding structures finding context for high-precision semantic search.
func FormatFindingForEmbedding(rule, category, codeSnippet, description string) string {
	var sb strings.Builder
	if rule != "" {
		sb.WriteString("Rule: ")
		sb.WriteString(rule)
		sb.WriteString("\n")
	}
	if category != "" {
		sb.WriteString("Category: ")
		sb.WriteString(category)
		sb.WriteString("\n")
	}
	if description != "" {
		sb.WriteString("Description: ")
		sb.WriteString(description)
		sb.WriteString("\n")
	}
	if codeSnippet != "" {
		sb.WriteString("Code:\n")
		sb.WriteString(strings.TrimSpace(codeSnippet))
	}
	return sb.String()
}

func isDismissalSuppressable(reason string) bool {
	clean := strings.ToUpper(strings.TrimSpace(reason))
	clean = strings.ReplaceAll(clean, "'", "")
	clean = strings.ReplaceAll(clean, "-", "_")
	clean = strings.ReplaceAll(clean, " ", "_")
	return clean == "FALSE_POSITIVE" || clean == "WONT_FIX" || clean == "IRRELEVANT" || clean == "INTENDED_BEHAVIOR" || clean == "SUPPRESS"
}
