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

// RepositoriesHandler audits repository onboarding, monitoring status, and webhook configurations.
type RepositoriesHandler struct{}

// NewRepositoriesHandler constructs a repository audit handler.
func NewRepositoriesHandler() *RepositoriesHandler {
	return &RepositoriesHandler{}
}

func (h *RepositoriesHandler) Category() audit.AuditEventCategory {
	return audit.CategoryRepositories
}

func (h *RepositoriesHandler) HandleEvent(ctx context.Context, event audit.EnterpriseLogEvent) error {
	if event.Target.TargetEntityID == "" {
		return fmt.Errorf("repositories audit event missing repository target ID")
	}
	return nil
}
