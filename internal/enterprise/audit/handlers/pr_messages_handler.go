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

// PRMessagesHandler audits modifications to pull request comment formats and suppression rules.
type PRMessagesHandler struct{}

func NewPRMessagesHandler() *PRMessagesHandler { return &PRMessagesHandler{} }
func (h *PRMessagesHandler) Category() audit.AuditEventCategory {
	return audit.CategoryPRMessages
}
func (h *PRMessagesHandler) HandleEvent(ctx context.Context, event audit.EnterpriseLogEvent) error {
	if event.Target.TargetEntityID == "" {
		return fmt.Errorf("pr messages audit event missing target entity ID")
	}
	return nil
}
