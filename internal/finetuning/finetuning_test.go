// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Fine-Tuning Subsystem Tests
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package finetuning_test

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scandrix/backend/internal/finetuning"
	reviewdomain "github.com/scandrix/backend/internal/review/domain"
)

// ─────────────────────────────────────────────────────────────
// 1. Dataset Builder Tests (Preserved & Enhanced)
// ─────────────────────────────────────────────────────────────

func TestFineTuningDatasetBuilder(t *testing.T) {
	builder := finetuning.NewFineTuningDatasetBuilder()
	wsID := uuid.New()

	// 1. Accepted sample
	builder.RecordSample(finetuning.ReviewSuggestionTrainingSample{
		ID:                 uuid.New(),
		WorkspaceID:        wsID,
		SystemPrompt:       "You are an expert security code reviewer.",
		InputCodeDiff:      "- query := fmt.Sprintf(\"SELECT * FROM users WHERE id = '%s'\", id)\n+ query := \"SELECT * FROM users WHERE id = $1\"",
		ExpectedSuggestion: "Use parameterized query placeholders to prevent SQL injection (CWE-89).",
		Status:             finetuning.FeedbackAccepted,
		CapturedAt:         time.Now().UTC(),
	})

	// 2. Rejected sample (developer flagged as false positive)
	builder.RecordSample(finetuning.ReviewSuggestionTrainingSample{
		ID:                 uuid.New(),
		WorkspaceID:        wsID,
		SystemPrompt:       "You are an expert security code reviewer.",
		InputCodeDiff:      "+ log.Printf(\"Processed payment for order %s\", orderID)",
		ExpectedSuggestion: "Sensitive data exposure in logs",
		Status:             finetuning.FeedbackRejected,
		CapturedAt:         time.Now().UTC(),
	})

	var buf bytes.Buffer
	count, err := builder.ExportJSONL(&buf)
	require.NoError(t, err)

	// Should export exactly 1 sample (accepted only)
	assert.Equal(t, 1, count)

	line := strings.TrimSpace(buf.String())
	var entry finetuning.ChatFineTuningEntry
	require.NoError(t, json.Unmarshal([]byte(line), &entry))

	assert.Len(t, entry.Messages, 3)
	assert.Equal(t, "system", entry.Messages[0].Role)
	assert.Equal(t, "user", entry.Messages[1].Role)
	assert.Equal(t, "assistant", entry.Messages[2].Role)
}

// ─────────────────────────────────────────────────────────────
// 2. Pure Go K-Means & Cosine Similarity Tests
// ─────────────────────────────────────────────────────────────

func TestCosineSimilarity(t *testing.T) {
	t.Run("Identical vectors", func(t *testing.T) {
		v1 := []float64{1.0, 2.0, 3.0}
		v2 := []float64{1.0, 2.0, 3.0}
		sim := finetuning.CosineSimilarity(v1, v2)
		assert.InDelta(t, 1.0, sim, 0.0001)
	})

	t.Run("Orthogonal vectors", func(t *testing.T) {
		v1 := []float64{1.0, 0.0}
		v2 := []float64{0.0, 1.0}
		sim := finetuning.CosineSimilarity(v1, v2)
		assert.InDelta(t, 0.0, sim, 0.0001)
	})

	t.Run("Opposing vectors", func(t *testing.T) {
		v1 := []float64{1.0, 0.0}
		v2 := []float64{-1.0, 0.0}
		sim := finetuning.CosineSimilarity(v1, v2)
		assert.InDelta(t, -1.0, sim, 0.0001)
	})

	t.Run("Zero and empty vectors", func(t *testing.T) {
		assert.Equal(t, 0.0, finetuning.CosineSimilarity(nil, nil))
		assert.Equal(t, 0.0, finetuning.CosineSimilarity([]float64{1.0}, []float64{1.0, 2.0}))
		assert.Equal(t, 0.0, finetuning.CosineSimilarity([]float64{0.0, 0.0}, []float64{1.0, 1.0}))
	})
}

