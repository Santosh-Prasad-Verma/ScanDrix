// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: drixy_rules_sync_service.go
// ═══════════════════════════════════════════════════════════════

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/domain/entities"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
	"github.com/scandrix/backend/internal/rules/drixy/domain/prompts"
	"github.com/scandrix/backend/internal/rules/drixy/domain/utils"
)

// SyncRepositoryParams specifies parameters for synchronizing repository rule files.
type SyncRepositoryParams struct {
	OrganizationID   string
	TeamID           string
	RepositoryID     string
	RepositoryName   string
	DefaultBranch    string
	LocalPath        string
	FileMap          map[string]string // Optional in-memory file contents (path -> content)
	MaxFiles         int
	MaxFileSizeBytes int64
	MaxTotalBytes    int64
	MaxConcurrent    int
}

// SyncRepositoryResult captures summary of repository sync operations.
type SyncRepositoryResult struct {
	Rules        []interfaces.DrixyRule `json:"rules"`
	TotalScanned int                    `json:"totalScanned"`
	SkippedFiles []SkippedFileInfo      `json:"skippedFiles,omitempty"`
	Errors       []string               `json:"errors,omitempty"`
}

type SkippedFileInfo struct {
	File   string `json:"file"`
	Reason string `json:"reason"`
}

// ChangedFile represents a file modified in a pull request or commit.
type ChangedFile struct {
	Filename         string `json:"filename"`
	PreviousFilename string `json:"previous_filename,omitempty"`
	Status           string `json:"status"` // "added", "modified", "removed", "renamed"
	Content          string `json:"content,omitempty"`
}

// DrixyRulesSyncService coordinates file-based synchronization of rules from Git repositories.
type DrixyRulesSyncService struct {
	rulesService contracts.IDrixyRulesService
	llmGateway   *llm.Gateway
}

// NewDrixyRulesSyncService constructs a new sync service.
func NewDrixyRulesSyncService(service contracts.IDrixyRulesService) *DrixyRulesSyncService {
	return &DrixyRulesSyncService{rulesService: service}
}

// SetLLMGateway attaches the LLM gateway for AI-assisted rule file parsing.
func (s *DrixyRulesSyncService) SetLLMGateway(gw *llm.Gateway) {
	s.llmGateway = gw
}

// HasPinnedSyncMarker checks if file content carries the per-file sync pin marker.
func HasPinnedSyncMarker(content string) bool {
	return strings.Contains(content, "@drixy-sync")
}

// SyncRepositoryMainFast discovers, parses, and imports rules from repository rule files.
func (s *DrixyRulesSyncService) SyncRepositoryMainFast(ctx context.Context, params SyncRepositoryParams) (*SyncRepositoryResult, error) {
	if s.rulesService == nil {
		return &SyncRepositoryResult{}, nil
	}

	maxFiles := params.MaxFiles
	if maxFiles <= 0 {
		maxFiles = 20
	}
	maxFileSizeBytes := params.MaxFileSizeBytes
	if maxFileSizeBytes <= 0 {
		maxFileSizeBytes = 200 * 1024 // 200 KB
	}
	maxTotalBytes := params.MaxTotalBytes
	if maxTotalBytes <= 0 {
		maxTotalBytes = 2 * 1024 * 1024 // 2 MB
	}

	res := &SyncRepositoryResult{
		Rules:        make([]interfaces.DrixyRule, 0),
		SkippedFiles: make([]SkippedFileInfo, 0),
		Errors:       make([]string, 0),
	}

	// 1. Gather candidate files
	discoveredFiles := make(map[string]string)

	if len(params.FileMap) > 0 {
		for p, c := range params.FileMap {
			if utils.IsIdeRuleSource(p) {
				discoveredFiles[p] = c
			}
		}
	} else if params.LocalPath != "" {
		_ = filepath.Walk(params.LocalPath, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			rel, relErr := filepath.Rel(params.LocalPath, path)
			if relErr != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			if utils.IsIdeRuleSource(rel) {
				if info.Size() > maxFileSizeBytes {
					res.SkippedFiles = append(res.SkippedFiles, SkippedFileInfo{
						File:   rel,
						Reason: "file exceeds max size limit",
					})
					return nil
				}
				contentBytes, readErr := os.ReadFile(path)
				if readErr == nil {
					discoveredFiles[rel] = string(contentBytes)
				}
			}
			return nil
		})
	}

	res.TotalScanned = len(discoveredFiles)
	if len(discoveredFiles) == 0 {
		return res, nil
	}

	// 2. Parse candidate files
	var totalBytes int64
	parsedCount := 0
	var candidateRules []interfaces.DrixyRule

	for relPath, content := range discoveredFiles {
		if parsedCount >= maxFiles {
			res.SkippedFiles = append(res.SkippedFiles, SkippedFileInfo{
				File:   relPath,
				Reason: "max files cap reached",
			})
			continue
		}

		size := int64(len(content))
		if totalBytes+size > maxTotalBytes {
			res.SkippedFiles = append(res.SkippedFiles, SkippedFileInfo{
				File:   relPath,
				Reason: "max aggregate size reached",
			})
			continue
		}
		totalBytes += size
		parsedCount++

		rulesFromFile := s.parseRuleFile(ctx, relPath, content, params.RepositoryID)
		candidateRules = append(candidateRules, rulesFromFile...)
	}

	// 3. Persist rules into organization
	if len(candidateRules) > 0 {
		if err := s.ImportRulesIntoOrganization(ctx, params.OrganizationID, params.TeamID, candidateRules); err != nil {
			res.Errors = append(res.Errors, err.Error())
			return res, err
		}
	}

	res.Rules = candidateRules
	return res, nil
}

