// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// CompressedTranscript models a deduplicated, milestone-extracted agent transcript.
type CompressedTranscript struct {
	OriginalEntries int                    `json:"original_entries"`
	CompressedCount int                    `json:"compressed_count"`
	ReductionRatio  float64                `json:"reduction_ratio"`
	UserGoals       []string               `json:"user_goals"`
	KeyDecisions    []string               `json:"key_decisions"`
	ToolSummary     map[string]int         `json:"tool_summary"`
	Milestones      []TranscriptMilestone  `json:"milestones"`
}

// TranscriptMilestone captures an inflection point in an agent session.
type TranscriptMilestone struct {
	TurnIndex   int      `json:"turn_index"`
	Phase       string   `json:"phase"` // e.g. "exploration", "refactoring", "verification"
	Description string   `json:"description"`
	FilesEdited []string `json:"files_edited,omitempty"`
}

// CompressTranscript takes a raw parsed transcript and distills key moments.
func CompressTranscript(raw *TranscriptParseResult) *CompressedTranscript {
	if raw == nil {
		return &CompressedTranscript{
			ToolSummary: make(map[string]int),
		}
	}

	compressed := &CompressedTranscript{
		OriginalEntries: raw.EntryCount,
		ToolSummary:     make(map[string]int),
		UserGoals:       make([]string, 0),
		KeyDecisions:    make([]string, 0),
		Milestones:      make([]TranscriptMilestone, 0),
	}

	// 1. Extract and deduplicate user prompts (goals)
	seenGoals := make(map[string]bool)
	for _, p := range raw.Prompts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		hash := sha256Hex(trimmed)
		if !seenGoals[hash] {
			seenGoals[hash] = true
			if len(trimmed) > 200 {
				trimmed = trimmed[:197] + "..."
			}
			compressed.UserGoals = append(compressed.UserGoals, trimmed)
		}
	}

	// 2. Count tool calls and filter repeated reads
	seenReadTools := make(map[string]bool)
	var activeMilestoneFiles []string

	for _, tc := range raw.ToolCalls {
		toolName := tc.ToolName
		compressed.ToolSummary[toolName]++

		// Deduplicate read-only repetitive scans
		if readTools[toolName] {
			readKey := fmt.Sprintf("%s:%s", toolName, tc.FileAffected)
			if seenReadTools[readKey] {
				continue
			}
			seenReadTools[readKey] = true
		}

		if writeTools[toolName] && tc.FileAffected != "" {
			activeMilestoneFiles = append(activeMilestoneFiles, tc.FileAffected)
		}
	}

	// 3. Extract assistant milestones from text
	for i, msg := range raw.AssistantMessages {
		trimmed := strings.TrimSpace(msg)
		if strings.Contains(trimmed, "I will") || strings.Contains(trimmed, "Decided to") || strings.Contains(trimmed, "Plan:") || strings.Contains(trimmed, "Approach:") {
			firstLine := strings.Split(trimmed, "\n")[0]
			if len(firstLine) > 120 {
				firstLine = firstLine[:117] + "..."
			}
			compressed.KeyDecisions = append(compressed.KeyDecisions, firstLine)

			phase := "exploration"
			if len(activeMilestoneFiles) > 0 {
				phase = "implementation"
			}
			if strings.Contains(strings.ToLower(trimmed), "test") || strings.Contains(strings.ToLower(trimmed), "verif") {
				phase = "verification"
			}

			compressed.Milestones = append(compressed.Milestones, TranscriptMilestone{
				TurnIndex:   i + 1,
				Phase:       phase,
				Description: firstLine,
				FilesEdited: uniqueStrings(activeMilestoneFiles),
			})
		}
	}

	compressed.CompressedCount = len(compressed.UserGoals) + len(compressed.KeyDecisions) + len(compressed.Milestones)
	if compressed.OriginalEntries > 0 {
		compressed.ReductionRatio = 1.0 - (float64(compressed.CompressedCount) / float64(compressed.OriginalEntries))
		if compressed.ReductionRatio < 0 {
			compressed.ReductionRatio = 0
		}
	}

	return compressed
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func uniqueStrings(slice []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range slice {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
