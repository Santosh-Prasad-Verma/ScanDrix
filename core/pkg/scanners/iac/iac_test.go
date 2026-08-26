package iac

import (
	"testing"

	"github.com/codehound/codehound/shared/domain"
	"github.com/google/uuid"
)

func TestScanDockerfile(t *testing.T) {
	scanner := NewDockerfileScanner()

	dockerfile := []byte(`
FROM node:latest
WORKDIR /app
COPY . .
RUN curl -fsSL https://insecure.site/install.sh | bash
EXPOSE 22
EXPOSE 3000
CMD ["npm", "start"]
`)

	tenantID := uuid.New()
	projectID := uuid.New()
	scanID := uuid.New()

	findings, evidences := scanner.ScanDockerfile(tenantID, projectID, scanID, "Dockerfile", dockerfile)

	if len(findings) < 4 {
		t.Fatalf("expected at least 4 Dockerfile findings, got %d", len(findings))
	}

	hasCurlPipe := false
	hasSSH := false
	hasLatest := false
	hasRootUser := false

	for _, f := range findings {
		if f.Category != domain.FindingCategoryIaCMisconfig {
			t.Errorf("expected IaC category, got %s", f.Category)
		}
		if f.Severity == domain.FindingSeverityCritical {
			hasCurlPipe = true
		}
		if f.Title == "[CODEHOUND-IAC-DOCKER-002] Exposing Insecure Remote Management Port (SSH Port 22)" {
			hasSSH = true
		}
		if f.Title == "[CODEHOUND-IAC-DOCKER-003] Use of Mutable Image Tag ':latest'" {
			hasLatest = true
		}
		if f.Title == "[CODEHOUND-IAC-DOCKER-004] Missing Non-Root USER Instruction in Dockerfile" {
			hasRootUser = true
		}
	}

	if !hasCurlPipe || !hasSSH || !hasLatest || !hasRootUser {
		t.Errorf("missing expected Dockerfile vulnerability findings")
	}
	if len(evidences) != len(findings) {
		t.Errorf("evidence count mismatch")
	}
}

func TestScanTerraform(t *testing.T) {
	scanner := NewTerraformScanner()

	tfCode := []byte(`
resource "aws_s3_bucket" "data_bucket" {
  bucket = "company-sensitive-records"
  acl    = "public-read"
}

resource "aws_security_group" "allow_all" {
  name        = "allow_all"
  description = "Allow all inbound traffic"

  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_db_instance" "prod_db" {
  allocated_storage = 50
  engine            = "postgres"
  storage_encrypted = false
}
`)

	tenantID := uuid.New()
	projectID := uuid.New()
	scanID := uuid.New()

	findings, evidences := scanner.ScanTerraform(tenantID, projectID, scanID, "main.tf", tfCode)

	if len(findings) != 3 {
		t.Fatalf("expected 3 Terraform findings, got %d", len(findings))
	}

	hasPublicS3 := false
	hasOpenSG := false
	hasUnencryptedDB := false

	for _, f := range findings {
		if f.Severity == domain.FindingSeverityCritical {
			hasPublicS3 = true
		}
		if f.Title == "[CODEHOUND-IAC-TF-SG-001] Open Ingress to Sensitive Port (0.0.0.0/0)" {
			hasOpenSG = true
		}
		if f.Title == "[CODEHOUND-IAC-TF-ENC-001] Unencrypted Storage Volume / Database" {
			hasUnencryptedDB = true
		}
	}

	if !hasPublicS3 || !hasOpenSG || !hasUnencryptedDB {
		t.Errorf("missing expected Terraform findings")
	}
	if len(evidences) != 3 {
		t.Errorf("evidence count mismatch")
	}
}

func TestScanK8sManifest(t *testing.T) {
	scanner := NewK8sScanner()

	k8sYAML := []byte(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: insecure-workload
spec:
  template:
    spec:
      hostPID: true
      containers:
      - name: app
        image: nginx
        securityContext:
          privileged: true
        volumeMounts:
        - mountPath: /host
          name: host-volume
      volumes:
      - name: host-volume
        hostPath:
          path: /
`)

	tenantID := uuid.New()
	projectID := uuid.New()
	scanID := uuid.New()

	findings, evidences := scanner.ScanK8sManifest(tenantID, projectID, scanID, "deployment.yaml", k8sYAML)

	if len(findings) != 3 {
		t.Fatalf("expected 3 Kubernetes findings, got %d", len(findings))
	}

	hasPrivileged := false
	hasHostPID := false
	hasHostPath := false

	for _, f := range findings {
		if f.Severity == domain.FindingSeverityCritical {
			hasPrivileged = true
		}
		if f.Title == "[CODEHOUND-IAC-K8S-002] Shared Host PID / Network Namespace" {
			hasHostPID = true
		}
		if f.Title == "[CODEHOUND-IAC-K8S-003] HostPath Volume Mount" {
			hasHostPath = true
		}
	}

	if !hasPrivileged || !hasHostPID || !hasHostPath {
		t.Errorf("missing expected K8s findings")
	}
	if len(evidences) != 3 {
		t.Errorf("evidence count mismatch")
	}
}