func (s *DrixyRulesSyncService) parseRuleFile(ctx context.Context, relPath, content, repositoryID string) []interfaces.DrixyRule {
	pinned := HasPinnedSyncMarker(content)
	now := time.Now().UTC()

	// 1. If it is a template file (.drixy/rules/** or rules/**/*.md), parse verbatim
	if utils.IsDrixyRuleTemplateFile(relPath) {
		parsed := utils.ParseDrixyRuleFile(content)
		if parsed != nil {
			if !parsed.Enabled {
				return nil
			}
			scopedPath := utils.ValidateAndScopeIdeRulePath(parsed.Path, relPath)
			sev := parsed.Severity
			if sev == "" {
				sev = "medium"
			}
			sc := interfaces.DrixyRulesScopeFile
			if strings.EqualFold(parsed.Scope, "pull-request") || strings.EqualFold(parsed.Scope, "pull_request") {
				sc = interfaces.DrixyRulesScopePullRequest
			}

			ruleID := parsed.UUID
			if ruleID == "" {
				ruleID = uuid.New().String()
			}

			rule := interfaces.DrixyRule{
				UUID:         ruleID,
				Title:        parsed.Title,
				Rule:         parsed.Rule,
				Path:         scopedPath,
				SourcePath:   relPath,
				RepositoryID: repositoryID,
				Status:       interfaces.DrixyRulesStatusActive,
				Severity:     sev,
				Scope:        sc,
				Origin:       interfaces.DrixyRulesOriginRepoFileSync,
				PinnedSync:   pinned,
				CreatedAt:    &now,
				UpdatedAt:    &now,
				Examples:     make([]interfaces.DrixyRulesExample, 0),
			}

			for _, ex := range parsed.Examples {
				rule.Examples = append(rule.Examples, interfaces.DrixyRulesExample{
					Snippet:   ex.Snippet,
					IsCorrect: ex.IsCorrect,
				})
			}

			return []interfaces.DrixyRule{rule}
		}
	}

	// 2. Free-form sources (CLAUDE.md, .cursorrules, .windsurfrules, etc.) -> AI extraction
	if s.llmGateway != nil && len(strings.TrimSpace(content)) > 50 {
		aiRules := s.extractRulesViaLLM(ctx, relPath, content, repositoryID, pinned)
		if len(aiRules) > 0 {
			return aiRules
		}
	}

	// 3. Fallback: single rule from whole file if substantive
	if len(strings.TrimSpace(content)) > 100 {
		rule := interfaces.DrixyRule{
			UUID:         uuid.New().String(),
			Title:        fmt.Sprintf("Standards from %s", filepath.Base(relPath)),
			Rule:         strings.TrimSpace(content),
			Path:         utils.ValidateAndScopeIdeRulePath("", relPath),
			SourcePath:   relPath,
			RepositoryID: repositoryID,
			Status:       interfaces.DrixyRulesStatusActive,
			Severity:     "medium",
			Scope:        interfaces.DrixyRulesScopeFile,
			Origin:       interfaces.DrixyRulesOriginRepoFileSync,
			PinnedSync:   pinned,
			CreatedAt:    &now,
			UpdatedAt:    &now,
			Examples:     make([]interfaces.DrixyRulesExample, 0),
		}
		return []interfaces.DrixyRule{rule}
	}

	return nil
}