func TestKMeans(t *testing.T) {
	t.Run("Two distinct clusters in 2D", func(t *testing.T) {
		vectors := [][]float64{
			{1.0, 1.0},
			{1.1, 0.9},
			{0.9, 1.1},
			{10.0, 10.0},
			{10.1, 9.9},
			{9.9, 10.1},
		}

		res, err := finetuning.KMeans(vectors, 2, &finetuning.KMeansOptions{
			MaxIterations:  10,
			Seed:           42,
			Initialization: "kmeans++",
		})
		require.NoError(t, err)
		require.NotNil(t, res)
		assert.Len(t, res.Clusters, 6)
		assert.Len(t, res.Centroids, 2)

		// First 3 should belong to same cluster, last 3 to another
		assert.Equal(t, res.Clusters[0], res.Clusters[1])
		assert.Equal(t, res.Clusters[0], res.Clusters[2])
		assert.Equal(t, res.Clusters[3], res.Clusters[4])
		assert.Equal(t, res.Clusters[3], res.Clusters[5])
		assert.NotEqual(t, res.Clusters[0], res.Clusters[3])
	})

	t.Run("Invalid inputs", func(t *testing.T) {
		_, err := finetuning.KMeans(nil, 2, nil)
		assert.Error(t, err)

		_, err = finetuning.KMeans([][]float64{{1.0}}, 0, nil)
		assert.Error(t, err)

		_, err = finetuning.KMeans([][]float64{{1.0, 2.0}, {1.0}}, 2, nil)
		assert.Error(t, err)
	})

	t.Run("Clamping k to vector count", func(t *testing.T) {
		vectors := [][]float64{{1.0, 2.0}, {3.0, 4.0}}
		res, err := finetuning.KMeans(vectors, 10, nil)
		require.NoError(t, err)
		assert.Len(t, res.Centroids, 2)
	})
}

// ─────────────────────────────────────────────────────────────
// 3. SuggestionEmbedded Database Repository Tests
// ─────────────────────────────────────────────────────────────

func TestSuggestionEmbeddedRepository(t *testing.T) {
	ctx := context.Background()
	repo := finetuning.NewSuggestionEmbeddedDatabaseRepository()
	orgID := uuid.New().String()
	repoID := "repo-123"

	// 1. Create single
	item := finetuning.SuggestionEmbedded{
		SuggestionID:       "sugg-1",
		SuggestionEmbed:    []float64{0.1, 0.2, 0.3},
		PullRequestNumber:  10,
		RepositoryID:       repoID,
		RepositoryFullName: "scandrix/test-repo",
		Organization:       &finetuning.OrganizationRef{UUID: orgID},
		Label:              "security",
		Severity:           "critical",
		FeedbackType:       string(finetuning.PositiveReaction),
		ImprovedCode:       "safeCode()",
		SuggestionContent:  "Fix vulnerability",
		Language:           "go",
	}

	created, err := repo.Create(ctx, item)
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.Equal(t, "sugg-1", created.SuggestionID())
	assert.Equal(t, "security", created.Label())
	assert.Equal(t, "go", created.Language())

	// 2. Find by SuggestionID
	found, err := repo.FindOne(ctx, "sugg-1")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, created.UUID(), found.UUID())

	// 3. Find with filter
	results, err := repo.Find(ctx, finetuning.SuggestionFilter{
		OrganizationID: orgID,
		Language:       "go",
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)

	// 4. Update
	updated, err := repo.Update(ctx, "sugg-1", finetuning.SuggestionEmbedded{
		FeedbackType: string(finetuning.SuggestionImplemented),
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, string(finetuning.SuggestionImplemented), updated.FeedbackType())

	// 5. BulkInsert & Concurrent stress testing
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			bulkItems := []finetuning.SuggestionEmbedded{
				{
					SuggestionID:       uuid.New().String(),
					SuggestionEmbed:    []float64{float64(idx), 0.5},
					PullRequestNumber:  idx,
					RepositoryID:       repoID,
					RepositoryFullName: "scandrix/test-repo",
					Organization:       &finetuning.OrganizationRef{UUID: orgID},
					Label:              "bug",
					Severity:           "low",
					FeedbackType:       string(finetuning.PositiveReaction),
					Language:           "go",
				},
			}
			_, _ = repo.BulkInsert(ctx, bulkItems)
		}(i)
	}
	wg.Wait()

	allGo, err := repo.Find(ctx, finetuning.SuggestionFilter{OrganizationID: orgID, Language: "go"})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(allGo), 21)
}

