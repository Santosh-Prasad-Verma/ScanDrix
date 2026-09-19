// Package semantic provides k-means clustering and feedback-driven suggestion fine-tuning
// for the ScanDrix code review pipeline, evaluating candidate suggestions against historical
// user reactions, accepted fixes, and dismissed false positives.
package semantic

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// FeedbackType categorizes user or system reactions to review suggestions.
type FeedbackType string

const (
	FeedbackNeutral               FeedbackType = "NEUTRAL"
	FeedbackPositiveReaction      FeedbackType = "POSITIVE_REACTION"
	FeedbackNegativeReaction      FeedbackType = "NEGATIVE_REACTION"
	FeedbackSuggestionImplemented FeedbackType = "SUGGESTION_IMPLEMENTED"
	FeedbackDismissed             FeedbackType = "DISMISSED"
)

// FineTuningDecision specifies the action to take for a candidate suggestion.
type FineTuningDecision string

const (
	DecisionKeep      FineTuningDecision = "KEEP"
	DecisionDiscard   FineTuningDecision = "DISCARD"
	DecisionUncertain FineTuningDecision = "UNCERTAIN"
)

// SuggestionLabel categorizes priority labels that override heuristic discard.
type SuggestionLabel string

const (
	LabelDrixyRules      SuggestionLabel = "DRIXY_RULES"
	LabelBreakingChanges SuggestionLabel = "BREAKING_CHANGES"
	LabelSecurity        SuggestionLabel = "SECURITY"
)

// Constants matching the ScanDrix k-means fine-tuning engine.
const (
	MaxClusters                 = 50
	DivisorForClusterQuantity   = 4
	SimilarityThresholdNegative = 0.60
	SimilarityThresholdPositive = 0.60
	SimilarityThresholdCluster  = 0.60
	DefaultKMeansMaxIterations  = 100
	DefaultConvergenceTolerance = 1e-4
)

// EmbeddedSuggestion represents a historical or candidate suggestion with vector representation.
type EmbeddedSuggestion struct {
	ID             uuid.UUID    `json:"id"`
	WorkspaceID    uuid.UUID    `json:"workspace_id"`
	RepositoryID   string       `json:"repository_id"`
	PullNumber     int          `json:"pull_number"`
	FilePath       string       `json:"file_path"`
	Title          string       `json:"title"`
	Description    string       `json:"description"`
	Remediation    string       `json:"remediation"`
	Language       string       `json:"language"`
	Label          string       `json:"label"`
	FeedbackType   FeedbackType `json:"feedback_type"`
	Embedding      []float64    `json:"embedding"`
	ThumbsUpCount  int          `json:"thumbs_up_count"`
	ThumbsDownCount int         `json:"thumbs_down_count"`
	CreatedAt      time.Time    `json:"created_at"`
}

// ClusterizedSuggestion wraps an embedded suggestion with its assigned cluster index.
type ClusterizedSuggestion struct {
	Cluster            int                 `json:"cluster"`
	OriginalSuggestion *EmbeddedSuggestion `json:"original_suggestion"`
	DistanceToCentroid float64             `json:"distance_to_centroid"`
	SimilarityToCentroid float64           `json:"similarity_to_centroid"`
}

// ClusterAnalysisResult captures the fine-tuning evaluation outcome for a candidate finding.
type ClusterAnalysisResult struct {
	AnalyzedFinding    *models.CodeFinding `json:"analyzed_finding"`
	Decision           FineTuningDecision  `json:"decision"`
	NearestClusterID   int                 `json:"nearest_cluster_id"`
	ClusterSimilarity  float64             `json:"cluster_similarity"`
	PositiveScore      float64             `json:"positive_score"`
	NegativeScore      float64             `json:"negative_score"`
	Justification      string              `json:"justification"`
}

// ClusterCentroid represents the mean vector and composition metrics of a cluster.
type ClusterCentroid struct {
	ClusterID     int       `json:"cluster_id"`
	Centroid      []float64 `json:"centroid"`
	MemberCount   int       `json:"member_count"`
	PositiveCount int       `json:"positive_count"`
	NegativeCount int       `json:"negative_count"`
	Inertia       float64   `json:"inertia"`
}

