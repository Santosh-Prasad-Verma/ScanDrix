// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package comments

import "testing"

func TestCommentMarkersAndDirectives(t *testing.T) {
	// Review commands
	if !IsReviewCommand("@drixy review", "") {
		t.Errorf("expected '@drixy review' to be review command")
	}
	if !IsReviewCommand("@drixy start-review", "") {
		t.Errorf("expected '@drixy start-review' to be review command")
	}
	if !IsReviewCommand("  @drixy review --force", "") {
		t.Errorf("expected '@drixy review --force' to be review command")
	}
	if !IsReviewCommand("@custombot review", "custombot") {
		t.Errorf("expected custom bot username review command to match")
	}

	// Force commands
	if !IsForceReviewCommand("@drixy review --force", "") {
		t.Errorf("expected force flag to be recognized")
	}
	if !IsForceReviewCommand("@drixy review -force", "") {
		t.Errorf("expected single dash force flag to be recognized")
	}
	if IsForceReviewCommand("@drixy review", "") {
		t.Errorf("expected regular review command not to be force")
	}

	// Heavy review commands
	if !IsHeavyReviewCommand("@drixy review --heavy", "") {
		t.Errorf("expected heavy flag to be recognized")
	}
	if !IsHeavyReviewCommand("@drixy review auth path --heavy", "") {
		t.Errorf("expected heavy flag anywhere in command to be recognized")
	}

	// Mention non-review (Git chat)
	if !IsDrixyMentionNonReview("@drixy what does this PR do?", "") {
		t.Errorf("expected non-review mention to be recognized")
	}
	if IsDrixyMentionNonReview("@drixy review", "") {
		t.Errorf("review command must not be non-review mention")
	}

	// Review markers
	if !HasReviewMarker("Here is a review <!-- drixy-codereview --> done") {
		t.Errorf("expected review marker to be found")
	}
	if HasReviewMarker("Normal comment without marker") {
		t.Errorf("expected false when marker is absent")
	}

	// Parse directives
	directive := ParseReviewDirective("@drixy review focus on database migrations", "")
	if directive != "focus on database migrations" {
		t.Errorf("unexpected directive parsed: %q", directive)
	}

	directiveWithFlags := ParseReviewDirective("@drixy review --force --heavy focus on auth", "")
	if directiveWithFlags != "focus on auth" {
		t.Errorf("unexpected directive parsed with flags: %q", directiveWithFlags)
	}

	directiveQuotes := ParseReviewDirective("@drixy review \"focus on speed\"", "")
	if directiveQuotes != "focus on speed" {
		t.Errorf("unexpected quotes in directive: %q", directiveQuotes)
	}

	// Sanitization prevents HTML breakout
	sanitized := SanitizeReviewDirective("<script>alert('pwn')</script>")
	if sanitized != "script alert('pwn') /script" {
		t.Errorf("unexpected sanitization result: %q", sanitized)
	}
}
