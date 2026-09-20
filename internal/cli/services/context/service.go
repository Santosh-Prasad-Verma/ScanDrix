// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package context

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scandrix/backend/internal/cli/services/git"
	"github.com/scandrix/backend/internal/cli/utils"
)

// ProjectContext aggregates relevant rules and guidelines from repository root.
type ProjectContext struct {
	CursorRules   string
	ClaudeRules   string
	ScanDrixRules string
	CustomContext string
}

// Service discovers and extracts context files.
type Service struct {
	gitService *git.Service
}

var defaultContextService = &Service{gitService: git.DefaultService()}

// DefaultService returns the default ContextService.
func DefaultService() *Service {
	return defaultContextService
}

// ReadProjectContext looks for .cursorrules, claude.md, .scandrix.md, and custom context.
func (s *Service) ReadProjectContext(ctx context.Context, customContextPath string) (*ProjectContext, error) {
	root, err := s.gitService.GetGitRoot(ctx)
	if err != nil {
		root = "."
	}

	pCtx := &ProjectContext{}

	// 1. Read .cursorrules
	if content, err := os.ReadFile(filepath.Join(root, ".cursorrules")); err == nil {
		pCtx.CursorRules = strings.TrimSpace(string(content))
	}

	// 2. Read claude.md or .claude.md
	if content, err := os.ReadFile(filepath.Join(root, "claude.md")); err == nil {
		pCtx.ClaudeRules = strings.TrimSpace(string(content))
	} else if content, err := os.ReadFile(filepath.Join(root, ".claude.md")); err == nil {
		pCtx.ClaudeRules = strings.TrimSpace(string(content))
	}

	// 3. Read .scandrix.md or .scandrix/rules.md or .drixy/rules.yaml
	if content, err := os.ReadFile(filepath.Join(root, ".scandrix.md")); err == nil {
		pCtx.ScanDrixRules = strings.TrimSpace(string(content))
	} else if content, err := os.ReadFile(filepath.Join(root, ".scandrix", "rules.md")); err == nil {
		pCtx.ScanDrixRules = strings.TrimSpace(string(content))
	} else if content, err := os.ReadFile(filepath.Join(root, ".drixy", "rules.yaml")); err == nil {
		pCtx.ScanDrixRules = strings.TrimSpace(string(content))
	}

	// 4. Custom context file
	if customContextPath != "" {
		targetPath := customContextPath
		if !filepath.IsAbs(targetPath) {
			targetPath = filepath.Join(root, targetPath)
		}
		if content, err := os.ReadFile(targetPath); err == nil {
			pCtx.CustomContext = strings.TrimSpace(string(content))
		} else {
			utils.Warn("Specified context file %s could not be read: %v", customContextPath, err)
		}
	}

	return pCtx, nil
}

// FormatContext returns a formatted context header to enrich code reviews.
func (s *Service) FormatContext(pCtx *ProjectContext) string {
	if pCtx == nil {
		return ""
	}

	var parts []string
	if pCtx.CursorRules != "" {
		parts = append(parts, "=== Cursor Rules (.cursorrules) ===\n"+pCtx.CursorRules)
	}
	if pCtx.ClaudeRules != "" {
		parts = append(parts, "=== Claude Rules (claude.md) ===\n"+pCtx.ClaudeRules)
	}
	if pCtx.ScanDrixRules != "" {
		parts = append(parts, "=== ScanDrix / Drixy Rules ===\n"+pCtx.ScanDrixRules)
	}
	if pCtx.CustomContext != "" {
		parts = append(parts, "=== Custom Project Context ===\n"+pCtx.CustomContext)
	}

	if len(parts) == 0 {
		return ""
	}

	return fmt.Sprintf("\n# PROJECT CONTEXT & TEAM GUIDELINES\n\n%s\n\n", strings.Join(parts, "\n\n"))
}

// EnrichDiff prepends formatted context to the unified diff.
func (s *Service) EnrichDiff(ctx context.Context, diff string, customContextPath string) (string, error) {
	pCtx, err := s.ReadProjectContext(ctx, customContextPath)
	if err != nil {
		return diff, nil
	}

	contextHeader := s.FormatContext(pCtx)
	if contextHeader == "" {
		return diff, nil
	}

	return contextHeader + diff, nil
}
