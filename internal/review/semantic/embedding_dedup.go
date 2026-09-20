// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package semantic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// DedupThresholds configure lexical and vector embedding matching bands.
const (
	DefaultContentThreshold = 0.3
	DefaultEmbeddingLow     = 0.40
	DefaultEmbeddingHigh    = 0.70
)

// DedupDecision classifies relationship between two findings.
type DedupDecision string

const (
	DecisionDuplicate DedupDecision = "DUPLICATE"
	DecisionDistinct  DedupDecision = "DISTINCT"
	DecisionAmbiguous DedupDecision = "AMBIGUOUS"
)

var wordRegex = regexp.MustCompile(`[a-z_][a-z0-9_]{2,}`)

// TokenizeWords extracts lowercased alpha-numeric words of length 3+.
func TokenizeWords(text string) map[string]struct{} {
	matches := wordRegex.FindAllString(strings.ToLower(text), -1)
	wordSet := make(map[string]struct{}, len(matches))
	for _, w := range matches {
		wordSet[w] = struct{}{}
	}
	return wordSet
}

// JaccardSimilarity computes intersection over union for two word sets.
func JaccardSimilarity(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}
	intersection := 0
	for w := range a {
		if _, exists := b[w]; exists {
			intersection++
		}
	}
	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0.0
	}
	return float64(intersection) / float64(union)
}

// CalculateContentSimilarity evaluates Jaccard word-overlap across finding text.
// Takes into account Title, Description, and Remediation. When both findings include
// a SuggestedDiff, diff tokens are incorporated to measure code fix alignment.
func CalculateContentSimilarity(a, b *models.CodeFinding) float64 {
	if a == nil || b == nil {
		return 0.0
	}

	bodyTextA := fmt.Sprintf("%s %s %s", a.Title, a.Description, a.Remediation)
	bodyTextB := fmt.Sprintf("%s %s %s", b.Title, b.Description, b.Remediation)

	wordsBodyA := TokenizeWords(bodyTextA)
	wordsBodyB := TokenizeWords(bodyTextB)
	bodySim := JaccardSimilarity(wordsBodyA, wordsBodyB)

	// If both findings include SuggestedDiff, evaluate combined lexical tokens
	if len(a.SuggestedDiff) > 0 && len(b.SuggestedDiff) > 0 {
		wordsDiffA := TokenizeWords(a.SuggestedDiff)
		wordsDiffB := TokenizeWords(b.SuggestedDiff)

		combinedA := make(map[string]struct{}, len(wordsBodyA)+len(wordsDiffA))
		for k := range wordsBodyA {
			combinedA[k] = struct{}{}
		}
		for k := range wordsDiffA {
			combinedA[k] = struct{}{}
		}

		combinedB := make(map[string]struct{}, len(wordsBodyB)+len(wordsDiffB))
		for k := range wordsBodyB {
			combinedB[k] = struct{}{}
		}
		for k := range wordsDiffB {
			combinedB[k] = struct{}{}
		}

		diffSim := JaccardSimilarity(combinedA, combinedB)
		if diffSim > bodySim {
			return diffSim
		}
	}

	return bodySim
}

// FindingDetailScore scores finding richness based on description length and diff size.
func FindingDetailScore(f *models.CodeFinding) int {
	if f == nil {
		return 0
	}
	return len(f.Description) + len(f.Remediation) + len(f.SuggestedDiff)
}

// CollapseNearDuplicates greedily collapses near-duplicate findings within each file.
// It groups candidates by FilePath, collapses candidates with lexical similarity >= threshold,
// and preserves the candidate with the highest detail score.
func CollapseNearDuplicates(findings []*models.CodeFinding, threshold float64) []*models.CodeFinding {
	if threshold <= 0 {
		threshold = DefaultContentThreshold
	}

	byFile := make(map[string][]*models.CodeFinding)
	for _, f := range findings {
		if f == nil {
			continue
		}
		byFile[f.FilePath] = append(byFile[f.FilePath], f)
	}

	var collapsed []*models.CodeFinding

	for _, list := range byFile {
		var bucket []*models.CodeFinding
		for _, candidate := range list {
			matchedIndex := -1
			for idx, rep := range bucket {
				if CalculateContentSimilarity(rep, candidate) >= threshold {
					matchedIndex = idx
					break
				}
			}

			if matchedIndex == -1 {
				bucket = append(bucket, candidate)
			} else {
				// Keep the one with richer detail
				existing := bucket[matchedIndex]
				if FindingDetailScore(candidate) > FindingDetailScore(existing) {
					bucket[matchedIndex] = candidate
					// Re-fold any other members in bucket now covered by candidate
					retained := make([]*models.CodeFinding, 0, len(bucket))
					for i, b := range bucket {
						if i == matchedIndex || CalculateContentSimilarity(b, candidate) < threshold {
							retained = append(retained, b)
						}
					}
					bucket = retained
				}
			}
		}
		collapsed = append(collapsed, bucket...)
	}

	return collapsed
}