// ClusterTuningEngine coordinates suggestion vector clustering and automated suggestion pruning.
type ClusterTuningEngine struct {
	mu                         sync.RWMutex
	rndMu                      sync.Mutex
	maxClusters                int
	divisorForClusterQuantity  int
	simThreshNegative          float64
	simThreshPositive          float64
	simThreshCluster           float64
	maxIterations              int
	convergenceTolerance       float64
	rnd                        *rand.Rand
}

// NewClusterTuningEngine creates a production cluster fine-tuning engine.
func NewClusterTuningEngine() *ClusterTuningEngine {
	return &ClusterTuningEngine{
		maxClusters:               MaxClusters,
		divisorForClusterQuantity: DivisorForClusterQuantity,
		simThreshNegative:         SimilarityThresholdNegative,
		simThreshPositive:         SimilarityThresholdPositive,
		simThreshCluster:          SimilarityThresholdCluster,
		maxIterations:             DefaultKMeansMaxIterations,
		convergenceTolerance:      DefaultConvergenceTolerance,
		rnd:                       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (e *ClusterTuningEngine) randIntn(n int) int {
	e.rndMu.Lock()
	defer e.rndMu.Unlock()
	return e.rnd.Intn(n)
}

func (e *ClusterTuningEngine) randFloat64() float64 {
	e.rndMu.Lock()
	defer e.rndMu.Unlock()
	return e.rnd.Float64()
}

// SetThresholds customizes operational matching thresholds.
func (e *ClusterTuningEngine) SetThresholds(negative, positive, cluster float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if negative > 0 && negative <= 1.0 {
		e.simThreshNegative = negative
	}
	if positive > 0 && positive <= 1.0 {
		e.simThreshPositive = positive
	}
	if cluster > 0 && cluster <= 1.0 {
		e.simThreshCluster = cluster
	}
}

// NormalizeText cleans and standardizes suggestion text for consistent embedding comparison.
func (e *ClusterTuningEngine) NormalizeText(text string) string {
	if text == "" {
		return ""
	}
	lower := strings.ToLower(text)
	var sb strings.Builder
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune(' ')
		}
	}
	words := strings.Fields(sb.String())
	return strings.Join(words, " ")
}

// ClassifyFeedback determines feedback type based on reaction counts and status flags.
func (e *ClusterTuningEngine) ClassifyFeedback(thumbsUp, thumbsDown int, implemented, dismissed bool) FeedbackType {
	if implemented {
		return FeedbackSuggestionImplemented
	}
	if dismissed {
		return FeedbackDismissed
	}
	if thumbsUp > 0 && thumbsUp > thumbsDown {
		return FeedbackPositiveReaction
	}
	if thumbsDown > 0 && thumbsDown > thumbsUp {
		return FeedbackNegativeReaction
	}
	return FeedbackNeutral
}

// CosineSimilarity computes cosine similarity between two float64 vectors.
func (e *ClusterTuningEngine) CosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
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

// CosineDistance computes 1.0 - CosineSimilarity.
func (e *ClusterTuningEngine) CosineDistance(a, b []float64) float64 {
	sim := e.CosineSimilarity(a, b)
	if sim > 1.0 {
		sim = 1.0
	} else if sim < -1.0 {
		sim = -1.0
	}
	return 1.0 - sim
}

