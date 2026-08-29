package verifier_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/verifier"
)

func TestSecondPassVerifier(t *testing.T) {
	v := verifier.NewSecondPassVerifier()
	ctx := context.Background()

	// 1. False Positive: SQL injection on parameterized query
	fpSQL := verifier.CandidateFinding{
		ID:          uuid.New(),
		FilePath:    "internal/db/users.go",
		LineNumber:  45,
		Title:       "Potential SQL Injection",
		Snippet:     `db.QueryRowContext(ctx, "SELECT id FROM users WHERE email = $1", userEmail)`,
		Severity:    "high",
		Description: "Raw query execution detected",
	}

	verdictFP := v.VerifyFinding(ctx, fpSQL, "")
	if verdictFP.Keep {
		t.Fatal("expected verifier to REFUTE parameterized query false positive")
	}

	// 2. Genuine Positive: SQL injection using string concatenation
	tpSQL := verifier.CandidateFinding{
		ID:          uuid.New(),
		FilePath:    "internal/db/orders.go",
		LineNumber:  82,
		Title:       "CWE-89: SQL Injection Vulnerability",
		Snippet:     `db.Query("SELECT * FROM orders WHERE id = '" + orderID + "'")`,
		Severity:    "critical",
		Description: "Unsanitized user variable directly concatenated into SQL query",
	}

	verdictTP := v.VerifyFinding(ctx, tpSQL, "")
	if !verdictTP.Keep || verdictTP.Confidence != "high" {
		t.Fatalf("expected verifier to CONFIRM true positive vulnerability, got %+v", verdictTP)
	}

	// 3. Batch Filtering
	candidates := []verifier.CandidateFinding{fpSQL, tpSQL}
	contexts := map[string]string{
		"internal/db/users.go":  fpSQL.Snippet,
		"internal/db/orders.go": tpSQL.Snippet,
	}

	verified := v.FilterCandidates(ctx, candidates, contexts)
	if len(verified) != 1 || verified[0].ID != tpSQL.ID {
		t.Fatalf("expected batch filter to keep only true positive, got %d items", len(verified))
	}
}
