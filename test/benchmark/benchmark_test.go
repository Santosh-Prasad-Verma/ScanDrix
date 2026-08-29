package benchmark_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/agents/deliberation"
	"github.com/scandrix/backend/internal/cache/limiter"
	"github.com/scandrix/backend/internal/codeanalysis/ast"
	"github.com/scandrix/backend/internal/queue/relay"
	"github.com/scandrix/backend/internal/webhooks/ingestion"
	"github.com/scandrix/backend/pkg/models"
)

func BenchmarkHMACVerification(b *testing.B) {
	verifier := ingestion.NewWebhookVerifier()
	secret := "test_webhook_secret_key_12345"
	payload := []byte(`{"action":"opened","number":42,"repository":{"full_name":"acme/core"}}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !verifier.VerifyGitHub(sig, payload, secret) {
			b.Fatal("hmac verification failed")
		}
	}
}

func BenchmarkASTAnalysis(b *testing.B) {
	analyzer := ast.NewASTComplexityAnalyzer()
	sampleSrc := `package main
func Compute(x int) int {
	if x > 10 {
		for i := 0; i < x; i++ {
			if i%2 == 0 {
				x += i
			}
		}
	}
	return x
}
`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rep, err := analyzer.Analyze("sample.go", sampleSrc)
		if err != nil || len(rep.Functions) == 0 {
			b.Fatal("ast analysis failed")
		}
	}
}

func BenchmarkHalsteadMetrics(b *testing.B) {
	sampleSrc := `package test
func Factorial(n int) int {
	if n <= 1 {
		return 1
	}
	return n * Factorial(n-1)
}
`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics := ast.CalculateHalstead(sampleSrc)
		if metrics.Volume <= 0 {
			b.Fatal("halstead volume computation failed")
		}
	}
}

func BenchmarkTokenBucket(b *testing.B) {
	tb := limiter.NewTokenBucketLimiter(limiter.RateLimitConfig{
		Capacity:        100000,
		RefillRatePerSec: 100000,
	})
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = tb.Allow(ctx, "tenant-bench", 1)
		}
	})
}

func BenchmarkOutboxInboxClaim(b *testing.B) {
	inbox := relay.NewInboxDeduplicator()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msgID := uuid.New().String()
		claimed, err := inbox.ClaimMessage(ctx, msgID, "worker-bench")
		if err != nil || !claimed {
			b.Fatal("claim failed")
		}
	}
}

func BenchmarkConsensusAdjudication(b *testing.B) {
	deliberator := deliberation.NewAgentDeliberator(0.70)
	ctx := context.Background()

	cand1 := deliberation.ProposeFinding(
		deliberation.PersonaSecurityAuditor,
		"internal/auth.go", 10, 12,
		"Insecure direct object reference",
		models.SeverityHigh, 0.90, "Reason", "Fix",
	)
	crit1 := deliberation.ProposeCritique(
		cand1.ID,
		deliberation.PersonaCleanCodeReviewer,
		deliberation.VerdictAgree, "Agreed", 0.0,
	)

	cands := []deliberation.CandidateFinding{cand1}
	crits := []deliberation.PeerCritique{crit1}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		findings, decisions := deliberator.DeliberateExec(ctx, cands, crits)
		if len(findings) != 1 || len(decisions) != 1 {
			b.Fatal("deliberation failed")
		}
	}
}
