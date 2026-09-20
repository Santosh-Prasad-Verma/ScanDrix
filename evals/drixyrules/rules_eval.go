// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Evaluation Suite
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package drixyrules

import (
	"encoding/json"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
)

const (
	DefaultLineTolerance = 2
)

// Site identifies an exact file and line where a rule violation exists or was flagged.
type Site struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

// CaseScore evaluates rule violation detection against ground-truth sites.
type CaseScore struct {
	TotalGroundTruth int     `json:"total_ground_truth"`
	TotalFlags       int     `json:"total_flags"`
	Caught           int     `json:"caught"`
	OnTarget         int     `json:"on_target"`
	Recall           float64 `json:"recall"`
	Precision        float64 `json:"precision"`
	F1Score          float64 `json:"f1_score"`
}

// ScoreCase computes occurrence-recall and line-precision for a set of flagged violations
// against ground-truth sites within a configurable line tolerance (default ±2 lines).
func ScoreCase(sites []Site, flags []Site, customTolerance ...int) CaseScore {
	lineTol := DefaultLineTolerance
	if len(customTolerance) > 0 && customTolerance[0] >= 0 {
		lineTol = customTolerance[0]
	} else if val := os.Getenv("SCANDRIX_EVAL_LINE_TOLERANCE"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed >= 0 {
			lineTol = parsed
		}
	}

	near := func(a, b Site) bool {
		return strings.EqualFold(normalizePath(a.File), normalizePath(b.File)) &&
			math.Abs(float64(a.Line-b.Line)) <= float64(lineTol)
	}

	caught := 0
	for _, g := range sites {
		for _, f := range flags {
			if near(f, g) {
				caught++
				break
			}
		}
	}

	onTarget := 0
	for _, f := range flags {
		for _, g := range sites {
			if near(f, g) {
				onTarget++
				break
			}
		}
	}

	recall := 0.0
	if len(sites) > 0 {
		recall = float64(caught) / float64(len(sites))
	} else if len(flags) == 0 {
		recall = 1.0 // Clean codebase, 0 violations expected and 0 flagged
	}

	precision := 0.0
	if len(flags) > 0 {
		precision = float64(onTarget) / float64(len(flags))
	} else if len(sites) == 0 {
		precision = 1.0
	}

	f1 := 0.0
	if (precision + recall) > 0 {
		f1 = 2 * (precision * recall) / (precision + recall)
	}

	return CaseScore{
		TotalGroundTruth: len(sites),
		TotalFlags:       len(flags),
		Caught:           caught,
		OnTarget:         onTarget,
		Recall:           recall,
		Precision:        precision,
		F1Score:          f1,
	}
}

// ParseViolations extracts violation locations from raw model prose, markdown JSON blocks,
// or direct JSON payloads without crashing on malformed inputs.
func ParseViolations(text string) []Site {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	t := text
	fenceRegex := regexp.MustCompile(`(?s)` + "```" + `(?:json)?\s*([\s\S]*?)` + "```")
	if match := fenceRegex.FindStringSubmatch(t); len(match) > 1 {
		t = strings.TrimSpace(match[1])
	}

	start := strings.Index(t, "{")
	end := strings.LastIndex(t, "}")
	if start >= 0 && end > start {
		t = t[start : end+1]
	}

	var parsed struct {
		Violations []struct {
			File string `json:"file"`
			Line int    `json:"line"`
		} `json:"violations"`
	}

	sites := make([]Site, 0)
	if err := json.Unmarshal([]byte(t), &parsed); err == nil && len(parsed.Violations) > 0 {
		for _, v := range parsed.Violations {
			if v.File != "" && v.Line > 0 {
				sites = append(sites, Site{
					File: normalizePath(v.File),
					Line: v.Line,
				})
			}
		}
		return sites
	}

	// Fallback: array of sites
	var arraySites []struct {
		File string `json:"file"`
		Line int    `json:"line"`
	}
	if err := json.Unmarshal([]byte(t), &arraySites); err == nil {
		for _, v := range arraySites {
			if v.File != "" && v.Line > 0 {
				sites = append(sites, Site{
					File: normalizePath(v.File),
					Line: v.Line,
				})
			}
		}
	}

	return sites
}

func normalizePath(p string) string {
	clean := strings.TrimPrefix(p, "/")
	clean = strings.ReplaceAll(clean, "\\", "/")
	return strings.TrimRight(clean, "/")
}
