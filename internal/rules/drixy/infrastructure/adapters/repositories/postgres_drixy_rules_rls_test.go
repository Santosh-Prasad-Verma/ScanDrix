// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: postgres_drixy_rules_rls_test.go
// ═══════════════════════════════════════════════════════════════

package repositories

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/rules/drixy/domain/entities"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
)

// These tests exercise the tenant plumbing added for AUDIT_REMEDIATION.md
// F-37. They matter because the failure mode they guard is silent: when the
// repository queried through client.Pool without setting the RLS context, the
// policy admitted zero rows and the caller received "this workspace has no
// rules" instead of an error (AGENTS.md 2.7.2).
//
// Skips unless SCANDRIX_E2E_RUNTIME_DSN points at a live database.

func liveRulesClient(t *testing.T) *database.Client {
	t.Helper()
	dsn := os.Getenv("SCANDRIX_E2E_RUNTIME_DSN")
	if dsn == "" {
		t.Skip("set SCANDRIX_E2E_RUNTIME_DSN to a live database to run this test")
	}
	client, err := database.NewClient(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(client.Close)
	return client
}

func TestDrixyRulesRepositoryIsTenantScoped(t *testing.T) {
	client := liveRulesClient(t)
	ctx := context.Background()
	repo := NewPostgresDrixyRulesRepository(client)

	tenantA := uuid.New().String()
	tenantB := uuid.New().String()

	// Each tenant gets its own rules document.
	for _, tenant := range []string{tenantA, tenantB} {
		_, err := repo.Create(ctx, &interfaces.DrixyRules{
			OrganizationID: tenant,
			Rules: []interfaces.DrixyRule{{
				UUID:         "rule-" + tenant[:8],
				RepositoryID: "repo-1",
				Title:        "probe",
				Rule:         "x",
				Status:       interfaces.DrixyRulesStatusActive,
			}},
		})
		if err != nil {
			t.Fatalf("create for %s: %v", tenant[:8], err)
		}
	}

	// A tenant sees its own rules.
	for _, tenant := range []string{tenantA, tenantB} {
		entity, err := repo.FindByOrganizationID(ctx, tenant)
		if err != nil {
			t.Fatalf("find for %s: %v", tenant[:8], err)
		}
		if entity == nil {
			t.Fatalf("tenant %s must see its own rules; a nil here means the RLS context was not set", tenant[:8])
		}
		if got := len(entity.Rules()); got != 1 {
			t.Fatalf("tenant %s: expected 1 rule, got %d", tenant[:8], got)
		}
	}

	// A rule written for one tenant must never be returned for another. This is
	// the property RLS is supposed to provide at the repository boundary.
	entityB, err := repo.FindByOrganizationID(ctx, tenantB)
	if err != nil {
		t.Fatalf("find for B: %v", err)
	}
	if entityB != nil {
		for _, rule := range entityB.Rules() {
			if rule.UUID == "rule-"+tenantA[:8] {
				t.Fatalf("tenant B was served tenant A's rule %s", rule.UUID)
			}
		}
	}
}

// TestDrixyRulesSaveRejectsInvalidTenant proves the tenant id is validated
// rather than silently passed through to set_config.
func TestDrixyRulesSaveRejectsInvalidTenant(t *testing.T) {
	client := liveRulesClient(t)
	repo := NewPostgresDrixyRulesRepository(client)

	entity := entities.NewDrixyRulesEntity(interfaces.DrixyRules{
		OrganizationID: "not-a-uuid",
		Rules:          []interfaces.DrixyRule{},
	})
	if err := repo.Save(context.Background(), entity); err == nil {
		t.Fatal("a non-uuid organization id must be rejected, not silently coerced")
	}
}
