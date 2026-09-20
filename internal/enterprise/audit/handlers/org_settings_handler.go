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

// OrgSettingsHandler audits enterprise organization settings (SSO, IP allowlists, data retention).
type OrgSettingsHandler struct{}

func NewOrgSettingsHandler() *OrgSettingsHandler { return &OrgSettingsHandler{} }
func (h *OrgSettingsHandler) Category() audit.AuditEventCategory {
	return audit.CategoryOrgSettings
}
func (h *OrgSettingsHandler) HandleEvent(ctx context.Context, event audit.EnterpriseLogEvent) error {
	if event.Target.TargetEntityID == "" {
		return fmt.Errorf("org settings audit event missing org target ID")
	}
	return nil
}
