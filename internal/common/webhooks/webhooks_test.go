package webhooks

import (
	"testing"
)

func TestDraftTitleRecognition(t *testing.T) {
	if !IsDraftTitle("Draft: Feature addition") {
		t.Fatalf("expected 'Draft: Feature addition' to be recognized as draft")
	}

	if !IsDraftTitle("[DRAFT] Add security audit") {
		t.Fatalf("expected '[DRAFT] Add security audit' to be recognized as draft")
	}

	if !IsDraftTitle("WIP: Work in progress changes") {
		t.Fatalf("expected 'WIP:' to be recognized as draft")
	}

	if IsDraftTitle("Feature: Normal production ready PR") {
		t.Fatalf("expected normal PR not to be draft")
	}

	stripped := StripDraftPrefix("[Draft] Implement notifications")
	if stripped != "Implement notifications" {
		t.Fatalf("expected 'Implement notifications', got %q", stripped)
	}
}

func TestGithubMapper(t *testing.T) {
	mapper := &GithubMapper{}
	payload := map[string]any{
		"action": "opened",
		"pull_request": map[string]any{
			"id":     12345.0,
			"number": 42.0,
			"title":  "Add high throughput webhook processor",
			"body":   "Resolves issue #99",
			"draft":  false,
			"head": map[string]any{
				"ref": "feature/webhooks",
				"sha": "abc1234",
			},
			"base": map[string]any{
				"ref": "main",
				"sha": "def5678",
			},
			"user": map[string]any{
				"id":         101.0,
				"login":      "octocat",
				"avatar_url": "https://avatar.com/octocat",
			},
		},
		"repository": map[string]any{
			"id":        9876.0,
			"name":      "backend",
			"full_name": "scandrix/backend",
			"html_url":  "https://github.com/scandrix/backend",
		},
	}

	pr := mapper.MapPullRequest(payload)
	if pr == nil {
		t.Fatalf("expected non-nil MappedPullRequest")
	}

	if pr.Number != 42 || pr.Title != "Add high throughput webhook processor" {
		t.Fatalf("unexpected PR mapping values: number=%d title=%s", pr.Number, pr.Title)
	}

	if pr.Head.Ref != "feature/webhooks" || pr.Head.SHA != "abc1234" {
		t.Fatalf("unexpected Head commit: %v", pr.Head)
	}

	if pr.Repository == nil || pr.Repository.FullName != "scandrix/backend" {
		t.Fatalf("unexpected repository mapping")
	}

	if mapper.MapAction("opened") != ActionOpened {
		t.Fatalf("expected ActionOpened")
	}
}

func TestBitbucketMapper(t *testing.T) {
	mapper := &BitbucketMapper{}
	payload := map[string]any{
		"pullrequest": map[string]any{
			"id":          105.0,
			"title":       "Improve pipeline throughput",
			"description": "Closes #12",
			"state":       "OPEN",
			"draft":       false,
			"links": map[string]any{
				"html": map[string]any{
					"href": "https://bitbucket.org/scandrix/backend/pull-requests/105",
				},
			},
			"author": map[string]any{
				"uuid":         "{user-uuid-123}",
				"nickname":     "bbuser",
				"display_name": "Bitbucket User",
			},
			"source": map[string]any{
				"branch": map[string]any{"name": "feature/perf"},
				"commit": map[string]any{"hash": "abc999"},
				"repository": map[string]any{
					"full_name": "scandrix/backend",
				},
			},
			"destination": map[string]any{
				"branch": map[string]any{"name": "main"},
				"commit": map[string]any{"hash": "def000"},
				"repository": map[string]any{
					"full_name": "scandrix/backend",
				},
			},
		},
		"repository": map[string]any{
			"uuid":      "{repo-uuid-456}",
			"name":      "backend",
			"full_name": "scandrix/backend",
			"is_private": true,
		},
	}

	pr := mapper.MapPullRequest(payload)
	if pr == nil {
		t.Fatalf("expected non-nil MappedPullRequest")
	}
	if pr.Number != 105 || pr.Title != "Improve pipeline throughput" {
		t.Errorf("unexpected PR: number=%d, title=%s", pr.Number, pr.Title)
	}
	if pr.Head.Ref != "feature/perf" || pr.Head.SHA != "abc999" {
		t.Errorf("unexpected Head: %v", pr.Head)
	}
	if pr.Base.Ref != "main" {
		t.Errorf("unexpected Base ref: %s", pr.Base.Ref)
	}
	if pr.User == nil || pr.User.Login != "bbuser" {
		t.Errorf("unexpected User: %v", pr.User)
	}

	// Action tests
	if mapper.MapAction("pullrequest:created") != ActionOpened {
		t.Errorf("expected ActionOpened")
	}
	if mapper.MapAction("pullrequest:fulfilled") != ActionMerged {
		t.Errorf("expected ActionMerged")
	}
	if mapper.MapAction("pullrequest:rejected") != ActionClosed {
		t.Errorf("expected ActionClosed")
	}
}

