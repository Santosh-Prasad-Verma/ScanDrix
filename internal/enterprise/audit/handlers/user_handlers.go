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

// UserStatusHandler audits user status transitions (active, deactivated, suspended, SCIM).
type UserStatusHandler struct{}

func NewUserStatusHandler() *UserStatusHandler { return &UserStatusHandler{} }
func (h *UserStatusHandler) Category() audit.AuditEventCategory {
	return audit.CategoryUserStatus
}
func (h *UserStatusHandler) HandleEvent(ctx context.Context, event audit.EnterpriseLogEvent) error {
	if event.Target.TargetEntityID == "" {
		return fmt.Errorf("user status event missing target user ID")
	}
	return nil
}

// UserInviteHandler audits enterprise team member invitations and acceptance.
type UserInviteHandler struct{}

func NewUserInviteHandler() *UserInviteHandler { return &UserInviteHandler{} }
func (h *UserInviteHandler) Category() audit.AuditEventCategory {
	return audit.CategoryUserInvite
}
func (h *UserInviteHandler) HandleEvent(ctx context.Context, event audit.EnterpriseLogEvent) error {
	if event.Target.TargetEntityID == "" {
		return fmt.Errorf("user invite event missing invite ID")
	}
	return nil
}

// UserRoleChangeHandler audits RBAC role modifications.
type UserRoleChangeHandler struct{}

func NewUserRoleChangeHandler() *UserRoleChangeHandler { return &UserRoleChangeHandler{} }
func (h *UserRoleChangeHandler) Category() audit.AuditEventCategory {
	return audit.CategoryUserRoleChange
}
func (h *UserRoleChangeHandler) HandleEvent(ctx context.Context, event audit.EnterpriseLogEvent) error {
	if event.Target.TargetEntityID == "" {
		return fmt.Errorf("user role change event missing target user ID")
	}
	return nil
}

// UserRepoAccessHandler audits repository authorization assignments.
type UserRepoAccessHandler struct{}

func NewUserRepoAccessHandler() *UserRepoAccessHandler { return &UserRepoAccessHandler{} }
func (h *UserRepoAccessHandler) Category() audit.AuditEventCategory {
	return audit.CategoryUserRepoAccess
}
func (h *UserRepoAccessHandler) HandleEvent(ctx context.Context, event audit.EnterpriseLogEvent) error {
	if event.Target.TargetEntityID == "" {
		return fmt.Errorf("user repo access event missing target user ID")
	}
	return nil
}
