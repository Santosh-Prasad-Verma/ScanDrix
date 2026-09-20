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

	"github.com/scandrix/backend/internal/enterprise/audit"
)

// RulesConfigHandler audits custom Drixy rule lifecycles (creation, updates, deletions).
type RulesConfigHandler struct{}

// NewRulesConfigHandler constructs a custom rule audit handler.
func NewRulesConfigHandler() *RulesConfigHandler {
	return &RulesConfigHandler{}
}

func (h *RulesConfigHandler) Category() audit.AuditEventCategory {
	return audit.CategoryDrixyRules
}

func (h *RulesConfigHandler) HandleEvent(ctx context.Context, event audit.EnterpriseLogEvent) error {
	if event.Target.TargetEntityID == "" {
		return fmt.Errorf("rules config event missing target entity ID")
	}

	validActions := map[string]bool{
		"CREATE": true,
		"UPDATE": true,
		"DELETE": true,
		"ENABLE": true,
		"DISABLE": true,
	}

	if !validActions[event.Action] {
		return fmt.Errorf("invalid action %s for custom rules audit", event.Action)
	}

	return nil
}
