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

// IntegrationHandler audits third-party SCM (GitHub, GitLab, Bitbucket, Azure DevOps, Forgejo) and PM connections.
type IntegrationHandler struct{}

// NewIntegrationHandler constructs an integration audit handler.
func NewIntegrationHandler() *IntegrationHandler {
	return &IntegrationHandler{}
}

func (h *IntegrationHandler) Category() audit.AuditEventCategory {
	return audit.CategoryIntegration
}

func (h *IntegrationHandler) HandleEvent(ctx context.Context, event audit.EnterpriseLogEvent) error {
	if event.Target.TargetEntityID == "" {
		return fmt.Errorf("integration audit event missing provider target ID")
	}
	return nil
}