func (s *DrixyRulesSyncService) extractRulesViaLLM(ctx context.Context, relPath, content, repositoryID string, pinned bool) []interfaces.DrixyRule {
	userPrompt := fmt.Sprintf("File Path: %s\n\nContent:\n%s", relPath, content)
	resp, err := s.llmGateway.GenerateChatResponse(ctx, prompts.DrixyRulesIDEGeneratorSystem(), nil, userPrompt)
	if err != nil {
		return nil
	}

	resp = strings.TrimSpace(resp)
	if idx := strings.Index(resp, "{"); idx >= 0 {
		if endIdx := strings.LastIndex(resp, "}"); endIdx > idx {
			resp = resp[idx : endIdx+1]
		}
	}

	var parsedOutput prompts.RuleGeneratorOutput
	if err := json.Unmarshal([]byte(resp), &parsedOutput); err != nil {
		return nil
	}

	now := time.Now().UTC()
	var extracted []interfaces.DrixyRule
	for _, item := range parsedOutput.Rules {
		if strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.Rule) == "" {
			continue
		}
		scopedPath := utils.ValidateAndScopeIdeRulePath(item.Scope, relPath)
		rule := interfaces.DrixyRule{
			UUID:         uuid.New().String(),
			Title:        item.Title,
			Rule:         item.Rule,
			Path:         scopedPath,
			SourcePath:   relPath,
			RepositoryID: repositoryID,
			Status:       interfaces.DrixyRulesStatusActive,
			Severity:     item.Severity,
			Scope:        interfaces.DrixyRulesScopeFile,
			Origin:       interfaces.DrixyRulesOriginRepoFileSync,
			PinnedSync:   pinned,
			CreatedAt:    &now,
			UpdatedAt:    &now,
			Examples:     item.Examples,
		}
		extracted = append(extracted, rule)
	}
	return extracted
}

// SyncFromChangedFiles handles incremental rule file changes from merged PRs or pushed commits.
func (s *DrixyRulesSyncService) SyncFromChangedFiles(
	ctx context.Context,
	organizationID, teamID, repositoryID string,
	files []ChangedFile,
) error {
	var toUpdate []interfaces.DrixyRule
	var toDeleteSourcePaths []string

	for _, f := range files {
		if !utils.IsIdeRuleSource(f.Filename) && !utils.IsIdeRuleSource(f.PreviousFilename) {
			continue
		}

		if f.Status == "removed" {
			toDeleteSourcePaths = append(toDeleteSourcePaths, f.Filename)
			continue
		}

		if f.Content != "" {
			parsed := s.parseRuleFile(ctx, f.Filename, f.Content, repositoryID)
			toUpdate = append(toUpdate, parsed...)
		}
	}

	if len(toDeleteSourcePaths) > 0 {
		entity, err := s.rulesService.FindByOrganizationID(ctx, organizationID)
		if err == nil && entity != nil {
			rules := entity.Rules()
			now := time.Now().UTC()
			for i := range rules {
				for _, delPath := range toDeleteSourcePaths {
					if rules[i].RepositoryID == repositoryID && rules[i].SourcePath == delPath {
						rules[i].Status = interfaces.DrixyRulesStatusDeleted
						rules[i].UpdatedAt = &now
					}
				}
			}
			updated := entities.NewDrixyRulesEntity(interfaces.DrixyRules{
				UUID:           entity.UUID(),
				OrganizationID: organizationID,
				Rules:          rules,
				CreatedAt:      entity.CreatedAt(),
				UpdatedAt:      &now,
			})
			_ = s.rulesService.Save(ctx, updated)
		}
	}

	if len(toUpdate) > 0 {
		return s.ImportRulesIntoOrganization(ctx, organizationID, teamID, toUpdate)
	}

	return nil
}

