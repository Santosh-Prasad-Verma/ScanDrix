package verifier

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/checker"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiffGate_ValidateFindingDiff(t *testing.T) {
	syntaxValidator := checker.NewASTSyntaxValidator()
	gate := NewDiffGate(syntaxValidator)
	ctx := context.Background()

	fileContent := `package main

import "fmt"

func processUser(id int) {
	fmt.Println("Processing user", id)
}
`

	t.Run("Valid Committable Go Suggestion", func(t *testing.T) {
		finding := models.CodeFinding{
			ID:            uuid.New(),
			FilePath:      "user.go",
			StartLine:     6,
			EndLine:       6,
			Severity:      models.SeverityMedium,
			Category:      "bug",
			SuggestedDiff: "\tif id <= 0 {\n\t\treturn\n\t}\n\tfmt.Println(\"Processing user\", id)",
		}

		res := gate.ValidateFindingDiff(ctx, finding, fileContent)
		assert.Equal(t, DecisionCommittable, res.Decision)
		assert.True(t, res.IsCommittable)
		assert.GreaterOrEqual(t, res.ConfidenceScore, 0.90)
		assert.NotEmpty(t, res.PatchedContent)
	})

	t.Run("No-op Suggestion Rejection", func(t *testing.T) {
		finding := models.CodeFinding{
			ID:            uuid.New(),
			FilePath:      "user.go",
			StartLine:     6,
			EndLine:       6,
			Severity:      models.SeverityMedium,
			Category:      "bug",
			SuggestedDiff: "\tfmt.Println(\"Processing user\", id)", // Exactly identical to existing line 6
		}

		res := gate.ValidateFindingDiff(ctx, finding, fileContent)
		assert.Equal(t, DecisionRejected, res.Decision)
		assert.False(t, res.IsCommittable)
		assert.Contains(t, res.RejectionReason, "no-op")
	})

	t.Run("Cosmetic Comments on Security Finding Rejection", func(t *testing.T) {
		finding := models.CodeFinding{
			ID:            uuid.New(),
			FilePath:      "user.go",
			StartLine:     6,
			EndLine:       6,
			Severity:      models.SeverityCritical,
			Category:      "security",
			SuggestedDiff: "// TODO: check ID validity here",
		}

		res := gate.ValidateFindingDiff(ctx, finding, fileContent)
		assert.Equal(t, DecisionRejected, res.Decision)
		assert.False(t, res.IsCommittable)
		assert.Contains(t, res.RejectionReason, "cosmetic")
	})

	t.Run("Syntax Error Downgraded to Advisory", func(t *testing.T) {
		finding := models.CodeFinding{
			ID:            uuid.New(),
			FilePath:      "user.go",
			StartLine:     6,
			EndLine:       6,
			Severity:      models.SeverityHigh,
			Category:      "bug",
			SuggestedDiff: "if id <= 0 { return", // broken syntax unclosed brace
		}

		res := gate.ValidateFindingDiff(ctx, finding, fileContent)
		assert.Equal(t, DecisionAdvisory, res.Decision)
		assert.False(t, res.IsCommittable)
		assert.Contains(t, res.AdvisoryReason, "syntax error")
	})

	t.Run("Exceeds Max Line Threshold Downgraded to Advisory", func(t *testing.T) {
		manyLines := ""
		for i := 0; i < 25; i++ {
			manyLines += "fmt.Println(\"line\")\n"
		}
		finding := models.CodeFinding{
			ID:            uuid.New(),
			FilePath:      "user.go",
			StartLine:     6,
			EndLine:       6,
			Severity:      models.SeverityMedium,
			Category:      "performance",
			SuggestedDiff: manyLines,
		}

		res := gate.ValidateFindingDiff(ctx, finding, fileContent)
		assert.Equal(t, DecisionAdvisory, res.Decision)
		assert.False(t, res.IsCommittable)
		assert.Contains(t, res.AdvisoryReason, "exceeds committable threshold")
	})

	t.Run("Empty Code Snippet Advisory Only", func(t *testing.T) {
		finding := models.CodeFinding{
			ID:          uuid.New(),
			FilePath:    "user.go",
			StartLine:   6,
			EndLine:     6,
			Severity:    models.SeverityInfo,
			Category:    "style",
			Remediation: "",
		}

		res := gate.ValidateFindingDiff(ctx, finding, fileContent)
		assert.Equal(t, DecisionAdvisory, res.Decision)
		assert.False(t, res.IsCommittable)
	})
}

func TestDiffGate_ValidateFindingsBatch(t *testing.T) {
	syntaxValidator := checker.NewASTSyntaxValidator()
	gate := NewDiffGate(syntaxValidator)
	ctx := context.Background()

	fileContent := "package main\nfunc run() {}\n"
	fileContexts := map[string]string{
		"app.go": fileContent,
	}

	findings := []models.CodeFinding{
		{
			ID:            uuid.New(),
			FilePath:      "app.go",
			StartLine:     2,
			EndLine:       2,
			Severity:      models.SeverityHigh,
			Category:      "bug",
			SuggestedDiff: "func run() { println(\"ok\") }",
		},
		{
			ID:            uuid.New(),
			FilePath:      "app.go",
			StartLine:     2,
			EndLine:       2,
			Severity:      models.SeverityLow,
			Category:      "style",
			SuggestedDiff: "func run() {}\n", // no-op
		},
	}

	kept, results := gate.ValidateFindingsBatch(ctx, findings, fileContexts)
	require.Len(t, kept, 1)
	assert.Equal(t, findings[0].ID, kept[0].ID)
	assert.Len(t, results, 2)
	assert.Equal(t, DecisionCommittable, results[findings[0].ID.String()].Decision)
	assert.Equal(t, DecisionRejected, results[findings[1].ID.String()].Decision)
}