// ClusterizeSuggestions groups historical embedded suggestions into K clusters via Lloyd's k-means.
func (e *ClusterTuningEngine) ClusterizeSuggestions(
	suggestions []*EmbeddedSuggestion,
) ([]*ClusterizedSuggestion, map[int]*ClusterCentroid, error) {
	e.mu.RLock()
	maxK := e.maxClusters
	divisor := e.divisorForClusterQuantity
	maxIter := e.maxIterations
	tol := e.convergenceTolerance
	e.mu.RUnlock()

	n := len(suggestions)
	if n == 0 {
		return nil, nil, errors.New("cannot clusterize empty suggestion set")
	}

	// Validate dimension consistency
	dim := len(suggestions[0].Embedding)
	if dim == 0 {
		return nil, nil, errors.New("suggestion embeddings must have non-zero dimension")
	}
	for i, s := range suggestions {
		if len(s.Embedding) != dim {
			return nil, nil, fmt.Errorf("inconsistent embedding dimension at index %d: expected %d, got %d", i, dim, len(s.Embedding))
		}
	}

	// Determine optimal cluster count K
	k := n / divisor
	if k < 2 {
		k = 2
	}
	if k > maxK {
		k = maxK
	}
	if k > n {
		k = n
	}

	// 1. Initialize centroids via k-means++ initialization
	centroids := e.initializeKMeansPlusPlus(suggestions, k, dim)

	assignments := make([]int, n)
	for i := range assignments {
		assignments[i] = -1
	}

	// 2. Iterative optimization loop (Lloyd-Forgy)
	for iter := 0; iter < maxIter; iter++ {
		changed := false

		// Expectation step: assign each point to nearest centroid (highest cosine similarity)
		for i, s := range suggestions {
			bestCluster := 0
			bestSim := -2.0

			for cIdx := 0; cIdx < k; cIdx++ {
				sim := e.CosineSimilarity(s.Embedding, centroids[cIdx])
				if sim > bestSim {
					bestSim = sim
					bestCluster = cIdx
				}
			}

			if assignments[i] != bestCluster {
				assignments[i] = bestCluster
				changed = true
			}
		}

		// Maximization step: recompute centroids
		newCentroids := make([][]float64, k)
		counts := make([]int, k)
		for cIdx := 0; cIdx < k; cIdx++ {
			newCentroids[cIdx] = make([]float64, dim)
		}

		for i, s := range suggestions {
			cIdx := assignments[i]
			counts[cIdx]++
			for d := 0; d < dim; d++ {
				newCentroids[cIdx][d] += s.Embedding[d]
			}
		}

		var maxShift float64
		for cIdx := 0; cIdx < k; cIdx++ {
			if counts[cIdx] > 0 {
				invCount := 1.0 / float64(counts[cIdx])
				for d := 0; d < dim; d++ {
					newCentroids[cIdx][d] *= invCount
				}
				// Re-normalize to unit hypersphere for cosine space
				var norm float64
				for d := 0; d < dim; d++ {
					norm += newCentroids[cIdx][d] * newCentroids[cIdx][d]
				}
				if norm > 0 {
					invNorm := 1.0 / math.Sqrt(norm)
					for d := 0; d < dim; d++ {
						newCentroids[cIdx][d] *= invNorm
					}
				}
			} else {
				// Re-seed empty cluster with random observation
				randIdx := e.randIntn(n)
				copy(newCentroids[cIdx], suggestions[randIdx].Embedding)
			}

			shift := e.CosineDistance(centroids[cIdx], newCentroids[cIdx])
			if shift > maxShift {
				maxShift = shift
			}
		}

		centroids = newCentroids
		if !changed || maxShift < tol {
			break
		}
	}

	// 3. Build clusterized output and centroid summary records
	clusterized := make([]*ClusterizedSuggestion, n)
	centroidMap := make(map[int]*ClusterCentroid, k)

	for cIdx := 0; cIdx < k; cIdx++ {
		centroidMap[cIdx] = &ClusterCentroid{
			ClusterID: cIdx,
			Centroid:  centroids[cIdx],
		}
	}

	for i, s := range suggestions {
		cIdx := assignments[i]
		sim := e.CosineSimilarity(s.Embedding, centroids[cIdx])
		dist := 1.0 - sim

		item := &ClusterizedSuggestion{
			Cluster:              cIdx,
			OriginalSuggestion:   s,
			DistanceToCentroid:   dist,
			SimilarityToCentroid: sim,
		}
		clusterized[i] = item

		cm := centroidMap[cIdx]
		cm.MemberCount++
		cm.Inertia += dist * dist

		if s.FeedbackType == FeedbackPositiveReaction || s.FeedbackType == FeedbackSuggestionImplemented {
			cm.PositiveCount++
		} else if s.FeedbackType == FeedbackNegativeReaction || s.FeedbackType == FeedbackDismissed {
			cm.NegativeCount++
		}
	}

	return clusterized, centroidMap, nil
}

