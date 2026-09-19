// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package skills

import (
	"os"

	"github.com/scandrix/backend/internal/cli/skills"
)

// SKILLS TARGET & SYNC RESULT MODELS

// Target describes a local agent directory where skills can be synchronized.
type Target = skills.TargetDirectory

// SyncResult details files created, updated, or removed during sync.
type SyncResult struct {
	Created        int      `json:"created"`
	Updated        int      `json:"updated"`
	Unchanged      int      `json:"unchanged"`
	Removed        int      `json:"removed"`
	RemovedManaged int      `json:"removed_managed"`
	RemovedLegacy  int      `json:"removed_legacy"`
	Targets        []string `json:"targets"`
}

// SKILLS SERVICE & FACTORY

// Service manages bundled AI assistant skills across agents.
type Service struct {
	workDir string
}

var defaultSkillsService = &Service{workDir: "."}

// DefaultService returns the default SkillsService.
func DefaultService() *Service {
	return defaultSkillsService
}

// Catalog returns all embedded skills.
func (s *Service) Catalog() []skills.BundledSkill {
	return skills.BundledSkillsCatalog()
}

// TARGET DETECTION & SYNCHRONIZATION

// DetectTargets locates existing or potential agent configurations.
func (s *Service) DetectTargets(workDir string, createIfMissing bool) []skills.TargetDirectory {
	home, _ := os.UserHomeDir()
	all := skills.BuildSkillSyncTargets(workDir, home)
	var active []skills.TargetDirectory
	for _, t := range all {
		if _, err := os.Stat(t.BaseDir); err == nil || createIfMissing {
			active = append(active, t)
		}
	}
	return active
}

// Sync writes all catalog skills to detected agent directories.
func (s *Service) Sync(workDir string, dryRun bool, installMode bool) (*SyncResult, error) {
	home, _ := os.UserHomeDir()
	res, err := skills.Sync(workDir, home, dryRun, installMode)
	if err != nil {
		return nil, err
	}

	return &SyncResult{
		Created:        res.CreatedCount,
		Updated:        res.UpdatedCount,
		Unchanged:      res.UnchangedCount,
		Removed:        res.RemovedCount,
		RemovedManaged: res.RemovedManaged,
		RemovedLegacy:  res.RemovedLegacy,
		Targets:        res.Targets,
	}, nil
}

// SKILLS UNINSTALLATION

// Uninstall removes managed skills from detected agent directories.
func (s *Service) Uninstall(workDir string, dryRun bool) (*SyncResult, error) {
	home, _ := os.UserHomeDir()
	res, err := skills.UninstallTargets(workDir, home, dryRun)
	if err != nil {
		return nil, err
	}

	return &SyncResult{
		Created:        res.CreatedCount,
		Updated:        res.UpdatedCount,
		Unchanged:      res.UnchangedCount,
		Removed:        res.RemovedCount,
		RemovedManaged: res.RemovedManaged,
		RemovedLegacy:  res.RemovedLegacy,
		Targets:        res.Targets,
	}, nil
}

// SKILLS AUDIT & CHECK

// Check audits installed skills in all agent directories.
func (s *Service) Check(workDir string) (*skills.SkillCheckReport, error) {
	home, _ := os.UserHomeDir()
	return skills.Check(workDir, home)
}

// FormatPrompt formats catalog skills into XML, JSON, or Markdown for LLM prompt context injection.
func (s *Service) FormatPrompt(format string, selected []string) (string, error) {
	return skills.FormatSkillsPrompt(s.Catalog(), format, selected)
}
