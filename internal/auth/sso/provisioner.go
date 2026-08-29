package sso

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// JITProvisioner handles Just-In-Time account synchronization upon SSO federation.
type JITProvisioner struct {
	mu    sync.RWMutex
	users map[string]*models.AccountProfile // key: email
}

// NewJITProvisioner initializes the JIT provisioner.
func NewJITProvisioner() *JITProvisioner {
	return &JITProvisioner{
		users: make(map[string]*models.AccountProfile),
	}
}

// ProvisionUser evaluates domain whitelist, maps enterprise groups to RBAC roles, and provisions user.
func (p *JITProvisioner) ProvisionUser(ctx context.Context, idp IdPConfiguration, ident *FederatedIdentity) (*models.AccountProfile, bool, error) {
	if ident == nil || ident.Email == "" {
		return nil, false, fmt.Errorf("federated identity has no email")
	}

	emailParts := strings.Split(ident.Email, "@")
	if len(emailParts) != 2 {
		return nil, false, fmt.Errorf("invalid email address format: %s", ident.Email)
	}
	domain := strings.ToLower(emailParts[1])

	// 1. Enforce AllowedDomains whitelist
	if len(idp.AllowedDomains) > 0 {
		domainAllowed := false
		for _, d := range idp.AllowedDomains {
			if strings.ToLower(d) == domain {
				domainAllowed = true
				break
			}
		}
		if !domainAllowed {
			return nil, false, fmt.Errorf("domain '%s' is not authorized for SSO authentication on this workspace", domain)
		}
	}

	// 2. Evaluate Role Mapping from Enterprise Groups
	assignedRole := models.RoleMember
	for _, group := range ident.Groups {
		if roleStr, exists := idp.RoleMapping[group]; exists {
			switch strings.ToUpper(roleStr) {
			case "ADMIN", "ORGANIZATION_ADMIN":
				assignedRole = models.RoleAdmin
			case "MEMBER", "ENGINEER", "DEVELOPER":
				assignedRole = models.RoleMember
			case "VIEWER", "AUDITOR":
				assignedRole = models.RoleViewer
			}
		}
	}
	ident.MappedRole = assignedRole

	displayName := strings.TrimSpace(ident.FirstName + " " + ident.LastName)
	if displayName == "" {
		displayName = ident.Email
	}

	// 3. Atomically Provision or Update Account Profile
	p.mu.Lock()
	defer p.mu.Unlock()

	user, exists := p.users[ident.Email]
	isNew := false
	now := time.Now().UTC()

	if !exists {
		isNew = true
		user = &models.AccountProfile{
			ID:          uuid.New(),
			WorkspaceID: idp.WorkspaceID,
			Email:       ident.Email,
			DisplayName: displayName,
			Role:        assignedRole,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		p.users[ident.Email] = user
	} else {
		// Sync latest federated role and display name
		user.Role = assignedRole
		user.DisplayName = displayName
		user.UpdatedAt = now
	}

	return user, isNew, nil
}

// GetUser returns a provisioned user by email.
func (p *JITProvisioner) GetUser(email string) *models.AccountProfile {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.users[email]
}