func TestForgejoMapper(t *testing.T) {
	mapper := &ForgejoMapper{}
	payload := map[string]any{
		"action": "opened",
		"pull_request": map[string]any{
			"id":     501.0,
			"number": 12.0,
			"title":  "Refactor git webhook parsing",
			"body":   "Full parity with forgejo",
			"state":  "open",
			"draft":  false,
			"html_url": "https://codeberg.org/scandrix/backend/pulls/12",
			"user": map[string]any{
				"id":        200.0,
				"login":     "forgeuser",
				"full_name": "Forgejo User",
			},
			"head": map[string]any{
				"ref": "refactor/webhooks",
				"sha": "forge-sha-1",
				"repo": map[string]any{
					"full_name": "scandrix/backend",
				},
			},
			"base": map[string]any{
				"ref": "main",
				"sha": "forge-sha-base",
				"repo": map[string]any{
					"full_name": "scandrix/backend",
				},
			},
			"labels": []any{
				map[string]any{"name": "enhancement"},
			},
		},
		"repository": map[string]any{
			"id":             300.0,
			"name":           "backend",
			"full_name":      "scandrix/backend",
			"default_branch": "main",
			"private":        true,
		},
	}

	pr := mapper.MapPullRequest(payload)
	if pr == nil {
		t.Fatalf("expected non-nil MappedPullRequest")
	}
	if pr.Number != 12 || pr.Title != "Refactor git webhook parsing" {
		t.Errorf("unexpected PR number or title: %d, %s", pr.Number, pr.Title)
	}
	if len(pr.Labels) != 1 || pr.Labels[0] != "enhancement" {
		t.Errorf("unexpected labels: %v", pr.Labels)
	}
	if mapper.MapAction("opened") != ActionOpened {
		t.Errorf("expected ActionOpened")
	}
	if mapper.MapAction("synchronized") != ActionUpdated {
		t.Errorf("expected ActionUpdated")
	}
}

func TestWebhookTokenCrypto(t *testing.T) {
	// Valid 32-byte key in hex (64 hex characters)
	secretHex := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	token := "webhook-token-secret-xyz-987"

	encrypted, err := GenerateWebhookToken(secretHex, token)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	// Valid token should validate
	if !ValidateWebhookToken(secretHex, token, encrypted) {
		t.Fatalf("expected token to validate successfully")
	}

	// Wrong token should fail
	if ValidateWebhookToken(secretHex, "wrong-token", encrypted) {
		t.Fatalf("expected wrong token to fail validation")
	}

	// Wrong secret should fail
	wrongSecretHex := "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
	if ValidateWebhookToken(wrongSecretHex, token, encrypted) {
		t.Fatalf("expected wrong secret to fail validation")
	}

	// Corrupted encrypted token should fail
	if ValidateWebhookToken(secretHex, token, "corrupted:data") {
		t.Fatalf("expected corrupted token to fail validation")
	}

	// Invalid hex key length in generator
	_, err = GenerateWebhookToken("shortkey", token)
	if err == nil {
		t.Fatalf("expected error for short secret key")
	}
}
