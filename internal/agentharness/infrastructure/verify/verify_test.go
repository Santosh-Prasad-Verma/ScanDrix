// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package verify_test

import (
	"testing"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/tools"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/verify"
	"github.com/stretchr/testify/assert"
)

func TestBuildVerifierAgentSpec(t *testing.T) {
	spec := verify.BuildVerifierAgentSpec(verify.BuildVerifierAgentSpecParams{
		SystemPrompt: "Verify this finding strictly.",
		Tools:        tools.NewInMemoryToolRegistry(),
		MaxSteps:     5,
	})

	assert.Equal(t, "verifier", spec.ID)
	assert.Equal(t, verify.VerifyDoneTool, spec.ResultToolName)
	assert.Equal(t, 5, spec.MaxSteps)

	// Ensure submitVerdict tool was automatically appended
	_, ok := spec.Tools.Get(verify.VerifyDoneTool)
	assert.True(t, ok)
}

func TestExtractVerdict(t *testing.T) {
	t.Run("extracts keep: false when explicitly refuted", func(t *testing.T) {
		state := &contracts.RunState{
			Artifacts: []contracts.Artifact{
				{
					Type: verify.VerifyDoneTool,
					Payload: map[string]any{
						"keep":       false,
						"rationale":  "Finding refuted: null check exists at line 10.",
						"confidence": "high",
					},
				},
			},
		}

		v := verify.ExtractVerdict(state)
		assert.False(t, v.Keep)
		assert.Equal(t, "high", v.Confidence)
		assert.Contains(t, v.Rationale, "Finding refuted")
	})

	t.Run("fail-open default: keep=true when no parseable verdict", func(t *testing.T) {
		state := &contracts.RunState{
			Artifacts: []contracts.Artifact{},
		}

		v := verify.ExtractVerdict(state)
		assert.True(t, v.Keep, "must fail-open on absent or unparseable verdict")
		assert.Contains(t, v.Rationale, "kept by default")
	})
}