// ─────────────────────────────────────────────────────────────
// 4. SuggestionEmbedded Service Tests
// ─────────────────────────────────────────────────────────────

func TestSuggestionEmbeddedService(t *testing.T) {
	ctx := context.Background()
	repo := finetuning.NewSuggestionEmbeddedDatabaseRepository()
	embedder := finetuning.NewDeterministicHasherEmbedder(32)
	svc := finetuning.NewSuggestionEmbeddedService(repo, embedder)

	orgID := uuid.New().String()
	repoID := "repo-xyz"

	t.Run("DeterministicHasherEmbedder properties", func(t *testing.T) {
		vec1, err := embedder.Embed(ctx, "test query")
		require.NoError(t, err)
		assert.Len(t, vec1, 32)

		vec2, err := embedder.Embed(ctx, "test query")
		require.NoError(t, err)
		assert.Equal(t, vec1, vec2, "embeddings for identical text must be strictly deterministic")

		// Verify unit norm (L2 normalized)
		var norm float64
		for _, v := range vec1 {
			norm += v * v
		}
		assert.InDelta(t, 1.0, math.Sqrt(norm), 0.0001)
	})

	t.Run("BulkCreateFromMongoData with feedback counting", func(t *testing.T) {
		toEmbed := []finetuning.SuggestionToEmbed{
			{
				ID:                 uuid.New().String(),
				SuggestionContent:  "Vulnerability in auth check",
				OneSentenceSummary: "Sanitize JWT payload",
				Label:              "security",
				Severity:           "high",
				FeedbackType:       string(finetuning.PositiveReaction),
				ImprovedCode:       "jwt.Verify()",
				Language:           "typescript",
				OrganizationID:     orgID,
				PullRequest: finetuning.SuggestionToEmbed{}.PullRequest,
			},
			{
				ID:                 uuid.New().String(),
				SuggestionContent:  "Typo in variable name",
				OneSentenceSummary: "Fix naming convention",
				Label:              "style",
				Severity:           "low",
				FeedbackType:       string(finetuning.NegativeReaction),
				ImprovedCode:       "userCount",
				Language:           "typescript",
				OrganizationID:     orgID,
				PullRequest: finetuning.SuggestionToEmbed{}.PullRequest,
			},
			{
				ID:                 uuid.New().String(),
				SuggestionContent:  "Dead code block",
				OneSentenceSummary: "Remove unreachable switch case",
				Label:              "optimization",
				Severity:           "medium",
				FeedbackType:       string(finetuning.SuggestionImplemented),
				ImprovedCode:       "// deleted",
				Language:           "python",
				OrganizationID:     orgID,
				PullRequest: finetuning.SuggestionToEmbed{}.PullRequest,
			},
		}
		for i := range toEmbed {
			toEmbed[i].PullRequest.Number = 42
			toEmbed[i].PullRequest.Repository.ID = repoID
			toEmbed[i].PullRequest.Repository.FullName = "scandrix/demo"
		}

		created, err := svc.BulkCreateFromMongoData(ctx, toEmbed)
		require.NoError(t, err)
		assert.Len(t, created, 3)

		// Check organization metrics
		metrics, err := svc.GetByOrganization(ctx, orgID)
		require.NoError(t, err)
		assert.Equal(t, 2, metrics.PositiveFeedbacks) // PositiveReaction + SuggestionImplemented
		assert.Equal(t, 1, metrics.NegativeFeedbacks)
		assert.Equal(t, 3, metrics.Total)

		// Check language metrics
		langMetrics, err := svc.GetByOrganizationWithLanguages(ctx, orgID)
		require.NoError(t, err)
		assert.Equal(t, 3, langMetrics.Total)
		assert.Equal(t, 2, langMetrics.PositiveFeedbacks.Total)
		assert.Equal(t, 1, langMetrics.NegativeFeedbacks.Total)
	})

	t.Run("EmbedSuggestionsForSuggestionToEmbed", func(t *testing.T) {
		suggID := uuid.New()
		suggestions := []*reviewdomain.CodeSuggestion{
			{
				ID:                 suggID,
				SuggestionContent:  "Avoid unescaped SQL fragments",
				OneSentenceSummary: "SQL Injection Prevention",
				Label:              "security",
				Severity:           reviewdomain.SeverityHigh,
				ImprovedCode:       "db.Query($1)",
				Language:           "go",
			},
		}

		embedded, err := svc.EmbedSuggestionsForSuggestionToEmbed(
			ctx,
			suggestions,
			orgID,
			101,
			repoID,
			"scandrix/backend",
		)
		require.NoError(t, err)
		require.Len(t, embedded, 1)
		assert.Equal(t, suggID.String(), embedded[0].ID)
		assert.NotEmpty(t, embedded[0].SuggestionEmbed)
		assert.Equal(t, 101, embedded[0].PullRequest.Number)
	})
}

