package scanners

import (
	"testing"

	"github.com/codehound/codehound/shared/domain"
	"github.com/google/uuid"
)

func TestCoordinatorScanRepository(t *testing.T) {
	coordinator := NewCoordinator()

	files := map[string][]byte{
		"server.go": []byte(`package main
import (
	"database/sql"
	"fmt"
)
var AWSSecret = "AKIA1234567890ABCDEF"
func QueryUser(db *sql.DB, id string) {
	q := fmt.Sprintf("SELECT * FROM users WHERE id = '%s'", id)
	db.Query(q)
}
`),
		"package.json": []byte(`{
  "name": "web-frontend",
  "version": "1.0.0",
  "license": "GPL-3.0",
  "dependencies": {
    "ejs": "3.1.5"
  }
}`),
		"Dockerfile": []byte(`FROM alpine:3.19
RUN curl -fsSL https://get.docker.com | sh
CMD ["./server"]
`),
		"infra.tf": []byte(`resource "aws_s3_bucket" "b" {
  acl = "public-read"
}`),
		"deploy.yaml": []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-app
spec:
  template:
    spec:
      containers:
      - name: test
        image: nginx
        securityContext:
          privileged: true
`),
	}

	tenantID := uuid.New()
	projectID := uuid.New()
	scanID := uuid.New()

	result, err := coordinator.ScanRepository(tenantID, projectID, scanID, files)
	if err != nil {
		t.Fatalf("failed to scan repository: %v", err)
	}

	if len(result.Findings) < 6 {
		t.Fatalf("expected at least 6 unified findings across engines, got %d", len(result.Findings))
	}

	hasSecret := false
	hasSAST := false
	hasSCA := false
	hasLicense := false
	hasIaCDocker := false
	hasIaCTF := false
	hasIaCK8s := false

	for _, f := range result.Findings {
		switch f.Category {
		case domain.FindingCategorySecretLeak:
			hasSecret = true
		case domain.FindingCategorySecurityVuln:
			if f.CVEID != nil {
				hasSCA = true
			} else {
				hasSAST = true
			}
		case domain.FindingCategoryLicenseConflict:
			hasLicense = true
		case domain.FindingCategoryIaCMisconfig:
			if f.PrimaryFile == "Dockerfile" {
				hasIaCDocker = true
			} else if f.PrimaryFile == "infra.tf" {
				hasIaCTF = true
			} else if f.PrimaryFile == "deploy.yaml" {
				hasIaCK8s = true
			}
		}
	}

	if !hasSecret {
		t.Errorf("missing Secret leak finding")
	}
	if !hasSAST {
		t.Errorf("missing SAST SQLi finding")
	}
	if !hasSCA {
		t.Errorf("missing SCA CVE finding")
	}
	if !hasLicense {
		t.Errorf("missing License conflict finding")
	}
	if !hasIaCDocker {
		t.Errorf("missing Dockerfile IaC finding")
	}
	if !hasIaCTF {
		t.Errorf("missing Terraform IaC finding")
	}
	if !hasIaCK8s {
		t.Errorf("missing Kubernetes IaC finding")
	}

	if result.SBOM == nil || len(result.SBOM.Components) != 1 {
		t.Errorf("expected CycloneDX SBOM with 1 component, got %+v", result.SBOM)
	}
	if len(result.Evidences) < len(result.Findings) {
		t.Errorf("expected at least %d evidences, got %d", len(result.Findings), len(result.Evidences))
	}
}