// initializeKMeansPlusPlus implements the k-means++ seeded initialization for faster convergence.
func (e *ClusterTuningEngine) initializeKMeansPlusPlus(suggestions []*EmbeddedSuggestion, k, dim int) [][]float64 {
	n := len(suggestions)
	centroids := make([][]float64, k)

	// 1. Pick first centroid uniformly at random
	firstIdx := e.randIntn(n)
	centroids[0] = make([]float64, dim)
	copy(centroids[0], suggestions[firstIdx].Embedding)

	distances := make([]float64, n)

	// 2. Choose remaining k-1 centroids with probability proportional to D(x)^2
	for cIdx := 1; cIdx < k; cIdx++ {
		var sumDistSq float64
		for i := 0; i < n; i++ {
			// Distance to nearest existing centroid
			minDist := math.MaxFloat64
			for prev := 0; prev < cIdx; prev++ {
				d := e.CosineDistance(suggestions[i].Embedding, centroids[prev])
				if d < minDist {
					minDist = d
				}
			}
			distances[i] = minDist * minDist
			sumDistSq += distances[i]
		}

		// Roulette wheel selection
		target := e.randFloat64() * sumDistSq
		var cumulative float64
		chosenIdx := n - 1
		for i := 0; i < n; i++ {
			cumulative += distances[i]
			if cumulative >= target {
				chosenIdx = i
				break
			}
		}

		centroids[cIdx] = make([]float64, dim)
		copy(centroids[cIdx], suggestions[chosenIdx].Embedding)
	}

	return centroids
}

// CompareCandidateWithClusters evaluates a single candidate finding against existing clusters.
func (e *ClusterTuningEngine) CompareCandidateWithClusters(
	finding *models.CodeFinding,
	candidateEmbedding []float64,
	clusterized []*ClusterizedSuggestion,
	centroids map[int]*ClusterCentroid,
) *ClusterAnalysisResult {
	res := &ClusterAnalysisResult{
		AnalyzedFinding: finding,
		Decision:        DecisionUncertain,
	}

	// 1. Safety Override: DrixyRules and Security are never dropped via clustering heuristics
	if finding != nil {
		catUpper := strings.ToUpper(finding.Category)
		if catUpper == string(LabelDrixyRules) || catUpper == string(LabelSecurity) || catUpper == string(LabelBreakingChanges) {
			res.Decision = DecisionKeep
			res.Justification = "Preserved by high-priority category rule (never pruned by clustering)"
			return res
		}
	}

	if len(candidateEmbedding) == 0 || len(clusterized) == 0 || len(centroids) == 0 {
		res.Decision = DecisionKeep
		res.Justification = "No historical cluster context available; keeping candidate"
		return res
	}

	// 2. Find closest cluster centroid by cosine similarity
	bestClusterID := -1
	bestSim := -2.0

	for cID, cm := range centroids {
		sim := e.CosineSimilarity(candidateEmbedding, cm.Centroid)
		if sim > bestSim {
			bestSim = sim
			bestClusterID = cID
		}
	}

	res.NearestClusterID = bestClusterID
	res.ClusterSimilarity = bestSim

	// 3. If similarity to nearest centroid is below threshold, observation is distinct (uncertain -> keep)
	if bestSim < e.simThreshCluster {
		res.Decision = DecisionKeep
		res.Justification = fmt.Sprintf("Candidate is outside historical cluster bounds (similarity %.3f < %.2f)", bestSim, e.simThreshCluster)
		return res
	}

	// 4. Gather member suggestions in the best cluster
	var members []*ClusterizedSuggestion
	for _, cs := range clusterized {
		if cs.Cluster == bestClusterID {
			members = append(members, cs)
		}
	}

	if len(members) == 0 {
		res.Decision = DecisionKeep
		res.Justification = "Matched cluster contains zero active members"
		return res
	}

	// 5. Unanimous cluster check
	allPositive := true
	allNegative := true
	for _, m := range members {
		fb := m.OriginalSuggestion.FeedbackType
		if fb == FeedbackPositiveReaction || fb == FeedbackSuggestionImplemented {
			allNegative = false
		} else if fb == FeedbackNegativeReaction || fb == FeedbackDismissed {
			allPositive = false
		} else {
			allPositive = false
			allNegative = false
		}
	}

	if allPositive {
		res.Decision = DecisionKeep
		res.Justification = "Cluster members exhibit unanimous positive acceptance"
		return res
	}
	if allNegative {
		res.Decision = DecisionDiscard
		res.Justification = "Cluster members exhibit unanimous rejection / historical false positive dismissal"
		return res
	}

	// 6. Granular similarity-weighted voting within cluster
	type scoredMember struct {
		isPositive bool
		similarity float64
	}
	scored := make([]scoredMember, 0, len(members))
	for _, m := range members {
		sim := e.CosineSimilarity(candidateEmbedding, m.OriginalSuggestion.Embedding)
		fb := m.OriginalSuggestion.FeedbackType
		isPos := (fb == FeedbackPositiveReaction || fb == FeedbackSuggestionImplemented)
		scored = append(scored, scoredMember{
			isPositive: isPos,
			similarity: sim,
		})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].similarity > scored[j].similarity
	})

	var positiveVotes, negativeVotes float64
	for _, sm := range scored {
		if sm.isPositive && sm.similarity >= e.simThreshPositive {
			positiveVotes += sm.similarity
		} else if !sm.isPositive && sm.similarity >= e.simThreshNegative {
			negativeVotes += sm.similarity
		}
	}

	res.PositiveScore = positiveVotes
	res.NegativeScore = negativeVotes

	if negativeVotes > positiveVotes && negativeVotes >= e.simThreshNegative {
		res.Decision = DecisionDiscard
		res.Justification = fmt.Sprintf("Discarded: Negative feedback similarity (%.3f) exceeds positive (%.3f)", negativeVotes, positiveVotes)
	} else if positiveVotes > negativeVotes && positiveVotes >= e.simThreshPositive {
		res.Decision = DecisionKeep
		res.Justification = fmt.Sprintf("Kept: Positive feedback similarity (%.3f) exceeds negative (%.3f)", positiveVotes, negativeVotes)
	} else {
		res.Decision = DecisionKeep
		res.Justification = "Ambiguous feedback distribution; defaulting to keep candidate finding"
	}

	return res
}

