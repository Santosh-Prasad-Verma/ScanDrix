// Package utils implements PR mutation builders for ScanDrix configuration files and custom review rules.
package utils

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/scandrix/backend/internal/centralizedconfig/domain"
)

// PreviousGroupRuleEntry represents a rule file name or a rule with its content for renames.
type PreviousGroupRuleEntry struct {
	FileName string
	Content  string
}

// PreviousGroupRuleFileNames catalogs existing review and memory rules within a directory group.
type PreviousGroupRuleFileNames struct {
	Review   []PreviousGroupRuleEntry
	Memories []PreviousGroupRuleEntry
}

// ScanDrixConfigMutationParams holds parameters for building a configuration mutation PR.
type ScanDrixConfigMutationParams struct {
	RepositoryFolder       string
	DirectoryPath          string
	Folders                []domain.DirectoryGroupFolderRef
	PreviousFolders        []domain.DirectoryGroupFolderRef
	PreviousRulesFileNames *PreviousGroupRuleFileNames
	ConfigFileContent      map[string]any
	Title                  string
	CommitMessage          string
}

// BuildScanDrixConfigCentralizedMutationRequest generates file operations for updating scandrix-config.yaml and directory group rules.
func BuildScanDrixConfigCentralizedMutationRequest(params ScanDrixConfigMutationParams) ([]domain.FileMutationOp, error) {
	repoFolder := params.RepositoryFolder
	if repoFolder == "" {
		repoFolder = "default"
	}

	isDirectoryGroup := len(params.Folders) > 0

	var newFolderName, oldFolderName string
	var err error

	if isDirectoryGroup {
		var paths []string
		for _, f := range params.Folders {
			paths = append(paths, f.Path)
		}
		newFolderName, err = BuildGroupFolderName(paths)
		if err != nil {
			return nil, err
		}
	}

	if len(params.PreviousFolders) > 0 {
		var oldPaths []string
		for _, f := range params.PreviousFolders {
			oldPaths = append(oldPaths, f.Path)
		}
		oldFolderName, err = BuildGroupFolderName(oldPaths)
		if err != nil {
			return nil, err
		}
	}

	folderRenamed := oldFolderName != "" && newFolderName != "" && oldFolderName != newFolderName
	folderRemoved := oldFolderName != "" && newFolderName == ""

	if isDirectoryGroup || folderRemoved {
		return buildDirectoryGroupFileOps(
			repoFolder,
			newFolderName,
			oldFolderName,
			folderRenamed,
			folderRemoved,
			params.ConfigFileContent,
			params.PreviousRulesFileNames,
		)
	}

	normalizedDir := normalizeDirectoryPath(params.DirectoryPath)
	var targetPath string
	if repoFolder == "global" {
		if normalizedDir != "" {
			targetPath = fmt.Sprintf("%s/scandrix-config.yaml", normalizedDir)
		} else {
			targetPath = "scandrix-config.yaml"
		}
	} else {
		if normalizedDir != "" {
			targetPath = fmt.Sprintf("%s/%s/scandrix-config.yaml", repoFolder, normalizedDir)
		} else {
			targetPath = fmt.Sprintf("%s/scandrix-config.yaml", repoFolder)
		}
	}

	if len(params.ConfigFileContent) == 0 {
		return []domain.FileMutationOp{
			{Path: targetPath, Operation: "delete"},
		}, nil
	}

	yamlContent, err := DumpCentralizedYAML(params.ConfigFileContent)
	if err != nil {
		return nil, err
	}

	return []domain.FileMutationOp{
		{Path: targetPath, Operation: "upsert", Content: yamlContent},
	}, nil
}

