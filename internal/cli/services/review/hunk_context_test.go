// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package review

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

func TestHunkContextAndSidecar(t *testing.T) {
	res := &ReviewResult{
		Summary:       "Analysis found critical injection issues",
		FilesAnalyzed: 3,
		Findings: []models.CodeFinding{
			{
				ID:          uuid.New(),
				FilePath:    "pkg/db/query.go",
				StartLine:   10,
				EndLine:     15,
				Severity:    models.FindingSeverity("CRITICAL"),
				Title:       "SQL Injection",
				Description: "Unsanitized query input",
				Remediation: "Use query parameters",
			},
			{
				ID:          uuid.New(),
				FilePath:    "pkg/auth/jwt.go",
				StartLine:   50,
				EndLine:     52,
				Severity:    models.FindingSeverity("HIGH"),
				Title:       "Weak Signature Algorithm",
				Description: "HS256 with weak secret",
			},
		},
	}

	// 1. Convert to HunkAgentContext
	hunkCtx := ConvertReviewToHunkContext(res)
	if hunkCtx.Version != 1 {
		t.Errorf("expected version 1, got %d", hunkCtx.Version)
	}
	if len(hunkCtx.Files) != 2 {
		t.Fatalf("expected 2 files in hunk context, got %d", len(hunkCtx.Files))
	}
	if CountHunkAnnotations(hunkCtx) != 2 {
		t.Errorf("expected 2 total annotations, got %d", CountHunkAnnotations(hunkCtx))
	}

	// 2. Convert to ScanDrixHunkFindings
	sidecar := ConvertReviewToHunkFindings(res)
	if sidecar.Version != 1 {
		t.Errorf("expected sidecar version 1, got %d", sidecar.Version)
	}
	if len(sidecar.Findings) != 2 {
		t.Fatalf("expected 2 sidecar findings, got %d", len(sidecar.Findings))
	}
	if sidecar.Findings[0].ID != "scandrix-0" || sidecar.Findings[1].ID != "scandrix-1" {
		t.Errorf("unexpected sidecar finding IDs: %s, %s", sidecar.Findings[0].ID, sidecar.Findings[1].ID)
	}

	// 3. Export to temp files
	ctxPath, findingsPath, cleanup, err := ExportHunkSidecarFiles(res)
	if err != nil {
		t.Fatalf("ExportHunkSidecarFiles failed: %v", err)
	}
	defer cleanup()

	if _, err := os.Stat(ctxPath); err != nil {
		t.Errorf("expected context file to exist at %s", ctxPath)
	}
	if _, err := os.Stat(findingsPath); err != nil {
		t.Errorf("expected findings file to exist at %s", findingsPath)
	}

	cleanup()
	if _, err := os.Stat(ctxPath); !os.IsNotExist(err) {
		t.Errorf("expected context file to be removed after cleanup")
	}
	if _, err := os.Stat(findingsPath); !os.IsNotExist(err) {
		t.Errorf("expected findings file to be removed after cleanup")
	}
}
