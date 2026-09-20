// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package domain_test

import (
	"testing"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agentharness/domain"
	"github.com/stretchr/testify/assert"
)

func TestFinalText(t *testing.T) {
	t.Run("nil state returns empty", func(t *testing.T) {
		assert.Equal(t, "", domain.FinalText(nil))
	})

	t.Run("scans in reverse to find last non-empty assistant text", func(t *testing.T) {
		state := &contracts.RunState{
			Steps: []contracts.RunStep{
				{
					Index: 0,
					Message: contracts.AgentMessage{
						Role:    contracts.RoleAssistant,
						Content: "First thought",
					},
				},
				{
					Index: 1,
					Message: contracts.AgentMessage{
						Role:    contracts.RoleTool,
						Content: "tool result",
					},
				},
				{
					Index: 2,
					Message: contracts.AgentMessage{
						Role:    contracts.RoleAssistant,
						Content: "Final review conclusion.",
					},
				},
			},
		}

		assert.Equal(t, "Final review conclusion.", domain.FinalText(state))
	})

	t.Run("ignores empty assistant turns and non-assistant roles", func(t *testing.T) {
		state := &contracts.RunState{
			Steps: []contracts.RunStep{
				{
					Index: 0,
					Message: contracts.AgentMessage{
						Role:    contracts.RoleAssistant,
						Content: "Real answer",
					},
				},
				{
					Index: 1,
					Message: contracts.AgentMessage{
						Role:    contracts.RoleAssistant,
						Content: "   \n  ",
					},
				},
				{
					Index: 2,
					Message: contracts.AgentMessage{
						Role:    contracts.RoleUser,
						Content: "User message",
					},
				},
			},
		}

		assert.Equal(t, "Real answer", domain.FinalText(state))
	})
}

func TestArtifactHelpers(t *testing.T) {
	state := &contracts.RunState{
		Artifacts: []contracts.Artifact{
			{Type: "finding", Payload: "finding-1"},
			{Type: "metric", Payload: 42},
			{Type: "finding", Payload: "finding-2"},
		},
	}

	findings := domain.ArtifactsByType(state, "finding")
	assert.Len(t, findings, 2)

	last, ok := domain.LastArtifact(state, "finding")
	assert.True(t, ok)
	assert.Equal(t, "finding-2", last.Payload)

	_, ok = domain.LastArtifact(state, "unknown")
	assert.False(t, ok)
}
