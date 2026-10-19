package threatmodel

import (
	"testing"
)

func TestAnalyzeFileThreats_HTTPEndpoint(t *testing.T) {
	code := `
package server

import "net/http"

func RegisterRoutes(r *Router) {
	r.Get("/api/v1/projects", ListProjectsHandler)
	r.Post("/api/v1/deploy", TriggerDeployHandler)
}
`
	entries := AnalyzeFileThreats("server.go", code)
	if len(entries) != 2 {
		t.Fatalf("expected 2 HTTP endpoints, got %d", len(entries))
	}

	if entries[0].Type != "HTTP_ENDPOINT" || entries[0].Handler != "/api/v1/projects" {
		t.Errorf("unexpected entry 0: %+v", entries[0])
	}
	if entries[0].RiskCategory != StrideElevationOfPrivilege {
		t.Errorf("expected StrideElevationOfPrivilege, got %v", entries[0].RiskCategory)
	}
	if entries[0].Severity != "HIGH" {
		t.Errorf("expected HIGH severity, got %v", entries[0].Severity)
	}
}

func TestAnalyzeFileThreats_SQLSink(t *testing.T) {
	code := `
package db

func FindUser(userId string) {
	query := "SELECT * FROM users WHERE id = " + userId
	db.Query(query + " AND active = 1")
}
`
	entries := AnalyzeFileThreats("repo.go", code)
	if len(entries) < 1 {
		t.Fatalf("expected at least 1 SQL sink, got %d", len(entries))
	}

	found := false
	for _, entry := range entries {
		if entry.Type == "SQL_SINK" {
			found = true
			if entry.RiskCategory != StrideTampering {
				t.Errorf("expected StrideTampering, got %v", entry.RiskCategory)
			}
			if entry.Severity != "CRITICAL" {
				t.Errorf("expected CRITICAL severity, got %v", entry.Severity)
			}
		}
	}
	if !found {
		t.Errorf("expected SQL_SINK to be detected")
	}
}

func TestAnalyzeFileThreats_ExecSink(t *testing.T) {
	code := `
package runner

import "os/exec"

func RunScript(script string) error {
	cmd := exec.Command("bash", "-c", script)
	return cmd.Run()
}
`
	entries := AnalyzeFileThreats("runner.go", code)
	if len(entries) != 1 {
		t.Fatalf("expected 1 exec sink, got %d", len(entries))
	}
	if entries[0].Type != "EXEC_SINK" {
		t.Errorf("expected EXEC_SINK, got %v", entries[0].Type)
	}
	if entries[0].Severity != "CRITICAL" {
		t.Errorf("expected CRITICAL, got %v", entries[0].Severity)
	}
}

func TestAnalyzeFileThreats_SensitiveLogSink(t *testing.T) {
	code := `
package auth

import "log"

func Authenticate(token string) {
	log.Printf("Processing auth request with bearer token: %s", token)
}
`
	entries := AnalyzeFileThreats("auth.go", code)
	if len(entries) != 1 {
		t.Fatalf("expected 1 sensitive log sink, got %d", len(entries))
	}
	if entries[0].Type != "SENSITIVE_LOG_SINK" {
		t.Errorf("expected SENSITIVE_LOG_SINK, got %v", entries[0].Type)
	}
	if entries[0].RiskCategory != StrideInfoDisclosure {
		t.Errorf("expected StrideInfoDisclosure, got %v", entries[0].RiskCategory)
	}
}

func TestDeriveThreatModel(t *testing.T) {
	files := map[string]string{
		"api/routes.go": `
r.Post("/api/v1/execute", RunHandler)
`,
		"api/handler.go": `
func RunHandler(cmd string, token string) {
	log.Printf("executing command with token: %s", token)
	exec.Command("sh", "-c", cmd)
}
`,
	}

	report := DeriveThreatModel(files)
	if report == nil {
		t.Fatal("expected non-nil ThreatModelReport")
	}

	if len(report.TrustBoundaries) != 3 {
		t.Errorf("expected 3 default trust boundaries, got %d", len(report.TrustBoundaries))
	}

	if len(report.AttackSurface) != 3 {
		t.Errorf("expected 3 attack surface entries across 2 files, got %d", len(report.AttackSurface))
	}

	if report.Overview == "" {
		t.Errorf("expected non-empty overview string")
	}

	if report.DAG == nil {
		t.Fatal("expected non-nil ThreatDAG")
	}
	if len(report.DAG.Nodes) != 3 {
		t.Errorf("expected 3 DAG nodes, got %d", len(report.DAG.Nodes))
	}
	// 1 Ingress (HTTP) -> 2 Sinks (Exec & Log) = 2 edges
	if len(report.DAG.Edges) != 2 {
		t.Errorf("expected 2 DAG edges connecting ingress to sinks, got %d", len(report.DAG.Edges))
	}
	if report.DAG.HighRiskPaths != 2 {
		t.Errorf("expected 2 high risk paths, got %d", report.DAG.HighRiskPaths)
	}
}

func TestAnalyzeFileThreats_UploadAndWebhook(t *testing.T) {
	code := `
package web
func Handle(r *http.Request) {
	file, _ := r.FormFile("attachment")
	if VerifySignature(r.Header.Get("X-Hub-Signature")) {
		// webhook
	}
}
`
	entries := AnalyzeFileThreats("web.go", code)
	if len(entries) < 2 {
		t.Fatalf("expected at least 2 entries for upload and webhook, got %d", len(entries))
	}

	var hasUpload, hasWebhook bool
	for _, e := range entries {
		if e.Type == "FILE_UPLOAD" {
			hasUpload = true
		}
		if e.Type == "WEBHOOK_RECEIVER" {
			hasWebhook = true
		}
	}

	if !hasUpload {
		t.Errorf("expected FILE_UPLOAD to be detected")
	}
	if !hasWebhook {
		t.Errorf("expected WEBHOOK_RECEIVER to be detected")
	}
}