// CosineSimilarity calculates the dot-product cosine angle between two float32 vectors.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0.0
	}

	var dot, normA, normB float64
	for i := range a {
		valA := float64(a[i])
		valB := float64(b[i])
		dot += valA * valB
		normA += valA * valA
		normB += valB * valB
	}

	if normA == 0 || normB == 0 {
		return 0.0
	}

	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// CosineSimilarityFloat64 calculates cosine similarity between two float64 vectors.
func CosineSimilarityFloat64(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0.0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0.0
	}

	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// Embedder defines the contract for generating vector representations of text.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// DuplicateMatch records duplicate suppression details.
type DuplicateMatch struct {
	OriginalID      uuid.UUID     `json:"original_id"`
	DuplicateID     uuid.UUID     `json:"duplicate_id"`
	FilePath        string        `json:"file_path"`
	SimilarityScore float64       `json:"similarity_score"`
	MatchingTier    string        `json:"matching_tier"` // "LEXICAL", "EMBEDDING", or "EXACT_FINGERPRINT"
	Decision        DedupDecision `json:"decision"`
}

// CachedFinding stores a previously reviewed finding along with its computed embedding.
type CachedFinding struct {
	Finding     *models.CodeFinding
	Embedding   []float32
	Fingerprint string
	ReportedAt  time.Time
}

// SemanticDeduplicator manages cross-finding and cross-commit duplicate suppression.
type SemanticDeduplicator struct {
	mu            sync.RWMutex
	embedder      Embedder
	contentThresh float64
	embeddingLow  float64
	embeddingHigh float64
	// PR-keyed cache: "repoID:pullNumber" -> list of active findings
	prCache       map[string][]*CachedFinding
	maxCachePerPR int
}

// NewSemanticDeduplicator constructs a deduplicator with specified thresholds.
func NewSemanticDeduplicator(embedder Embedder, contentThresh, embeddingLow, embeddingHigh float64) *SemanticDeduplicator {
	if contentThresh <= 0 {
		contentThresh = DefaultContentThreshold
	}
	if embeddingLow <= 0 {
		embeddingLow = DefaultEmbeddingLow
	}
	if embeddingHigh <= 0 {
		embeddingHigh = DefaultEmbeddingHigh
	}

	return &SemanticDeduplicator{
		embedder:      embedder,
		contentThresh: contentThresh,
		embeddingLow:  embeddingLow,
		embeddingHigh: embeddingHigh,
		prCache:       make(map[string][]*CachedFinding),
		maxCachePerPR: 500,
	}
}

// GenerateFindingFingerprint creates a deterministic SHA-256 hash for finding identity.
func GenerateFindingFingerprint(f *models.CodeFinding) string {
	if f == nil {
		return ""
	}
	canonical := fmt.Sprintf("%s|%d|%d|%s|%s",
		f.FilePath,
		f.StartLine,
		f.EndLine,
		strings.TrimSpace(strings.ToLower(f.Title)),
		strings.TrimSpace(strings.ToLower(f.Category)),
	)
	hash := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(hash[:16])
}

// ExtractEmbeddingText produces the semantic text for vector embedding.
func ExtractEmbeddingText(f *models.CodeFinding) string {
	if f == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%s. %s", f.Title, f.Description))
}

