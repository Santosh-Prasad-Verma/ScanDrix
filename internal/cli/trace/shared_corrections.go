// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"context"
	"sort"
	"time"
)

// SharedCorrectionResult models the outcome of propagating a pin/forget override to the git trace branch.
type SharedCorrectionResult struct {
	Found     bool   `json:"found"`
	Pushed    bool   `json:"pushed"`
	PushError string `json:"push_error,omitempty"`
}

// UpdateSharedDecisionCorrection updates the decision branch record in scandrix/trace/v1
// so that human corrections (pin/unpin/forget) are committed and published to team clones.
func UpdateSharedDecisionCorrection(
	ctx context.Context,
	gitRoot string,
	decisionID string,
	action string, // "pin", "unpin", "forget"
	remote string,
) (*SharedCorrectionResult, error) {
	if remote == "" {
		remote = "origin"
	}

	records, err := ReadAllBranchRecords(ctx, gitRoot, remote)
	if err != nil {
		records = nil
	}

	// Find all records that contain the decision or reference it
	var matchingRecords []*TraceBranchRecord
	for i := range records {
		rec := &records[i]
		found := false
		for _, d := range rec.Decisions {
			if d.ID == decisionID {
				found = true
				break
			}
		}
		if !found && rec.Corrections != nil {
			for _, id := range rec.Corrections.Pins {
				if id == decisionID {
					found = true
					break
				}
			}
			if !found {
				for _, id := range rec.Corrections.Forgets {
					if id == decisionID {
						found = true
						break
					}
				}
			}
		}
		if found {
			matchingRecords = append(matchingRecords, rec)
		}
	}

	if len(matchingRecords) == 0 {
		return &SharedCorrectionResult{
			Found:  false,
			Pushed: false,
		}, nil
	}

	pushed := false
	var pushErr string

	for _, rec := range matchingRecords {
		if rec.Corrections == nil {
			rec.Corrections = &Overrides{
				Pins:    []string{},
				Forgets: []string{},
			}
		}

		pinnedMap := make(map[string]bool)
		for _, id := range rec.Corrections.Pins {
			pinnedMap[id] = true
		}
		forgottenMap := make(map[string]bool)
		for _, id := range rec.Corrections.Forgets {
			forgottenMap[id] = true
		}

		switch action {
		case "pin":
			pinnedMap[decisionID] = true
			delete(forgottenMap, decisionID)
			for i := range rec.Decisions {
				if rec.Decisions[i].ID == decisionID {
					rec.Decisions[i].Pinned = true
				}
			}
		case "unpin":
			delete(pinnedMap, decisionID)
			for i := range rec.Decisions {
				if rec.Decisions[i].ID == decisionID {
					rec.Decisions[i].Pinned = false
				}
			}
		case "forget":
			forgottenMap[decisionID] = true
			delete(pinnedMap, decisionID)
			var filtered []Decision
			for _, d := range rec.Decisions {
				if d.ID != decisionID {
					filtered = append(filtered, d)
				}
			}
			rec.Decisions = filtered
		}

		// Rebuild slice
		rec.Corrections.Pins = make([]string, 0, len(pinnedMap))
		for id := range pinnedMap {
			rec.Corrections.Pins = append(rec.Corrections.Pins, id)
		}
		sort.Strings(rec.Corrections.Pins)

		rec.Corrections.Forgets = make([]string, 0, len(forgottenMap))
		for id := range forgottenMap {
			rec.Corrections.Forgets = append(rec.Corrections.Forgets, id)
		}
		sort.Strings(rec.Corrections.Forgets)

		rec.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

		// Write to local cache and git object database
		if _, err := WriteBranchRecord(ctx, gitRoot, rec); err != nil {
			continue
		}

		// Push to remote branch
		if err := PushTraceBranch(ctx, gitRoot, remote); err != nil {
			pushErr = err.Error()
		} else {
			pushed = true
		}
	}

	return &SharedCorrectionResult{
		Found:     true,
		Pushed:    pushed,
		PushError: pushErr,
	}, nil
}