// PauseAllIdeSyncRulesForRepository bulk pauses all IDE-synced rules for a repository.
func (s *DrixyRulesSyncService) PauseAllIdeSyncRulesForRepository(ctx context.Context, organizationID, teamID, repositoryID string) error {
	entity, err := s.rulesService.FindByOrganizationID(ctx, organizationID)
	if err != nil || entity == nil {
		return err
	}

	rules := entity.Rules()
	now := time.Now().UTC()
	for i := range rules {
		if rules[i].RepositoryID == repositoryID && rules[i].Origin == interfaces.DrixyRulesOriginRepoFileSync {
			rules[i].Status = interfaces.DrixyRulesStatusPaused
			rules[i].UpdatedAt = &now
		}
	}

	updated := entities.NewDrixyRulesEntity(interfaces.DrixyRules{
		UUID:           entity.UUID(),
		OrganizationID: organizationID,
		Rules:          rules,
		CreatedAt:      entity.CreatedAt(),
		UpdatedAt:      &now,
	})
	return s.rulesService.Save(ctx, updated)
}

// ResumeAllIdeSyncRulesForRepository bulk reactivates all paused IDE-synced rules.
func (s *DrixyRulesSyncService) ResumeAllIdeSyncRulesForRepository(ctx context.Context, organizationID, teamID, repositoryID string) error {
	entity, err := s.rulesService.FindByOrganizationID(ctx, organizationID)
	if err != nil || entity == nil {
		return err
	}

	rules := entity.Rules()
	now := time.Now().UTC()
	for i := range rules {
		if rules[i].RepositoryID == repositoryID && rules[i].Origin == interfaces.DrixyRulesOriginRepoFileSync {
			rules[i].Status = interfaces.DrixyRulesStatusActive
			rules[i].LockedByPlan = false
			rules[i].UpdatedAt = &now
		}
	}

	updated := entities.NewDrixyRulesEntity(interfaces.DrixyRules{
		UUID:           entity.UUID(),
		OrganizationID: organizationID,
		Rules:          rules,
		CreatedAt:      entity.CreatedAt(),
		UpdatedAt:      &now,
	})
	return s.rulesService.Save(ctx, updated)
}

// PurgeAllIdeSyncRulesForRepository removes all IDE-synced rules for a repository.
func (s *DrixyRulesSyncService) PurgeAllIdeSyncRulesForRepository(ctx context.Context, organizationID, teamID, repositoryID string) error {
	entity, err := s.rulesService.FindByOrganizationID(ctx, organizationID)
	if err != nil || entity == nil {
		return err
	}

	var retained []interfaces.DrixyRule
	for _, r := range entity.Rules() {
		if !(r.RepositoryID == repositoryID && r.Origin == interfaces.DrixyRulesOriginRepoFileSync) {
			retained = append(retained, r)
		}
	}

	now := time.Now().UTC()
	updated := entities.NewDrixyRulesEntity(interfaces.DrixyRules{
		UUID:           entity.UUID(),
		OrganizationID: organizationID,
		Rules:          retained,
		CreatedAt:      entity.CreatedAt(),
		UpdatedAt:      &now,
	})
	return s.rulesService.Save(ctx, updated)
}

// CountIdeSyncRulesForRepository returns status breakdowns of IDE-synced rules.
func (s *DrixyRulesSyncService) CountIdeSyncRulesForRepository(ctx context.Context, organizationID, teamID, repositoryID string) (struct {
	Active  int
	Paused  int
	Deleted int
	Pinned  int
}, error) {
	counts := struct {
		Active  int
		Paused  int
		Deleted int
		Pinned  int
	}{}

	entity, err := s.rulesService.FindByOrganizationID(ctx, organizationID)
	if err != nil || entity == nil {
		return counts, nil
	}

	for _, r := range entity.Rules() {
		if r.RepositoryID == repositoryID && r.Origin == interfaces.DrixyRulesOriginRepoFileSync {
			switch r.Status {
			case interfaces.DrixyRulesStatusActive:
				counts.Active++
			case interfaces.DrixyRulesStatusPaused:
				counts.Paused++
			case interfaces.DrixyRulesStatusDeleted:
				counts.Deleted++
			}
			if r.PinnedSync {
				counts.Pinned++
			}
		}
	}
	return counts, nil
}

// CountGlobalSyncedRules computes currently active global-synced rules across the org.
func (s *DrixyRulesSyncService) CountGlobalSyncedRules(ctx context.Context, organizationID, teamID string) (int, error) {
	entity, err := s.rulesService.FindByOrganizationID(ctx, organizationID)
	if err != nil || entity == nil {
		return 0, nil
	}

	count := 0
	for _, r := range entity.Rules() {
		if r.RepositoryID == "global" && r.Origin == interfaces.DrixyRulesOriginGlobalRepoFileSync && r.Status == interfaces.DrixyRulesStatusActive {
			count++
		}
	}
	return count, nil
}