// DeduplicateBatch suppresses duplicates within a newly produced batch and against
// previously posted findings on the PR.
func (d *SemanticDeduplicator) DeduplicateBatch(
	ctx context.Context,
	repoID string,
	pullNumber int,
	candidates []*models.CodeFinding,
) ([]*models.CodeFinding, []DuplicateMatch, error) {
	if len(candidates) == 0 {
		return nil, nil, nil
	}

	prKey := fmt.Sprintf("%s:%d", repoID, pullNumber)

	// Step 1: Pre-collapse candidates via lexical similarity within file buckets
	lexicallyCollapsed := CollapseNearDuplicates(candidates, d.contentThresh)

	// Step 2: Fetch historical cache for this PR
	d.mu.RLock()
	cachedList := d.prCache[prKey]
	d.mu.RUnlock()

	var retained []*models.CodeFinding
	var duplicates []DuplicateMatch

	// Generate embeddings for lexically collapsed candidates if embedder is available
	var candidateEmbeddings [][]float32
	if d.embedder != nil && len(lexicallyCollapsed) > 0 {
		texts := make([]string, len(lexicallyCollapsed))
		for i, c := range lexicallyCollapsed {
			texts[i] = ExtractEmbeddingText(c)
		}
		embs, err := d.embedder.Embed(ctx, texts)
		if err == nil && len(embs) == len(lexicallyCollapsed) {
			candidateEmbeddings = embs
		}
	}

	// Step 3: Compare against existing cached findings
	for idx, candidate := range lexicallyCollapsed {
		candFP := GenerateFindingFingerprint(candidate)
		candidate.Fingerprint = candFP

		isDuplicate := false

		// Check against cache
		for _, cached := range cachedList {
			// A. Exact fingerprint match
			if cached.Fingerprint == candFP {
				duplicates = append(duplicates, DuplicateMatch{
					OriginalID:      cached.Finding.ID,
					DuplicateID:     candidate.ID,
					FilePath:        candidate.FilePath,
					SimilarityScore: 1.0,
					MatchingTier:    "EXACT_FINGERPRINT",
					Decision:        DecisionDuplicate,
				})
				isDuplicate = true
				break
			}

			// B. Lexical overlap check
			lexSim := CalculateContentSimilarity(candidate, cached.Finding)
			if lexSim >= d.contentThresh && candidate.FilePath == cached.Finding.FilePath {
				duplicates = append(duplicates, DuplicateMatch{
					OriginalID:      cached.Finding.ID,
					DuplicateID:     candidate.ID,
					FilePath:        candidate.FilePath,
					SimilarityScore: lexSim,
					MatchingTier:    "LEXICAL",
					Decision:        DecisionDuplicate,
				})
				isDuplicate = true
				break
			}

			// C. Semantic embedding cosine check
			if len(candidateEmbeddings) > idx && len(cached.Embedding) > 0 {
				cosSim := CosineSimilarity(candidateEmbeddings[idx], cached.Embedding)
				if cosSim >= d.embeddingHigh {
					duplicates = append(duplicates, DuplicateMatch{
						OriginalID:      cached.Finding.ID,
						DuplicateID:     candidate.ID,
						FilePath:        candidate.FilePath,
						SimilarityScore: cosSim,
						MatchingTier:    "EMBEDDING",
						Decision:        DecisionDuplicate,
					})
					isDuplicate = true
					break
				}
			}
		}

		if !isDuplicate {
			retained = append(retained, candidate)

			// Add to cache
			var emb []float32
			if len(candidateEmbeddings) > idx {
				emb = candidateEmbeddings[idx]
			}

			d.addFindingToCache(prKey, candidate, emb, candFP)
		}
	}

	return retained, duplicates, nil
}

func (d *SemanticDeduplicator) addFindingToCache(prKey string, f *models.CodeFinding, emb []float32, fp string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	item := &CachedFinding{
		Finding:     f,
		Embedding:   emb,
		Fingerprint: fp,
		ReportedAt:  time.Now().UTC(),
	}

	list := d.prCache[prKey]
	if len(list) >= d.maxCachePerPR {
		// Evict oldest
		list = list[1:]
	}
	d.prCache[prKey] = append(list, item)
}

// ClearPRCache resets the cache when a PR review cycle completes or is re-run from scratch.
func (d *SemanticDeduplicator) ClearPRCache(repoID string, pullNumber int) {
	prKey := fmt.Sprintf("%s:%d", repoID, pullNumber)
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.prCache, prKey)
}

// BuildTiebreakPrompt constructs a root-cause comparison prompt for ambiguous findings.
func BuildTiebreakPrompt(a, b *models.CodeFinding) string {
	formatFinding := func(f *models.CodeFinding) string {
		if f == nil {
			return ""
		}
		diffSnippet := f.SuggestedDiff
		if len(diffSnippet) > 250 {
			diffSnippet = diffSnippet[:250] + "..."
		}
		return fmt.Sprintf("File: %s (Lines %d-%d)\nTitle: %s\nDetails: %s\nRemediation: %s\nDiff:\n%s",
			f.FilePath, f.StartLine, f.EndLine, f.Title, f.Description, f.Remediation, diffSnippet)
	}

	return fmt.Sprintf(`Two code review findings were identified on the same pull request. Determine whether posting both would be a REDUNDANT DUPLICATE (same root defect) or if they are DISTINCT bugs that both warrant separate developer feedback.

State the single root-cause defect for Finding A and Finding B in one phrase each, then evaluate:
- sameBug = TRUE if both identify the same defect (even if suggested remediation or wording differs).
- sameBug = FALSE if they address different issues, even if in close line proximity.

Finding A:
%s

Finding B:
%s
`, formatFinding(a), formatFinding(b))
}
