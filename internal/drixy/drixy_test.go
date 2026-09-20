// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package drixy_test

import (
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/drixy"
)

func TestDrixyConstants(t *testing.T) {
	if drixy.Name != "Drixy" {
		t.Errorf("expected Name to be 'Drixy', got %s", drixy.Name)
	}
	if drixy.BotName != "drixy[bot]" {
		t.Errorf("expected BotName to be 'drixy[bot]', got %s", drixy.BotName)
	}
	if drixy.BotMention != "@drixy" {
		t.Errorf("expected BotMention to be '@drixy', got %s", drixy.BotMention)
	}
}

func TestPRCommentFooter(t *testing.T) {
	footer := drixy.PRCommentFooter()
	if !strings.Contains(footer, "Reviewed by **Drixy**") {
		t.Errorf("expected footer to mention Drixy, got: %s", footer)
	}
	if !strings.Contains(footer, "@drixy") {
		t.Errorf("expected footer to include @drixy mention, got: %s", footer)
	}
}

func TestFormatDrixyGreeting(t *testing.T) {
	greeting := drixy.FormatDrixyGreeting("octocat")
	if !strings.Contains(greeting, "@octocat") {
		t.Errorf("expected greeting to address author, got: %s", greeting)
	}
	if !strings.Contains(greeting, "Drixy") {
		t.Errorf("expected greeting to mention Drixy, got: %s", greeting)
	}
}

func TestFormatDrixyBanner(t *testing.T) {
	banner := drixy.FormatDrixyBanner()
	if !strings.Contains(banner, "DRIXY") {
		t.Errorf("expected banner to contain DRIXY, got: %s", banner)
	}
}