// ─────────────────────────────────────────────────────────────
// 5. DrixyFineTuningService Full Pipeline Tests
// ─────────────────────────────────────────────────────────────

type mockPRProvider struct {
	prs []finetuning.PullRequestDataModel
}

func (m *mockPRProvider) FindPullRequestsToSync(ctx context.Context, organizationID string, repo finetuning.RepositoryRef) ([]finetuning.PullRequestDataModel, error) {
	return m.prs, nil
}

func (m *mockPRProvider) UpdateSyncedSuggestionsFlag(ctx context.Context, prNumbers []int, repoID string, orgID string, synced bool) error {
	return nil
}

type mockFeedbackProvider struct {
	feedbacks []finetuning.CodeReviewFeedbackModel
}

func (m *mockFeedbackProvider) FindFeedbackByOrgAndRepo(ctx context.Context, organizationID string, repoID string, synced bool) ([]finetuning.CodeReviewFeedbackModel, error) {
	return m.feedbacks, nil
}

func (m *mockFeedbackProvider) UpdateSyncedFeedbackFlag(ctx context.Context, organizationID string, suggestionIDs []string, synced bool) error {
	return nil
}

func TestDrixyFineTuningService(t *testing.T) {
	ctx := context.Background()
	repo := finetuning.NewSuggestionEmbeddedDatabaseRepository()
	embedder := finetuning.NewDeterministicHasherEmbedder(16)
	embeddedSvc := finetuning.NewSuggestionEmbeddedService(repo, embedder)

	prProvider := &mockPRProvider{}
	feedbackProvider := &mockFeedbackProvider{}

	customCfg := &finetuning.FineTuningConfig{
		MaxClusters:                 10,
		DivisorForClusterQuantity:   2,
		PositiveThreshold:           0.5,
		NegativeThreshold:           0.5,
		SimilarityThresholdCluster:  0.4,
		MinSuggestionsForClustering: 5, // Lower for testing
	}

	ftService := finetuning.NewDrixyFineTuningService(
		prProvider,
		feedbackProvider,
		embeddedSvc,
		nil,
		customCfg,
	)

	orgID := uuid.New().String()
	repoRef := finetuning.RepositoryRef{
		ID:       "repo-ft",
		FullName: "scandrix/ft-test",
		Language: "go",
	}

	t.Run("Text normalization", func(t *testing.T) {
		raw := "   Fix: Unchecked   error   IN db.Close()!   "
		normalized := ftService.NormalizeText(raw)
		assert.Equal(t, "fix unchecked error in db.close()", normalized)
	})

	t.Run("Feedback identification", func(t *testing.T) {
		pos := ftService.IdentifyFeedbackType(finetuning.CodeReviewFeedbackModel{
			Reactions: finetuning.CodeReviewFeedbackReactions{ThumbsUp: 5, ThumbsDown: 1},
		})
		assert.Equal(t, finetuning.PositiveReaction, pos)

		neg := ftService.IdentifyFeedbackType(finetuning.CodeReviewFeedbackModel{
			Reactions: finetuning.CodeReviewFeedbackReactions{ThumbsUp: 0, ThumbsDown: 3},
		})
		assert.Equal(t, finetuning.NegativeReaction, neg)

		neutral := ftService.IdentifyFeedbackType(finetuning.CodeReviewFeedbackModel{
			Reactions: finetuning.CodeReviewFeedbackReactions{ThumbsUp: 2, ThumbsDown: 2},
		})
		assert.Equal(t, finetuning.Neutral, neutral)
	})

	t.Run("Special labels rule: drixy_rules and breaking_changes are ALWAYS kept", func(t *testing.T) {
		candidates := []*reviewdomain.CodeSuggestion{
			{
				ID:                 uuid.New(),
				SuggestionContent:  "Custom organization rule violated",
				OneSentenceSummary: "Must use secure logger",
				Label:              "drixy_rules",
				Severity:           reviewdomain.SeverityMedium,
				Language:           "go",
			},
			{
				ID:                 uuid.New(),
				SuggestionContent:  "Public API method signature changed",
				OneSentenceSummary: "Breaking Change Detected",
				Label:              "breaking_changes",
				Severity:           reviewdomain.SeverityCritical,
				Language:           "go",
			},
		}

		// Dummy clusters
		dummyClusters := []finetuning.ClusterizedSuggestion{
			{
				Cluster: 0,
				OriginalSuggestion: finetuning.SuggestionEmbedded{
					SuggestionEmbed: []float64{1.0, 0.0},
					FeedbackType:    string(finetuning.NegativeReaction),
				},
				Language: "go",
			},
		}

		result, err := ftService.FineTuningAnalysis(ctx, orgID, 1, repoRef, candidates, dummyClusters)
		require.NoError(t, err)
		assert.Len(t, result.KeepedSuggestions, 2, "special labels must always be kept")
		assert.Empty(t, result.DiscardedSuggestions)
	})

	t.Run("Unanimous positive cluster keeps suggestions", func(t *testing.T) {
		vec := []float64{0.9, 0.1}
		norm := math.Sqrt(0.9*0.9 + 0.1*0.1)
		vec[0] /= norm
		vec[1] /= norm

		clusters := []finetuning.ClusterizedSuggestion{
			{
				Cluster: 0,
				OriginalSuggestion: finetuning.SuggestionEmbedded{
					SuggestionEmbed: vec,
					FeedbackType:    string(finetuning.PositiveReaction),
				},
				Language: "go",
			},
			{
				Cluster: 0,
				OriginalSuggestion: finetuning.SuggestionEmbedded{
					SuggestionEmbed: vec,
					FeedbackType:    string(finetuning.SuggestionImplemented),
				},
				Language: "go",
			},
		}

		// Add filler clusters to reach MinSuggestionsForClustering = 5
		for i := 0; i < 4; i++ {
			clusters = append(clusters, finetuning.ClusterizedSuggestion{
				Cluster: 0,
				OriginalSuggestion: finetuning.SuggestionEmbedded{
					SuggestionEmbed: vec,
					FeedbackType:    string(finetuning.PositiveReaction),
				},
				Language: "go",
			})
		}

		candidates := []*reviewdomain.CodeSuggestion{
			{
				ID:                 uuid.New(),
				SuggestionContent:  "Very similar positive pattern",
				OneSentenceSummary: "Positive feedback pattern",
				Label:              "security",
				Language:           "go",
			},
		}

		result, err := ftService.FineTuningAnalysis(ctx, orgID, 1, repoRef, candidates, clusters)
		require.NoError(t, err)
		assert.Len(t, result.KeepedSuggestions, 1)
		assert.Empty(t, result.DiscardedSuggestions)
	})

	t.Run("Unanimous negative cluster discards suggestions", func(t *testing.T) {
		// Prepare a negative cluster
		testEmbed, _ := embedder.Embed(ctx, "Unwanted noisy warning Annoying alert noisy")

		var negClusters []finetuning.ClusterizedSuggestion
		for i := 0; i < 6; i++ {
			negClusters = append(negClusters, finetuning.ClusterizedSuggestion{
				Cluster: 0,
				OriginalSuggestion: finetuning.SuggestionEmbedded{
					SuggestionEmbed: testEmbed,
					FeedbackType:    string(finetuning.NegativeReaction),
				},
				Language: "go",
			})
		}

		candidates := []*reviewdomain.CodeSuggestion{
			{
				ID:                 uuid.New(),
				SuggestionContent:  "Unwanted noisy warning",
				OneSentenceSummary: "Annoying alert",
				Label:              "noisy",
				Language:           "go",
			},
		}

		result, err := ftService.FineTuningAnalysis(ctx, orgID, 1, repoRef, candidates, negClusters)
		require.NoError(t, err)
		assert.Len(t, result.DiscardedSuggestions, 1, "negative feedback cluster should discard matching suggestion")
		assert.Empty(t, result.KeepedSuggestions)
	})
}

