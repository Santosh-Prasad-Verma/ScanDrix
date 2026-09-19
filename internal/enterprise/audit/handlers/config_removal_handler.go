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

// ConfigRemovalHandler audits deletions of repository and directory-level review parameters.
type ConfigRemovalHandler struct {
	category audit.AuditEventCategory
}

// NewConfigRemovalHandler constructs a config removal audit handler.
func NewConfigRemovalHandler(category audit.AuditEventCategory) *ConfigRemovalHandler {
	return &ConfigRemovalHandler{category: category}
}

func (h *ConfigRemovalHandler) Category() audit.AuditEventCategory {
	return h.category
}

func (h *ConfigRemovalHandler) HandleEvent(ctx context.Context, event audit.EnterpriseLogEvent) error {
	if event.Target.TargetEntityID == "" {
		return fmt.Errorf("config removal audit event missing target entity ID")
	}
	return nil
}
