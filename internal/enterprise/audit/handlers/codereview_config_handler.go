// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package handlers

import (
	"context"
	"fmt"
	"reflect"

	"github.com/scandrix/backend/internal/enterprise/audit"
)

// CodeReviewConfigHandler processes and diffs code review parameter mutations.
type CodeReviewConfigHandler struct{}

// NewCodeReviewConfigHandler constructs a new config audit handler.
func NewCodeReviewConfigHandler() *CodeReviewConfigHandler {
	return &CodeReviewConfigHandler{}
}

func (h *CodeReviewConfigHandler) Category() audit.AuditEventCategory {
	return audit.CategoryCodeReviewConfig
}

// HandleEvent validates and computes before-and-after diffs for review configuration changes.
func (h *CodeReviewConfigHandler) HandleEvent(ctx context.Context, event audit.EnterpriseLogEvent) error {
	if event.Target.TargetEntityID == "" {
		return fmt.Errorf("code review config audit event missing target entity ID")
	}

	// Validate changes exist on UPDATE
	if event.Action == "UPDATE" && len(event.Changes) == 0 {
		return fmt.Errorf("update event for %s must specify at least one field change", event.Target.TargetEntityID)
	}

	return nil
}

// ComputeConfigDiff compares an old and new configuration map and returns a slice of FieldChanges.
func (h *CodeReviewConfigHandler) ComputeConfigDiff(oldCfg, newCfg map[string]any) []audit.FieldChange {
	var changes []audit.FieldChange

	allKeys := make(map[string]struct{})
	for k := range oldCfg {
		allKeys[k] = struct{}{}
	}
	for k := range newCfg {
		allKeys[k] = struct{}{}
	}

	for k := range allKeys {
		oldVal, oldExists := oldCfg[k]
		newVal, newExists := newCfg[k]

		if !oldExists && newExists {
			changes = append(changes, audit.FieldChange{
				Field:    k,
				OldValue: nil,
				NewValue: newVal,
			})
		} else if oldExists && !newExists {
			changes = append(changes, audit.FieldChange{
				Field:    k,
				OldValue: oldVal,
				NewValue: nil,
			})
		} else if !reflect.DeepEqual(oldVal, newVal) {
			changes = append(changes, audit.FieldChange{
				Field:    k,
				OldValue: oldVal,
				NewValue: newVal,
			})
		}
	}

	return changes
}
