// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package review

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// HUNK AGENT CONTEXT SCHEMA (Inline Diff Notes)

// HunkAgentContext models the standard agent-context JSON payload consumed by hunk.
type HunkAgentContext struct {
	Version int                    `json:"version"` // 1
	Summary string                 `json:"summary,omitempty"`
	Files   []HunkAgentContextFile `json:"files"`
}

// HunkAgentContextFile holds file-level inline annotations.
type HunkAgentContextFile struct {
	Path        string                `json:"path"`
	Summary     string                `json:"summary,omitempty"`
	Annotations []HunkAgentAnnotation `json:"annotations"`
}

// HunkAgentAnnotation models a single inline finding anchored to new diff lines.
type HunkAgentAnnotation struct {
	NewRange  [2]int `json:"newRange"` // [startLine, endLine]
	Summary   string `json:"summary"`
	Rationale string `json:"rationale,omitempty"`
	Markup    string `json:"markup,omitempty"` // Experimental STML
}

// SCANDRIX HUNK FINDINGS SIDECAR SCHEMA (Sidebar Extension)

// ScanDrixHunkFindings is the structured sidecar consumed by sidebar review extensions.
type ScanDrixHunkFindings struct {
	Version  int                   `json:"version"` // 1
	Summary  string                `json:"summary,omitempty"`
	Findings []ScanDrixHunkFinding `json:"findings"`
}

// ScanDrixHunkFinding is an individual structured finding record in the sidecar.
type ScanDrixHunkFinding struct {
	ID       string `json:"id"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	EndLine  int    `json:"endLine"`
	Severity string `json:"severity"` // critical, high, medium, low, info
	Title    string `json:"title"`
	Category string `json:"category,omitempty"`
	RuleID   string `json:"ruleId,omitempty"`
}

var severityGlyphs = map[string]string{
	"CRITICAL": "‼",
	"HIGH":     "✖",
	"MEDIUM":   "⚠",
	"LOW":      "ℹ",
	"INFO":     "ℹ",
}

// ConvertReviewToHunkContext converts a ReviewResult into HunkAgentContext.
func ConvertReviewToHunkContext(result *ReviewResult) *HunkAgentContext {
	filesMap := make(map[string]*HunkAgentContextFile)

	for _, f := range result.Findings {
		if f.FilePath == "" {
			continue
		}

		startLine := f.StartLine
		if startLine <= 0 {
			startLine = 1
		}
		endLine := f.EndLine
		if endLine < startLine {
			endLine = startLine
		}

		sevUpper := strings.ToUpper(string(f.Severity))
		glyph := severityGlyphs[sevUpper]
		if glyph == "" {
			glyph = "•"
		}

		headline := fmt.Sprintf("%s [%s] %s", glyph, sevUpper, f.Title)
		if len(headline) > 140 {
			headline = headline[:137] + "..."
		}

		annotation := HunkAgentAnnotation{
			NewRange:  [2]int{startLine, endLine},
			Summary:   headline,
			Rationale: f.Description,
		}
		if f.Remediation != "" {
			annotation.Rationale = fmt.Sprintf("%s\n\nRemediation:\n%s", f.Description, f.Remediation)
		}

		bucket, exists := filesMap[f.FilePath]
		if !exists {
			bucket = &HunkAgentContextFile{
				Path:        f.FilePath,
				Annotations: make([]HunkAgentAnnotation, 0),
			}
			filesMap[f.FilePath] = bucket
		}
		bucket.Annotations = append(bucket.Annotations, annotation)
	}

	files := make([]HunkAgentContextFile, 0, len(filesMap))
	for _, f := range filesMap {
		count := len(f.Annotations)
		plural := "findings"
		if count == 1 {
			plural = "finding"
		}
		f.Summary = fmt.Sprintf("%d %s", count, plural)

		sort.Slice(f.Annotations, func(i, j int) bool {
			if f.Annotations[i].NewRange[0] != f.Annotations[j].NewRange[0] {
				return f.Annotations[i].NewRange[0] < f.Annotations[j].NewRange[0]
			}
			return f.Annotations[i].NewRange[1] < f.Annotations[j].NewRange[1]
		})
		files = append(files, *f)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	summary := result.Summary
	if summary == "" {
		summary = fmt.Sprintf("ScanDrix review completed with %d findings across %d files.", len(result.Findings), result.FilesAnalyzed)
	}

	return &HunkAgentContext{
		Version: 1,
		Summary: summary,
		Files:   files,
	}
}

// ConvertReviewToHunkFindings converts a ReviewResult into ScanDrixHunkFindings.
func ConvertReviewToHunkFindings(result *ReviewResult) *ScanDrixHunkFindings {
	findings := make([]ScanDrixHunkFinding, 0, len(result.Findings))

	for i, f := range result.Findings {
		if f.FilePath == "" {
			continue
		}
		line := f.StartLine
		if line <= 0 {
			line = 1
		}
		endLine := f.EndLine
		if endLine < line {
			endLine = line
		}

		title := f.Title
		if title == "" {
			title = f.Description
		}
		if title == "" {
			title = "ScanDrix finding"
		}
		if len(title) > 200 {
			title = title[:197] + "..."
		}

		findings = append(findings, ScanDrixHunkFinding{
			ID:       fmt.Sprintf("scandrix-%d", i),
			File:     f.FilePath,
			Line:     line,
			EndLine:  endLine,
			Severity: strings.ToLower(string(f.Severity)),
			Title:    title,
			Category: f.Category,
		})
	}

	return &ScanDrixHunkFindings{
		Version:  1,
		Summary:  result.Summary,
		Findings: findings,
	}
}

// CountHunkAnnotations totals all annotations in a HunkAgentContext.
func CountHunkAnnotations(context *HunkAgentContext) int {
	if context == nil {
		return 0
	}
	total := 0
	for _, f := range context.Files {
		total += len(f.Annotations)
	}
	return total
}

// ExportHunkSidecarFiles serializes the context and sidecar findings into temporary files.
func ExportHunkSidecarFiles(result *ReviewResult) (contextPath, findingsPath string, cleanup func(), err error) {
	runID := uuid.New().String()[:8]
	tmpDir := os.TempDir()

	contextPath = filepath.Join(tmpDir, fmt.Sprintf("scandrix-review-%s.json", runID))
	findingsPath = filepath.Join(tmpDir, fmt.Sprintf("scandrix-findings-%s.json", runID))

	ctxObj := ConvertReviewToHunkContext(result)
	ctxData, err := json.MarshalIndent(ctxObj, "", "  ")
	if err != nil {
		return "", "", nil, err
	}
	if err := os.WriteFile(contextPath, ctxData, 0600); err != nil {
		return "", "", nil, err
	}

	findObj := ConvertReviewToHunkFindings(result)
	findData, err := json.MarshalIndent(findObj, "", "  ")
	if err != nil {
		_ = os.Remove(contextPath)
		return "", "", nil, err
	}
	if err := os.WriteFile(findingsPath, findData, 0600); err != nil {
		_ = os.Remove(contextPath)
		return "", "", nil, err
	}

	cleanup = func() {
		_ = os.Remove(contextPath)
		_ = os.Remove(findingsPath)
	}

	return contextPath, findingsPath, cleanup, nil
}