// FilterFindingsBatch analyzes a batch of candidate findings against historical clusters.
func (e *ClusterTuningEngine) FilterFindingsBatch(
	ctx context.Context,
	findings []models.CodeFinding,
	embeddings [][]float64,
	historical []*EmbeddedSuggestion,
) ([]models.CodeFinding, []models.CodeFinding, []*ClusterAnalysisResult, error) {
	if len(findings) == 0 {
		return nil, nil, nil, nil
	}
	if len(embeddings) != len(findings) {
		return nil, nil, nil, fmt.Errorf("findings count (%d) does not match embeddings count (%d)", len(findings), len(embeddings))
	}

	// If insufficient historical data (< 2 items), keep all findings
	if len(historical) < 2 {
		var kept []models.CodeFinding
		results := make([]*ClusterAnalysisResult, len(findings))
		for i := range findings {
			kept = append(kept, findings[i])
			results[i] = &ClusterAnalysisResult{
				AnalyzedFinding: &findings[i],
				Decision:        DecisionKeep,
				Justification:   "Insufficient historical samples for clustering",
			}
		}
		return kept, nil, results, nil
	}

	clusterized, centroids, err := e.ClusterizeSuggestions(historical)
	if err != nil {
		// On clustering error, fail-open to preserve candidate findings
		var kept []models.CodeFinding
		results := make([]*ClusterAnalysisResult, len(findings))
		for i := range findings {
			kept = append(kept, findings[i])
			results[i] = &ClusterAnalysisResult{
				AnalyzedFinding: &findings[i],
				Decision:        DecisionKeep,
				Justification:   fmt.Sprintf("Clustering fallback: %v", err),
			}
		}
		return kept, nil, results, nil
	}

	var kept []models.CodeFinding
	var discarded []models.CodeFinding
	results := make([]*ClusterAnalysisResult, len(findings))

	for i := range findings {
		select {
		case <-ctx.Done():
			return nil, nil, nil, ctx.Err()
		default:
		}

		res := e.CompareCandidateWithClusters(&findings[i], embeddings[i], clusterized, centroids)
		results[i] = res

		if res.Decision == DecisionDiscard {
			discarded = append(discarded, findings[i])
		} else {
			kept = append(kept, findings[i])
		}
	}

	return kept, discarded, results, nil
}
