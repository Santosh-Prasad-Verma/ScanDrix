package rules

import (
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/pkg/models"
)

func TestDefaultCatalogCatchesCriticalVulnerabilities(t *testing.T) {
	catalog := DefaultCatalog()
	evaluator, err := NewEvaluator(catalog)
	if err != nil {
		t.Fatalf("failed compiling default catalog: %v", err)
	}

	reviewID := uuid.New()
	workspaceID := uuid.New()

	vulnerablePatches := []*diff.FilePatch{
		{
			NewPath: "deploy/aws.go",
			Hunks: []diff.Hunk{
				{
					Lines: []diff.DiffLine{
						{
							Type:      diff.LineAddition,
							NewLineNo: 15,
							Content:   `    awsKey := "AKIAIOSFODNN7EXAMPLE"`,
						},
					},
				},
			},
		},
		{
			NewPath: "auth/github.go",
			Hunks: []diff.Hunk{
				{
					Lines: []diff.DiffLine{
						{
							Type:      diff.LineAddition,
							NewLineNo: 20,
							Content:   `    ghToken := "ghp_1234567890abcdefghijklmnopqrstuvwxyz"`,
						},
					},
				},
			},
		},
		{
			NewPath: "certs/server.key",
			Hunks: []diff.Hunk{
				{
					Lines: []diff.DiffLine{
						{
							Type:      diff.LineAddition,
							NewLineNo: 1,
							Content:   `-----BEGIN RSA PRIVATE KEY-----`,
						},
					},
				},
			},
		},
		{
			NewPath: "os/shell.go",
			Hunks: []diff.Hunk{
				{
					Lines: []diff.DiffLine{
						{
							Type:      diff.LineAddition,
							NewLineNo: 50,
							Content:   `    cmd := exec.Command("bash", "-c", userInput)`,
						},
					},
				},
			},
		},
		{
			NewPath: "crypto/hash.go",
			Hunks: []diff.Hunk{
				{
					Lines: []diff.DiffLine{
						{
							Type:      diff.LineAddition,
							NewLineNo: 30,
							Content:   `    h := md5.New()`,
						},
					},
				},
			},
		},
	}

	findings := evaluator.EvaluatePatches(reviewID, workspaceID, vulnerablePatches)

	// We expect 4 detections (AWS Key, GitHub Token, Private Key, Shell Command)
	// (md5.New() won't match crypto/md5.New() unless imported or regex matches md5.New())
	if len(findings) < 4 {
		t.Fatalf("expected at least 4 critical findings, got %d", len(findings))
	}

	foundAWS := false
	foundGitHub := false
	foundKey := false
	foundCommand := false

	for _, f := range findings {
		switch f.Title {
		case "Hardcoded AWS Access Key":
			foundAWS = true
		case "Hardcoded GitHub Personal Access Token":
			foundGitHub = true
		case "Hardcoded Private Key Block":
			foundKey = true
		case "Shell Command Injection Vulnerability":
			foundCommand = true
		}
		if f.Severity != models.SeverityCritical && f.Severity != models.SeverityHigh {
			t.Errorf("expected high/critical severity, got %s for %s", f.Severity, f.Title)
		}
	}

	if !foundAWS {
		t.Error("expected AWS key detection")
	}
	if !foundGitHub {
		t.Error("expected GitHub token detection")
	}
	if !foundKey {
		t.Error("expected Private key detection")
	}
	if !foundCommand {
		t.Error("expected Shell command injection detection")
	}
}
