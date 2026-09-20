package utils_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/centralizedconfig/domain"
	"github.com/scandrix/backend/internal/centralizedconfig/utils"
	"gopkg.in/yaml.v3"
)

func TestPathEncoding(t *testing.T) {
	paths := []string{"src/api", "internal/core"}
	folderName, err := utils.BuildGroupFolderName(paths)
	if err != nil {
		t.Fatalf("BuildGroupFolderName failed: %v", err)
	}

	decoded, err := utils.ParseGroupFolderName(folderName)
	if err != nil {
		t.Fatalf("ParseGroupFolderName failed: %v", err)
	}

	if len(decoded) != 2 || decoded[0] != "internal/core" || decoded[1] != "src/api" {
		t.Fatalf("unexpected decoded paths: %v", decoded)
	}
}

func TestBuildScanDrixConfigCentralizedMutationRequest_DirectoryGroupFlow(t *testing.T) {
	t.Run("emits a single upsert at the encoded folder when paths are unchanged", func(t *testing.T) {
		req, err := utils.BuildScanDrixConfigCentralizedMutationRequest(utils.ScanDrixConfigMutationParams{
			RepositoryFolder: "repo-1-name",
			Folders: []domain.DirectoryGroupFolderRef{
				{Path: "app/api"},
				{Path: "app/web"},
			},
			ConfigFileContent: map[string]any{
				"version": "1",
			},
			Title:         "t",
			CommitMessage: "c",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(req) != 1 {
			t.Fatalf("expected 1 file op, got %d", len(req))
		}
		if req[0].Path != "repo-1-name/app%2Fapi&app%2Fweb/scandrix-config.yaml" {
			t.Fatalf("unexpected path: %s", req[0].Path)
		}
		if req[0].Operation != "upsert" {
			t.Fatalf("unexpected op: %s", req[0].Operation)
		}

		var parsed map[string]any
		_ = yaml.Unmarshal([]byte(req[0].Content), &parsed)
		if parsed["version"] != "1" {
			t.Fatalf("unexpected parsed content: %v", parsed)
		}
	})

	t.Run("emits upsert(new) + delete(old config + old rules) when paths change", func(t *testing.T) {
		req, err := utils.BuildScanDrixConfigCentralizedMutationRequest(utils.ScanDrixConfigMutationParams{
			RepositoryFolder: "repo-1-name",
			Folders: []domain.DirectoryGroupFolderRef{
				{Path: "app/api"},
				{Path: "app/web"},
			},
			PreviousFolders: []domain.DirectoryGroupFolderRef{
				{Path: "app/api"},
			},
			PreviousRulesFileNames: &utils.PreviousGroupRuleFileNames{
				Review: []utils.PreviousGroupRuleEntry{
					{FileName: "no-console.yml"},
					{FileName: "naming.yml"},
				},
				Memories: []utils.PreviousGroupRuleEntry{
					{FileName: "style-guide.yml"},
				},
			},
			ConfigFileContent: map[string]any{
				"version": "1",
			},
			Title:         "t",
			CommitMessage: "c",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var stringified []string
		for _, op := range req {
			stringified = append(stringified, op.Operation+" "+op.Path)
		}
		sort.Strings(stringified)

		expected := []string{
			"delete repo-1-name/app%2Fapi/.drixy-rules/memories/style-guide.yml",
			"delete repo-1-name/app%2Fapi/.drixy-rules/review/naming.yml",
			"delete repo-1-name/app%2Fapi/.drixy-rules/review/no-console.yml",
			"delete repo-1-name/app%2Fapi/scandrix-config.yaml",
			"upsert repo-1-name/app%2Fapi&app%2Fweb/scandrix-config.yaml",
		}

		if len(stringified) != len(expected) {
			t.Fatalf("expected %d ops, got %d: %v", len(expected), len(stringified), stringified)
		}
		for i := range expected {
			if stringified[i] != expected[i] {
				t.Fatalf("at index %d: expected %q, got %q", i, expected[i], stringified[i])
			}
		}
	})

	t.Run("moves rule files to the new folder (upsert+delete) when content is provided on a rename", func(t *testing.T) {
		req, err := utils.BuildScanDrixConfigCentralizedMutationRequest(utils.ScanDrixConfigMutationParams{
			RepositoryFolder: "repo-1-name",
			Folders: []domain.DirectoryGroupFolderRef{
				{Path: "app/api"},
				{Path: "app/web"},
			},
			PreviousFolders: []domain.DirectoryGroupFolderRef{
				{Path: "app/api"},
			},
			PreviousRulesFileNames: &utils.PreviousGroupRuleFileNames{
				Review: []utils.PreviousGroupRuleEntry{
					{FileName: "no-console.yml", Content: "title: no console"},
				},
			},
			ConfigFileContent: map[string]any{
				"version": "1",
			},
			Title:         "t",
			CommitMessage: "c",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var stringified []string
		for _, op := range req {
			stringified = append(stringified, op.Operation+" "+op.Path)
		}
		sort.Strings(stringified)

		expected := []string{
			"delete repo-1-name/app%2Fapi/.drixy-rules/review/no-console.yml",
			"delete repo-1-name/app%2Fapi/scandrix-config.yaml",
			"upsert repo-1-name/app%2Fapi&app%2Fweb/.drixy-rules/review/no-console.yml",
			"upsert repo-1-name/app%2Fapi&app%2Fweb/scandrix-config.yaml",
		}

		if len(stringified) != len(expected) {
			t.Fatalf("expected %d ops, got %d: %v", len(expected), len(stringified), stringified)
		}
		for i := range expected {
			if stringified[i] != expected[i] {
				t.Fatalf("at index %d: expected %q, got %q", i, expected[i], stringified[i])
			}
		}
	})
}

func TestPRBuilders(t *testing.T) {
	cfgParams := utils.ScanDrixConfigMutationParams{
		RepositoryFolder: "frontend-repo",
		ConfigFileContent: map[string]any{
			"version": "1.2",
			"review": map[string]any{
				"maxLines": 500,
			},
		},
	}

	ops, err := utils.BuildScanDrixConfigCentralizedMutationRequest(cfgParams)
	if err != nil {
		t.Fatalf("BuildScanDrixConfigCentralizedMutationRequest failed: %v", err)
	}
	if len(ops) != 1 || ops[0].Path != "frontend-repo/scandrix-config.yaml" {
		t.Fatalf("unexpected ops: %v", ops)
	}

	ruleParams := utils.RuleMutationParams{
		RepositoryFolder: "backend-repo",
		Rule: domain.RuleFileMeta{
			Title:    "No Panic in Production",
			Rule:     "Always return error instead of panic.",
			Severity: "critical",
			Scope:    "file",
			Path:     "**/*.go",
			Enabled:  true,
		},
		Operation: "create",
	}

	ruleOps, err := utils.BuildRulesCentralizedMutationRequest(ruleParams)
	if err != nil {
		t.Fatalf("BuildRulesCentralizedMutationRequest failed: %v", err)
	}
	if len(ruleOps) != 1 || !strings.Contains(ruleOps[0].Path, "no-panic-in-production.yaml") {
		t.Fatalf("unexpected ruleOps: %v", ruleOps)
	}
}
