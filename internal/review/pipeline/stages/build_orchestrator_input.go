// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package stages

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/orchestrator"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/internal/review/priority"
)

// OrchestratorInputComputed captures stage-computed locals passed into the orchestrator.
type OrchestratorInputComputed struct {
	ChangedFiles    []orchestrator.ChangedFile
	PRNumber        int
	RepositoryID    uuid.UUID
	WorkspaceID     uuid.UUID
	ReviewOptions   orchestrator.ReviewOptions
	DrixyRules      []orchestrator.DrixyRule
	ExternalContext string
	CallGraph       string
	AdaptiveProfile priority.AdaptiveProfile
	ParentWarnings  []orchestrator.ReviewWarning
}

// BuildOrchestratorInput maps pipeline context and stage-computed locals into typed orchestrator input.
func BuildOrchestratorInput(
	pCtx *pipeline.PipelineContext,
	computed OrchestratorInputComputed,
) orchestrator.ReviewAgentInput {
	if pCtx == nil {
		return orchestrator.ReviewAgentInput{
			ChangedFiles:  computed.ChangedFiles,
			PRNumber:      computed.PRNumber,
			ReviewOptions: computed.ReviewOptions,
		}
	}

	prNum := computed.PRNumber
	if prNum <= 0 {
		prNum = pCtx.PullNumber
	}

	repoID := computed.RepositoryID
	if repoID == uuid.Nil {
		repoID = pCtx.RepositoryID
	}

	workspaceID := computed.WorkspaceID
	if workspaceID == uuid.Nil {
		workspaceID = pCtx.WorkspaceID
	}

	// 1. Files resolution: prefer computed changed files, fall back to pCtx.ChangedFiles or pCtx.Files
	changedFiles := computed.ChangedFiles
	if len(changedFiles) == 0 {
		srcFiles := pCtx.ChangedFiles
		if len(srcFiles) == 0 {
			srcFiles = pCtx.Files
		}
		if len(srcFiles) > 0 {
			changedFiles = make([]orchestrator.ChangedFile, 0, len(srcFiles))
			for _, f := range srcFiles {
				changedFiles = append(changedFiles, orchestrator.ChangedFile{
					Filename:        f.Filename,
					OldFilename:     f.OldPath,
					Status:          f.Status,
					Additions:       f.Additions,
					Deletions:       f.Deletions,
					Patch:           f.Patch,
					IsBinary:        false,
					IsVendored:      false,
				})
			}
		}
	}

	// 2. Rules resolution: prefer stage-computed rules (e.g. summary-swapped), fall back to context rules
	rules := computed.DrixyRules
	if len(rules) == 0 && len(pCtx.DrixyRules) > 0 {
		rules = make([]orchestrator.DrixyRule, 0, len(pCtx.DrixyRules))
		for _, r := range pCtx.DrixyRules {
			rules = append(rules, orchestrator.DrixyRule{
				ID:          r.ID,
				OrgID:       r.OrgID,
				RepoID:      r.RepoID,
				Name:        r.Name,
				Description: r.Description,
				Prompt:      r.Prompt,
				Severity:    r.Severity,
				Scope:       r.Scope,
				PathGlobs:   r.PathGlobs,
				IsActive:    r.IsActive,
			})
		}
	}

	// 3. Synthesize external architectural & steering context
	var extContext strings.Builder
	if computed.ExternalContext != "" {
		extContext.WriteString(computed.ExternalContext)
		extContext.WriteString("\n\n")
	}

	// Forward steering directive from review comments (e.g. "@scandrix review auth only")
	if pCtx.ReviewDirective != "" {
		extContext.WriteString(fmt.Sprintf("### Review Steering Directive:\n%s\n\n", pCtx.ReviewDirective))
	}

	// Forward architectural decision records (ADRs / Trace Context)
	if len(pCtx.TraceDecisions) > 0 {
		extContext.WriteString("### Recorded Architectural Decisions (Trace Context):\n")
		for _, td := range pCtx.TraceDecisions {
			extContext.WriteString(fmt.Sprintf("- **[%s] %s**: %s\n  why: %s (scope: %s)\n",
				td.DecisionKey, td.Title, td.Summary, td.Rationale, strings.Join(td.Files, ", ")))
		}
		extContext.WriteString("\n")
	}

	// Forward call graph summary if present and not dropped by adaptive profile
	if computed.CallGraph != "" && !computed.AdaptiveProfile.DropCallGraph {
		extContext.WriteString("### Call Graph Topology Context:\n")
		extContext.WriteString(computed.CallGraph)
		extContext.WriteString("\n\n")
	}

	// 4. Review options configuration
	options := computed.ReviewOptions
	if options.MaxTokens <= 0 {
		options = orchestrator.DefaultReviewOptions()
	}
	if pCtx.ResolvedConfig.ReviewMode != "" {
		switch strings.ToLower(pCtx.ResolvedConfig.ReviewMode) {
		case "fast":
			options.ReviewMode = orchestrator.ReviewModeFast
		case "deep":
			options.ReviewMode = orchestrator.ReviewModeDeep
		default:
			options.ReviewMode = orchestrator.ReviewModeNormal
		}
	}

	return orchestrator.ReviewAgentInput{
		PRNumber:        prNum,
		Title:           pCtx.Title,
		Description:     pCtx.Description,
		RepositoryName:  pCtx.RepoNamespace,
		BaseSHA:         pCtx.BaseSHA,
		HeadSHA:         pCtx.HeadSHA,
		BaseBranch:      pCtx.BaseBranch,
		HeadBranch:      pCtx.Branch,
		AuthorUsername:  pCtx.Author,
		ChangedFiles:    changedFiles,
		ReviewOptions:   options,
		DrixyRules:      rules,
		WorkspaceID:     workspaceID,
		RepositoryID:    repoID,
		ExternalContext: strings.TrimSpace(extContext.String()),
		ParentWarnings:  computed.ParentWarnings,
	}
}
