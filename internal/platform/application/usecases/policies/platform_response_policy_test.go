// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package policies

import (
	"strings"
	"testing"

	"github.com/scandrix/backend/pkg/models"
)

func TestPlatformResponsePolicyFactory(t *testing.T) {
	factory := &PlatformResponsePolicyFactory{}

	tests := []struct {
		name                   string
		provider               models.SCMProvider
		expectedAcknowledgment bool
		expectedReaction       bool
		expectedReactionName   string
		expectedBodyContains   string
	}{
		{
			name:                   "GitHub policy uses reactions and no acknowledgment body",
			provider:               models.ProviderGitHub,
			expectedAcknowledgment: false,
			expectedReaction:       true,
			expectedReactionName:   "rocket",
		},
		{
			name:                   "GitLab policy uses reactions and no acknowledgment body",
			provider:               models.ProviderGitLab,
			expectedAcknowledgment: false,
			expectedReaction:       true,
			expectedReactionName:   "rocket",
		},
		{
			name:                   "Bitbucket policy uses acknowledgment comment and no reaction",
			provider:               models.ProviderBitbucket,
			expectedAcknowledgment: true,
			expectedReaction:       false,
			expectedBodyContains:   "Analyzing your request...",
		},
		{
			name:                   "Azure DevOps policy uses acknowledgment comment with drixy marker",
			provider:               models.ProviderAzure,
			expectedAcknowledgment: true,
			expectedReaction:       false,
			expectedBodyContains:   "<!-- drixy-codereview -->",
		},
		{
			name:                   "Forgejo policy uses reactions and no acknowledgment body",
			provider:               models.ProviderForgejo,
			expectedAcknowledgment: false,
			expectedReaction:       true,
			expectedReactionName:   "rocket",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := factory.Create(tt.provider)

			if policy.RequiresAcknowledgment() != tt.expectedAcknowledgment {
				t.Errorf("RequiresAcknowledgment(): expected %v, got %v", tt.expectedAcknowledgment, policy.RequiresAcknowledgment())
			}

			if policy.UsesReaction() != tt.expectedReaction {
				t.Errorf("UsesReaction(): expected %v, got %v", tt.expectedReaction, policy.UsesReaction())
			}

			if tt.expectedReaction {
				reaction, err := policy.GetAcknowledgmentReaction()
				if err != nil {
					t.Fatalf("unexpected error getting reaction: %v", err)
				}
				if reaction != tt.expectedReactionName {
					t.Errorf("expected reaction %s, got %s", tt.expectedReactionName, reaction)
				}

				if _, err := policy.GetAcknowledgmentBody(); err == nil {
					t.Errorf("expected error getting acknowledgment body when reactions used")
				}
			} else {
				body, err := policy.GetAcknowledgmentBody()
				if err != nil {
					t.Fatalf("unexpected error getting body: %v", err)
				}
				if !strings.Contains(body, tt.expectedBodyContains) {
					t.Errorf("expected body to contain %q, got %q", tt.expectedBodyContains, body)
				}

				if _, err := policy.GetAcknowledgmentReaction(); err == nil {
					t.Errorf("expected error getting reaction when acknowledgment body used")
				}
			}
		})
	}
}
