package sca

import (
	"testing"

	"github.com/codehound/codehound/shared/domain"
	"github.com/google/uuid"
)

func TestParseGoMod(t *testing.T) {
	content := []byte(`module github.com/example/app

go 1.23.0

require (
	github.com/gin-gonic/gin v1.9.1
	golang.org/x/net v0.20.0 // indirect
)

require github.com/google/uuid v1.6.0
`)

	deps, err := ParseGoMod(content)
	if err != nil {
		t.Fatalf("failed to parse go.mod: %v", err)
	}

	if len(deps) != 3 {
		t.Fatalf("expected 3 dependencies, got %d", len(deps))
	}

	hasGin := false
	hasNet := false
	for _, d := range deps {
		if d.Name == "github.com/gin-gonic/gin" && d.Direct {
			hasGin = true
		}
		if d.Name == "golang.org/x/net" && !d.Direct {
			hasNet = true
		}
	}

	if !hasGin || !hasNet {
		t.Errorf("expected direct gin and indirect net dependencies")
	}
}

func TestParsePackageJSON(t *testing.T) {
	content := []byte(`{
  "name": "frontend-dashboard",
  "version": "1.0.0",
  "license": "GPL-3.0",
  "dependencies": {
    "ejs": "3.1.5",
    "react": "^18.2.0"
  },
  "devDependencies": {
    "typescript": "^5.0.0"
  }
}`)

	deps, license, err := ParsePackageJSON(content)
	if err != nil {
		t.Fatalf("failed to parse package.json: %v", err)
	}

	if license != "GPL-3.0" {
		t.Errorf("expected GPL-3.0 license, got %s", license)
	}
	if len(deps) != 3 {
		t.Fatalf("expected 3 dependencies (2 prod, 1 dev), got %d", len(deps))
	}
}

func TestGenerateCycloneDXBOM(t *testing.T) {
	deps := []PackageDependency{
		{Name: "react", Version: "18.2.0", Ecosystem: "npm", License: "MIT", Direct: true},
		{Name: "github.com/gin-gonic/gin", Version: "1.9.1", Ecosystem: "Go", License: "MIT", Direct: true},
	}

	bom, err := GenerateCycloneDXBOM("CodeHound-App", deps)
	if err != nil {
		t.Fatalf("failed to generate CycloneDX BOM: %v", err)
	}

	if bom.BOMFormat != "CycloneDX" || bom.SpecVersion != "1.5" {
		t.Errorf("invalid BOM schema: %s v%s", bom.BOMFormat, bom.SpecVersion)
	}
	if len(bom.Components) != 2 {
		t.Errorf("expected 2 components in BOM, got %d", len(bom.Components))
	}
	if bom.Components[0].PURL != "pkg:npm/react@18.2.0" {
		t.Errorf("unexpected PURL: %s", bom.Components[0].PURL)
	}
}

func TestOSVMatchVulnerabilities(t *testing.T) {
	db := NewOSVDatabase()

	deps := []PackageDependency{
		{Name: "ejs", Version: "3.1.5", Ecosystem: "npm", Direct: true},
		{Name: "golang.org/x/net", Version: "0.20.0", Ecosystem: "Go", Direct: false},
		{Name: "safe-package", Version: "1.0.0", Ecosystem: "npm", Direct: true},
	}

	tenantID := uuid.New()
	projectID := uuid.New()
	scanID := uuid.New()

	findings, evidences := db.MatchVulnerabilities(tenantID, projectID, scanID, "package.json", deps)

	if len(findings) != 2 {
		t.Fatalf("expected 2 matched CVE vulnerabilities, got %d", len(findings))
	}

	hasEJS := false
	for _, f := range findings {
		if f.CVEID != nil && *f.CVEID == "CVE-2022-29078" {
			hasEJS = true
			if f.Severity != domain.FindingSeverityCritical {
				t.Errorf("expected CRITICAL severity for ejs SSTI, got %s", f.Severity)
			}
		}
	}

	if !hasEJS {
		t.Errorf("expected ejs CVE match")
	}
	if len(evidences) != len(findings) {
		t.Errorf("expected matching evidence count")
	}
}

func TestLicenseComplianceChecker(t *testing.T) {
	checker := NewLicenseChecker()

	tenantID := uuid.New()
	projectID := uuid.New()
	scanID := uuid.New()

	// Strong Copyleft GPL-3.0
	findings, evidences := checker.CheckLicenseCompliance(tenantID, projectID, scanID, "package.json", "GPL-3.0")
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding for strong copyleft GPL-3.0, got %d", len(findings))
	}
	if findings[0].Category != domain.FindingCategoryLicenseConflict {
		t.Errorf("expected LICENSE_CONFLICT category, got %s", findings[0].Category)
	}
	if len(evidences) != 1 {
		t.Errorf("expected 1 evidence item for license conflict")
	}

	// Permissive MIT (should produce 0 findings)
	mitFindings, _ := checker.CheckLicenseCompliance(tenantID, projectID, scanID, "package.json", "MIT")
	if len(mitFindings) != 0 {
		t.Errorf("expected 0 findings for permissive MIT license, got %d", len(mitFindings))
	}
}
