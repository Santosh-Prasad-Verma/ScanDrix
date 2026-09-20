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

// CliKeyHandler audits team CLI token generation, rotation, and revocation.
type CliKeyHandler struct{}

func NewCliKeyHandler() *CliKeyHandler { return &CliKeyHandler{} }
func (h *CliKeyHandler) Category() audit.AuditEventCategory {
	return audit.CategoryCliKey
}
func (h *CliKeyHandler) HandleEvent(ctx context.Context, event audit.EnterpriseLogEvent) error {
	if event.Target.TargetEntityID == "" {
		return fmt.Errorf("cli key audit event missing key target ID")
	}
	return nil
}
