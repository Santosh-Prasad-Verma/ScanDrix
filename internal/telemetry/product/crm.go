// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package product

import (
	"context"

	"github.com/scandrix/backend/internal/integrations/zoho"
)

// CRMClient defines the contract for syncing CRM contacts/leads.
type CRMClient interface {
	IsEnabled() bool
	UpsertLead(ctx context.Context, lead zoho.Lead) (string, error)
}

// Ensure zoho.Client implements CRMClient
var _ CRMClient = (*zoho.Client)(nil)
