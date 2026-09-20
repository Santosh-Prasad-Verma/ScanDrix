package stages

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/pipeline"
)

// ConfigResolver abstracts retrieving stored review configuration and PR messages.
type ConfigResolver interface {
	GetCodeReviewParameter(ctx context.Context, orgID, teamID, repoID string) (domain.CodeReviewConfig, error)
	FindByRepoOrDirectory(ctx context.Context, orgID, repoID, dirID string) (*domain.PullRequestMessages, error)
}

// ResolveConfigStage (Stage 3) resolves repository configuration and custom templates.
type ResolveConfigStage struct {
	resolver ConfigResolver
}

// NewResolveConfigStage constructs Stage 3.
func NewResolveConfigStage(resolver ConfigResolver) *ResolveConfigStage {
	return &ResolveConfigStage{resolver: resolver}
}

func (s *ResolveConfigStage) Name() string {
	return "ResolveConfigStage"
}

func (s *ResolveConfigStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	if pCtx.SkipReview {
		return nil
	}

	orgID := pCtx.WorkspaceID.String()
	repoID := pCtx.RepositoryID.String()

	// 1. Resolve configuration through hierarchy
	cfg := domain.DefaultCodeReviewConfig()
	if s.resolver != nil {
		resolved, err := s.resolver.GetCodeReviewParameter(ctx, orgID, "", repoID)
		if err == nil {
			cfg = resolved
		}
	}

	// 2. Check for committed in-repo `.scandrix/config.json` in patches
	for _, patch := range pCtx.ParsedPatches {
		if strings.HasSuffix(patch.NewPath, ".scandrix/config.json") || strings.HasSuffix(patch.NewPath, ".scandrix/config.yaml") {
			// Extract file content if provided in hunk or file change
			for _, h := range patch.Hunks {
				var sb strings.Builder
				for _, l := range h.Lines {
					if l.Type != 2 { // Not deleted
						sb.WriteString(l.Content + "\n")
					}
				}
				var fileCfg domain.CodeReviewConfig
				if err := json.Unmarshal([]byte(sb.String()), &fileCfg); err == nil {
					cfg = fileCfg
				}
			}
		}
	}

	pCtx.ResolvedConfig = cfg

	// 3. Resolve custom PR messages
	if s.resolver != nil {
		msgs, err := s.resolver.FindByRepoOrDirectory(ctx, orgID, repoID, "")
		if err == nil && msgs != nil {
			pCtx.PullRequestMessages = msgs
		}
	}

	return nil
}