func buildDirectoryGroupFileOps(
	repoFolder string,
	newFolderName string,
	oldFolderName string,
	folderRenamed bool,
	folderRemoved bool,
	configFileContent map[string]any,
	previousRules *PreviousGroupRuleFileNames,
) ([]domain.FileMutationOp, error) {
	var ops []domain.FileMutationOp
	hasNewContent := len(configFileContent) > 0

	if newFolderName != "" {
		newConfigPath := fmt.Sprintf("%s/%s/scandrix-config.yaml", repoFolder, newFolderName)
		if hasNewContent {
			yamlContent, err := DumpCentralizedYAML(configFileContent)
			if err != nil {
				return nil, err
			}
			ops = append(ops, domain.FileMutationOp{
				Path:      newConfigPath,
				Operation: "upsert",
				Content:   yamlContent,
			})
		} else if !folderRenamed {
			ops = append(ops, domain.FileMutationOp{
				Path:      newConfigPath,
				Operation: "delete",
			})
		}
	}

	if oldFolderName != "" && (folderRenamed || folderRemoved) {
		oldConfigPath := fmt.Sprintf("%s/%s/scandrix-config.yaml", repoFolder, oldFolderName)
		ops = append(ops, domain.FileMutationOp{
			Path:      oldConfigPath,
			Operation: "delete",
		})

		var newTarget string
		if folderRenamed {
			newTarget = newFolderName
		}

		if previousRules != nil {
			ops = appendRuleMoves(ops, repoFolder, oldFolderName, newTarget, previousRules.Review, "review")
			ops = appendRuleMoves(ops, repoFolder, oldFolderName, newTarget, previousRules.Memories, "memories")
		}
	}

	return ops, nil
}

func appendRuleMoves(
	ops []domain.FileMutationOp,
	repoFolder string,
	oldFolderName string,
	newFolderName string,
	entries []PreviousGroupRuleEntry,
	rulesDir string,
) []domain.FileMutationOp {
	for _, entry := range entries {
		if newFolderName != "" && entry.Content != "" {
			ops = append(ops, domain.FileMutationOp{
				Path:      fmt.Sprintf("%s/%s/.drixy-rules/%s/%s", repoFolder, newFolderName, rulesDir, entry.FileName),
				Operation: "upsert",
				Content:   entry.Content,
			})
		}
		ops = append(ops, domain.FileMutationOp{
			Path:      fmt.Sprintf("%s/%s/.drixy-rules/%s/%s", repoFolder, oldFolderName, rulesDir, entry.FileName),
			Operation: "delete",
		})
	}
	return ops
}

func normalizeDirectoryPath(p string) string {
	clean := strings.Trim(p, "/")
	return clean
}

// RuleMutationParams holds parameters for creating, updating, or deleting a rule in the centralized repository.
type RuleMutationParams struct {
	RepositoryFolder string
	GroupFolderName  string
	RulesDirectory   string // "review" or "memories"
	Rule             domain.RuleFileMeta
	Operation        string // "create", "update", "delete"
}

// BuildRulesCentralizedMutationRequest generates file operations for custom rules under .drixy-rules/.
func BuildRulesCentralizedMutationRequest(params RuleMutationParams) ([]domain.FileMutationOp, error) {
	repoFolder := params.RepositoryFolder
	if repoFolder == "" {
		repoFolder = "default"
	}

	rulesDir := params.RulesDirectory
	if rulesDir == "" {
		if params.Rule.IsMemory {
			rulesDir = "memories"
		} else {
			rulesDir = "review"
		}
	}

	slug := strings.ToLower(strings.ReplaceAll(params.Rule.Title, " ", "-"))
	if slug == "" {
		slug = "custom-rule"
	}
	fileName := fmt.Sprintf("%s.yaml", slug)

	var targetPath string
	if params.GroupFolderName != "" {
		targetPath = fmt.Sprintf("%s/%s/.drixy-rules/%s/%s", repoFolder, params.GroupFolderName, rulesDir, fileName)
	} else if repoFolder == "global" {
		targetPath = fmt.Sprintf(".drixy-rules/%s/%s", rulesDir, fileName)
	} else {
		targetPath = fmt.Sprintf("%s/.drixy-rules/%s/%s", repoFolder, rulesDir, fileName)
	}

	targetPath = filepath.ToSlash(targetPath)

	if strings.ToLower(params.Operation) == "delete" {
		return []domain.FileMutationOp{
			{
				Path:      targetPath,
				Operation: "delete",
			},
		}, nil
	}

	content := params.Rule.Content
	if content == "" {
		ruleMap := map[string]any{
			"title":    params.Rule.Title,
			"rule":     params.Rule.Rule,
			"severity": string(params.Rule.Severity),
			"enabled":  params.Rule.Enabled,
		}
		if params.Rule.Prompt != "" {
			ruleMap["prompt"] = params.Rule.Prompt
		}
		dumped, err := DumpCentralizedYAML(ruleMap)
		if err != nil {
			return nil, err
		}
		content = dumped
	}

	return []domain.FileMutationOp{
		{
			Path:      targetPath,
			Operation: "upsert",
			Content:   content,
		},
	}, nil
}
