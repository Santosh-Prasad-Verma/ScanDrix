// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package webhooks

import (
	"testing"

	"github.com/scandrix/backend/pkg/models"
)

func TestWebhooksMappers(t *testing.T) {
	// Test GitHub Mapped Platform
	gh := GetMappedPlatform(models.SCMProviderGitHub)
	if gh == nil {
		t.Fatalf("expected GitHub mapped platform")
	}

	ghPayload := map[string]any{
		"action": "opened",
		"repository": map[string]any{
			"id":             123,
			"name":           "repo1",
			"full_name":      "owner/repo1",
			"html_url":       "https://github.com/owner/repo1",
			"default_branch": "main",
		},
		"pull_request": map[string]any{
			"number":   9,
			"title":    "PR 9",
			"body":     "Description 9",
			"html_url": "https://github.com/owner/repo1/pull/9",
			"head": map[string]any{
				"ref": "feature",
				"sha": "sha123",
				"repo": map[string]any{
					"full_name": "owner/repo1",
				},
			},
			"base": map[string]any{
				"ref": "main",
				"repo": map[string]any{
					"full_name": "owner/repo1",
				},
			},
		},
		"comment": map[string]any{
			"id":   555,
			"body": "Comment text",
		},
	}

	pr := gh.MapPullRequest(ghPayload)
	if pr == nil || pr.Number != 9 || pr.Head.SHA != "sha123" || pr.Base.Repo.DefaultBranch != "main" {
		t.Errorf("unexpected mapped GitHub PR: %+v", pr)
	}

	repo := gh.MapRepository(ghPayload)
	if repo == nil || repo.ID != "123" || repo.FullName != "owner/repo1" {
		t.Errorf("unexpected mapped GitHub repo: %+v", repo)
	}

	comment := gh.MapComment(ghPayload)
	if comment == nil || comment.ID != "555" || comment.Body != "Comment text" {
		t.Errorf("unexpected mapped GitHub comment: %+v", comment)
	}

	// Test GitLab Mapped Platform
	gl := GetMappedPlatform(models.SCMProviderGitLab)
	if gl == nil {
		t.Fatalf("expected GitLab mapped platform")
	}

	glPayload := map[string]any{
		"project": map[string]any{
			"id":                  999,
			"name":                "gl-repo",
			"path_with_namespace": "group/gl-repo",
			"web_url":             "https://gitlab.com/group/gl-repo",
		},
		"object_attributes": map[string]any{
			"iid":           12,
			"title":         "Draft: GL PR",
			"description":   "GL body",
			"source_branch": "feature-gl",
			"target_branch": "main",
			"last_commit": map[string]any{
				"id": "commit-gl-sha",
			},
			"source": map[string]any{
				"path_with_namespace": "group/gl-repo",
			},
			"target": map[string]any{
				"path_with_namespace": "group/gl-repo",
				"default_branch":      "main",
			},
		},
	}

	glPR := gl.MapPullRequest(glPayload)
	if glPR == nil || glPR.Number != 12 || !glPR.IsDraft || glPR.Head.SHA != "commit-gl-sha" {
		t.Errorf("unexpected mapped GitLab MR: %+v", glPR)
	}

	// Test Bitbucket UUID stripping
	uuidWithBraces := "{12345678-abcd-ef01-2345-6789abcdef01}"
	cleaned := StripCurlyBracesFromUUID(uuidWithBraces)
	if cleaned != "12345678-abcd-ef01-2345-6789abcdef01" {
		t.Errorf("expected stripped braces, got %q", cleaned)
	}
}
