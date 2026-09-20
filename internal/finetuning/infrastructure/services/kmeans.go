// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Fine-Tuning Subsystem
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package services

import (
	"errors"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/finetuning/domain/interfaces"
)

// KMeansOptions configures the k-means clustering process.
type KMeansOptions struct {
	MaxIterations  int
	Seed           int64
	Initialization string // "kmeans++" or "random"
}

// KMeansResult holds the clustering outcome.
type KMeansResult struct {
	Clusters   []int       // Cluster index for each input vector
	Centroids  [][]float64 // Centroid coordinates for each cluster
	Iterations int
}

var (
	ErrEmptyInputVectors = errors.New("cannot cluster empty set of vectors")
	ErrInvalidK          = errors.New("k must be greater than 0")
	ErrDimensionMismatch = errors.New("all vectors must have identical dimensions")
)

// CosineSimilarity calculates the cosine similarity between two float64 vectors.
func CosineSimilarity(vecA, vecB []float64) float64 {
	if len(vecA) == 0 || len(vecB) == 0 || len(vecA) != len(vecB) {
		return 0.0
	}

	var dotProduct, normA, normB float64
	for i := 0; i < len(vecA); i++ {
		dotProduct += vecA[i] * vecB[i]
	}
	for i := 0; i < len(vecA); i++ {
		normA += vecA[i] * vecA[i]
		normB += vecB[i] * vecB[i]
	}

	if normA <= 0.0 || normB <= 0.0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// SquaredDistance computes the squared Euclidean distance between two vectors.
func SquaredDistance(a, b []float64) float64 {
	var sum float64
	for i := 0; i < len(a); i++ {
		diff := a[i] - b[i]
		sum += diff * diff
	}
	return sum
}

// KMeans executes k-means clustering with k-means++ centroid initialization.
func KMeans(vectors [][]float64, k int, opts *KMeansOptions) (*KMeansResult, error) {
	n := len(vectors)
	if n == 0 {
		return nil, ErrEmptyInputVectors
	}
	if k <= 0 {
		return nil, ErrInvalidK
	}

	dim := len(vectors[0])
	if dim == 0 {
		return nil, ErrEmptyInputVectors
	}
	for _, v := range vectors {
		if len(v) != dim {
			return nil, ErrDimensionMismatch
		}
	}

	if k > n {
		k = n
	}

	maxIters := 1
	var rng *rand.Rand
	if opts != nil {
		if opts.MaxIterations > 0 {
			maxIters = opts.MaxIterations
		}
		if opts.Seed != 0 {
			rng = rand.New(rand.NewSource(opts.Seed))
		}
	}
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	// 1. Initialize centroids using k-means++
	centroids := initKMeansPlusPlus(vectors, k, rng)

	clusters := make([]int, n)
	iterations := 0

	for iter := 0; iter < maxIters; iter++ {
		iterations++
		changed := false

		// Assignment step: assign each vector to the closest centroid
		for i, v := range vectors {
			bestCluster := 0
			bestDist := math.MaxFloat64

			for cIdx, centroid := range centroids {
				dist := SquaredDistance(v, centroid)
				if dist < bestDist {
					bestDist = dist
					bestCluster = cIdx
				}
			}

			if clusters[i] != bestCluster {
				clusters[i] = bestCluster
				changed = true
			}
		}

		// If no assignments changed or it's the last iteration, break
		if !changed || iter == maxIters-1 {
			break
		}

		// Update step: recompute centroids as mean of cluster members
		counts := make([]int, k)
		newCentroids := make([][]float64, k)
		for c := 0; c < k; c++ {
			newCentroids[c] = make([]float64, dim)
		}

		for i, c := range clusters {
			counts[c]++
			for d := 0; d < dim; d++ {
				newCentroids[c][d] += vectors[i][d]
			}
		}

		for c := 0; c < k; c++ {
			if counts[c] > 0 {
				for d := 0; d < dim; d++ {
					newCentroids[c][d] /= float64(counts[c])
				}
				centroids[c] = newCentroids[c]
			}
		}
	}

	return &KMeansResult{
		Clusters:   clusters,
		Centroids:  centroids,
		Iterations: iterations,
	}, nil
}

// initKMeansPlusPlus selects k centroids using the k-means++ probability distribution.
func initKMeansPlusPlus(vectors [][]float64, k int, rng *rand.Rand) [][]float64 {
	n := len(vectors)
	dim := len(vectors[0])

	centroids := make([][]float64, k)

	// 1. Choose first centroid uniformly at random
	firstIdx := rng.Intn(n)
	centroids[0] = make([]float64, dim)
	copy(centroids[0], vectors[firstIdx])

	if k == 1 {
		return centroids
	}

	dists := make([]float64, n)

	// 2. Choose remaining k-1 centroids
	for c := 1; c < k; c++ {
		var totalDistSq float64

		for i, v := range vectors {
			minDistSq := math.MaxFloat64
			for j := 0; j < c; j++ {
				d := SquaredDistance(v, centroids[j])
				if d < minDistSq {
					minDistSq = d
				}
			}
			dists[i] = minDistSq
			totalDistSq += minDistSq
		}

		if totalDistSq <= 0 {
			// All remaining points coincide with existing centroids, pick arbitrary point
			randIdx := rng.Intn(n)
			centroids[c] = make([]float64, dim)
			copy(centroids[c], vectors[randIdx])
			continue
		}

		// Roulette wheel selection based on D(x)^2
		target := rng.Float64() * totalDistSq
		var cumulative float64
		selectedIdx := n - 1

		for i, d := range dists {
			cumulative += d
			if cumulative >= target {
				selectedIdx = i
				break
			}
		}

		centroids[c] = make([]float64, dim)
		copy(centroids[c], vectors[selectedIdx])
	}

	return centroids
}

// CalculateClusterCentroids groups suggestions by cluster ID and calculates each centroid.
func CalculateClusterCentroids(suggestions []interfaces.ClusterizedSuggestion) map[int][]float64 {
	clusters := make(map[int][][]float64)

	for _, s := range suggestions {
		embed := s.OriginalSuggestion.SuggestionEmbed
		if len(embed) == 0 {
			continue
		}
		clusters[s.Cluster] = append(clusters[s.Cluster], embed)
	}

	centroids := make(map[int][]float64)
	for clusterID, embeddings := range clusters {
		if len(embeddings) == 0 {
			continue
		}
		dim := len(embeddings[0])
		centroid := make([]float64, dim)

		for _, emb := range embeddings {
			for i := 0; i < dim; i++ {
				centroid[i] += emb[i]
			}
		}

		count := float64(len(embeddings))
		for i := 0; i < dim; i++ {
			centroid[i] /= count
		}

		centroids[clusterID] = centroid
	}

	return centroids
}

// ThreadSafeKMeansRunner wraps KMeans for concurrent fine-tuning operations.
type ThreadSafeKMeansRunner struct {
	mu sync.RWMutex
}

func NewThreadSafeKMeansRunner() *ThreadSafeKMeansRunner {
	return &ThreadSafeKMeansRunner{}
}

func (r *ThreadSafeKMeansRunner) Cluster(vectors [][]float64, k int, opts *KMeansOptions) (*KMeansResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return KMeans(vectors, k, opts)
}
