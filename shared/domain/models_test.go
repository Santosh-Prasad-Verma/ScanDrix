package domain_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/codehound/codehound/shared/domain"
	"github.com/google/uuid"
)

func TestDomainModelsJSONSerialization(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	tenantID := uuid.New()
	projectID := uuid.New()
	scanID := uuid.New()
	findingID := uuid.New()

	// 1. Test Tenant model
	tenant := domain.Tenant{
		ID:        tenantID,
		Slug:      "acme-corp",
		Name:      "Acme Corporation",
		Plan:      domain.TenantPlanEnterprise,
		Status:    domain.TenantStatusActive,
		Settings:  map[string]any{"require_mutation_score": 0.85},
		CreatedAt: now,
		UpdatedAt: now,
	}

	tenantJSON, err := json.Marshal(tenant)
	if err != nil {
		t.Fatalf("failed to marshal Tenant: %v", err)
	}

	var tenantUnmarshaled domain.Tenant
	if err := json.Unmarshal(tenantJSON, &tenantUnmarshaled); err != nil {
		t.Fatalf("failed to unmarshal Tenant: %v", err)
	}
	if tenantUnmarshaled.Slug != "acme-corp" || tenantUnmarshaled.Plan != domain.TenantPlanEnterprise {
		t.Errorf("tenant unmarshaled mismatch: %+v", tenantUnmarshaled)
	}

	// 2. Test Finding and FindingOccurrence models
	finding := domain.Finding{
		ID:              findingID,
		TenantID:        tenantID,
		ProjectID:       projectID,
		CanonicalKey:    "SEC-SQLI-001",
		Category:        domain.FindingCategorySecurityVuln,
		Severity:        domain.FindingSeverityCritical,
		State:           domain.FindingStateVerifiedProven,
		Confidence:      0.99,
		Title:           "SQL Injection in Search Handler",
		Description:     "Unsanitized user input concatenated to SQL statement",
		PrimaryFile:     "api/search.go",
		PrimaryLine:     42,
		FirstSeenScanID: scanID,
		LastSeenScanID:  scanID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	findingJSON, err := json.Marshal(finding)
	if err != nil {
		t.Fatalf("failed to marshal Finding: %v", err)
	}

	var findingUnmarshaled domain.Finding
	if err := json.Unmarshal(findingJSON, &findingUnmarshaled); err != nil {
		t.Fatalf("failed to unmarshal Finding: %v", err)
	}
	if findingUnmarshaled.Severity != domain.FindingSeverityCritical || findingUnmarshaled.State != domain.FindingStateVerifiedProven {
		t.Errorf("finding unmarshaled mismatch: %+v", findingUnmarshaled)
	}

	occ := domain.FindingOccurrence{
		ID:              uuid.New(),
		FindingID:       findingID,
		ScanID:          scanID,
		CommitSHA:       "abc1234567890def",
		FilePath:        "api/search.go",
		StartLine:       40,
		EndLine:         45,
		CodeSnippetHash: "hash123",
		CreatedAt:       now,
	}

	occJSON, err := json.Marshal(occ)
	if err != nil {
		t.Fatalf("failed to marshal FindingOccurrence: %v", err)
	}

	var occUnmarshaled domain.FindingOccurrence
	if err := json.Unmarshal(occJSON, &occUnmarshaled); err != nil {
		t.Fatalf("failed to unmarshal FindingOccurrence: %v", err)
	}
	if occUnmarshaled.CommitSHA != "abc1234567890def" || occUnmarshaled.StartLine != 40 {
		t.Errorf("occurrence unmarshaled mismatch: %+v", occUnmarshaled)
	}
}