// RemoveGlobalRulesFromSourceRepository soft-deletes global rules imported from a deselected source repo.
func (s *DrixyRulesSyncService) RemoveGlobalRulesFromSourceRepository(ctx context.Context, organizationID, teamID, sourceRepositoryID string) error {
	entity, err := s.rulesService.FindByOrganizationID(ctx, organizationID)
	if err != nil || entity == nil {
		return err
	}

	rules := entity.Rules()
	now := time.Now().UTC()
	for i := range rules {
		if rules[i].RepositoryID == "global" && rules[i].SourceRepositoryID == sourceRepositoryID && rules[i].Origin == interfaces.DrixyRulesOriginGlobalRepoFileSync {
			rules[i].Status = interfaces.DrixyRulesStatusDeleted
			rules[i].UpdatedAt = &now
		}
	}

	updated := entities.NewDrixyRulesEntity(interfaces.DrixyRules{
		UUID:           entity.UUID(),
		OrganizationID: organizationID,
		Rules:          rules,
		CreatedAt:      entity.CreatedAt(),
		UpdatedAt:      &now,
	})
	return s.rulesService.Save(ctx, updated)
}

// ImportRulesIntoOrganization registers parsed rules into the organization's rule aggregate.
func (s *DrixyRulesSyncService) ImportRulesIntoOrganization(
	ctx context.Context,
	organizationID string,
	teamID string,
	rules []interfaces.DrixyRule,
) error {
	entity, err := s.rulesService.FindByOrganizationID(ctx, organizationID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()

	var existingRules []interfaces.DrixyRule
	if entity != nil {
		existingRules = entity.Rules()
	}

	existingByKey := make(map[string]int)
	for idx, r := range existingRules {
		key := fmt.Sprintf("%s:%s:%s", r.RepositoryID, r.SourcePath, r.Title)
		existingByKey[key] = idx
	}

	for _, newRule := range rules {
		if newRule.UUID == "" {
			newRule.UUID = uuid.New().String()
		}
		newRule.CreatedAt = &now
		newRule.UpdatedAt = &now

		key := fmt.Sprintf("%s:%s:%s", newRule.RepositoryID, newRule.SourcePath, newRule.Title)
		if idx, found := existingByKey[key]; found {
			newRule.UUID = existingRules[idx].UUID
			existingRules[idx] = newRule
		} else {
			existingRules = append(existingRules, newRule)
		}
	}

	updated := entities.NewDrixyRulesEntity(interfaces.DrixyRules{
		UUID:           uuid.New().String(),
		OrganizationID: organizationID,
		Rules:          existingRules,
		UpdatedAt:      &now,
	})
	if entity != nil {
		updated = entities.NewDrixyRulesEntity(interfaces.DrixyRules{
			UUID:           entity.UUID(),
			OrganizationID: organizationID,
			Rules:          existingRules,
			CreatedAt:      entity.CreatedAt(),
			UpdatedAt:      &now,
		})
	}

	return s.rulesService.Save(ctx, updated)
}

// SyncRepositoryMain performs standard Git repository file sync into repository scope.
func (s *DrixyRulesSyncService) SyncRepositoryMain(ctx context.Context, params SyncRepositoryParams) error {
	_, err := s.SyncRepositoryMainFast(ctx, params)
	return err
}

// SyncRepositoryGlobal syncs rules from a source repository into global organization scope.
func (s *DrixyRulesSyncService) SyncRepositoryGlobal(ctx context.Context, params SyncRepositoryParams) error {
	fastParams := params
	fastParams.RepositoryID = "global"
	res, err := s.SyncRepositoryMainFast(ctx, fastParams)
	if err != nil {
		return err
	}

	// Stamp global origin
	now := time.Now().UTC()
	for i := range res.Rules {
		res.Rules[i].Origin = interfaces.DrixyRulesOriginGlobalRepoFileSync
		res.Rules[i].SourceRepositoryID = params.RepositoryID
		res.Rules[i].UpdatedAt = &now
	}
	return nil
}

// PurgeGlobalRulesForSourceRepository soft deletes global rules sourced from a repository.
func (s *DrixyRulesSyncService) PurgeGlobalRulesForSourceRepository(ctx context.Context, organizationID, teamID, sourceRepositoryID string) error {
	return s.RemoveGlobalRulesFromSourceRepository(ctx, organizationID, teamID, sourceRepositoryID)
}
