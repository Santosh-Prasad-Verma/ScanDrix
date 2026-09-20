// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"bytes"
	"strings"
	"testing"
)

func TestBannerUtils_TruncateLine(t *testing.T) {
	if TruncateLine("short", 10) != "short" {
		t.Fatalf("expected 'short', got %q", TruncateLine("short", 10))
	}
	if TruncateLine("very long line here", 10) != "very lo..." {
		t.Fatalf("expected 'very lo...', got %q", TruncateLine("very long line here", 10))
	}
	if TruncateLine("hello", 3) != "hel" {
		t.Fatalf("expected 'hel', got %q", TruncateLine("hello", 3))
	}
	if TruncateLine("hello", 1) != "h" {
		t.Fatalf("expected 'h', got %q", TruncateLine("hello", 1))
	}
}

func TestBannerUtils_MakeTwoColumnRows(t *testing.T) {
	left := []string{"Left 1", "Left 2"}
	right := []string{"Right 1", "Right 2", "Right 3"}

	rows := MakeTwoColumnRows(left, right, 60, 0.5)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	for _, r := range rows {
		if !strings.Contains(r, "  |  ") {
			t.Fatalf("row missing column separator: %s", r)
		}
	}
}

func TestBannerUtils_RenderBanner(t *testing.T) {
	buf := &bytes.Buffer{}
	RenderBanner(buf, "1.2.0", "Team API Key", []string{"Reviewed PR #42", "Applied 3 security fixes"})

	output := buf.String()
	if !strings.Contains(output, "ScanDrix AI Code Review Cockpit v1.2.0") {
		t.Fatalf("banner missing cockpit title: %s", output)
	}
	if !strings.Contains(output, "Auth Mode:") || !strings.Contains(output, "Team API Key") {
		t.Fatalf("banner missing auth mode: %s", output)
	}
	if !strings.Contains(output, "Quick Start") || !strings.Contains(output, "Common Commands") {
		t.Fatalf("banner missing commands sections: %s", output)
	}
	if !strings.Contains(output, "Reviewed PR #42") {
		t.Fatalf("banner missing recent activity: %s", output)
	}
}
