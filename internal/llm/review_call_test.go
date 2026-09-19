// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package llm_test

import (
	"context"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/llm/byok"
	_ "github.com/scandrix/backend/internal/llm/providers/all"
)

type sampleFindingsTarget struct {
	Summary  string `json:"summary"`
	Severity string `json:"severity"`
}

func TestStructuredReviewCall_NilTargetError(t *testing.T) {
	_, err := llm.RunStructuredReviewCall(context.Background(), llm.StructuredReviewCallParams{
		BaseReviewCallParams: llm.BaseReviewCallParams{
			User: "review this code",
		},
		Target: nil,
	})
	if err == nil {
		t.Fatal("expected error when target is nil")
	}
}

func TestReviewCall_TimeoutPropagation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel context

	var target sampleFindingsTarget
	_, err := llm.RunStructuredReviewCall(ctx, llm.StructuredReviewCallParams{
		BaseReviewCallParams: llm.BaseReviewCallParams{
			Slot: &byok.NormalizedModel{
				Provider: byok.ProviderOpenAI,
				Model:    "gpt-4o",
			},
			User:    "review this code",
			Timeout: 10 * time.Millisecond,
		},
		Target: &target,
	})
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
}