// ─────────────────────────────────────────────────────────────
// 6. Context Preparation Service Tests
// ─────────────────────────────────────────────────────────────

func TestDrixyFineTuningContextPreparationService(t *testing.T) {
	ctx := context.Background()
	repo := finetuning.NewSuggestionEmbeddedDatabaseRepository()
	embedder := finetuning.NewDeterministicHasherEmbedder(16)
	embeddedSvc := finetuning.NewSuggestionEmbeddedService(repo, embedder)

	ftService := finetuning.NewDrixyFineTuningService(
		nil,
		nil,
		embeddedSvc,
		nil,
		nil,
	)

	prepSvc := finetuning.NewDrixyFineTuningContextPreparationService(ftService)

	repoRef := finetuning.RepositoryRef{ID: "r1", FullName: "scandrix/core", Language: "go"}
	suggestions := []*reviewdomain.CodeSuggestion{
		{ID: uuid.New(), SuggestionContent: "code review finding 1", Language: "go"},
	}

	t.Run("Disabled fine tuning keeps all suggestions", func(t *testing.T) {
		res, err := prepSvc.PrepareDrixyFineTuningContext(
			ctx,
			"org-1",
			10,
			repoRef,
			suggestions,
			false, // disabled
			nil,
		)
		require.NoError(t, err)
		assert.Len(t, res.KeepedSuggestions, 1)
		assert.Empty(t, res.DiscardedSuggestions)
	})

	t.Run("Empty suggestions returns empty", func(t *testing.T) {
		res, err := prepSvc.PrepareDrixyFineTuningContext(
			ctx,
			"org-1",
			10,
			repoRef,
			nil,
			true,
			nil,
		)
		require.NoError(t, err)
		assert.Empty(t, res.KeepedSuggestions)
		assert.Empty(t, res.DiscardedSuggestions)
	})

	t.Run("Empty clusters preserves all suggestions", func(t *testing.T) {
		res, err := prepSvc.PrepareDrixyFineTuningContext(
			ctx,
			"org-1",
			10,
			repoRef,
			suggestions,
			true,
			[]finetuning.ClusterizedSuggestion{},
		)
		require.NoError(t, err)
		assert.Len(t, res.KeepedSuggestions, 1)
		assert.Empty(t, res.DiscardedSuggestions)
	})
}
