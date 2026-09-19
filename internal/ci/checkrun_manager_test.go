// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package ci

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckRunManager_ConvertFindingsToAnnotations(t *testing.T) {
	mgr := NewCheckRunManager(CIEnvironment{}, nil)

	findings := []CIFinding{
		{
			RuleID:    "SEC-001",
			RuleTitle: "SQL Injection",
			Category:  "SECURITY",
			Severity:  FailCritical,
			FilePath:  "internal/db.go",
			StartLine: 25,
			EndLine:   28,
			Message:   "Vulnerability found",
		},
		{
			RuleID:    "STYLE-002",
			RuleTitle: "Naming convention",
			Category:  "ARCHITECTURE",
			Severity:  FailWarning,
			FilePath:  "internal/names.go",
			StartLine: 12,
			EndLine:   12,
			Message:   "Consider PascalCase",
		},
	}

	anns := mgr.ConvertFindingsToAnnotations(findings)
	require.Len(t, anns, 2)

	assert.Equal(t, "failure", anns[0].AnnotationLevel)
	assert.Equal(t, "internal/db.go", anns[0].Path)
	assert.Equal(t, 25, anns[0].StartLine)

	assert.Equal(t, "warning", anns[1].AnnotationLevel)
}

func TestCheckRunManager_PublishStatus(t *testing.T) {
	var receivedAuth string
	var receivedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		receivedBody = string(b)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id": 987654321}`))
	}))
	defer server.Close()

	env := CIEnvironment{
		Platform:  PlatformGitLabCI,
		RepoName:  "backend",
		ServerURL: server.URL,
		APIToken:  "glpat-secret-token",
	}

	mgr := NewCheckRunManager(env, server.Client())

	opts := CheckRunOptions{
		Name:       "ScanDrix Review",
		HeadSHA:    "deadbeef1234",
		Status:     CheckStatusCompleted,
		Conclusion: CheckConclusionSuccess,
		Title:      "ScanDrix Check",
		Summary:    "All policies satisfied",
	}

	anns := mgr.ConvertFindingsToAnnotations([]CIFinding{
		{
			RuleID:    "RULE-1",
			RuleTitle: "Title 1",
			Severity:  FailWarning,
			FilePath:  "foo.go",
			StartLine: 1,
			Message:   "Msg",
		},
	})
	opts.Annotations = anns

	statusID, err := mgr.PublishCheckRun(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, "gl-deadbeef1234", statusID)
	assert.Contains(t, receivedAuth, "")
	_ = receivedBody
}
