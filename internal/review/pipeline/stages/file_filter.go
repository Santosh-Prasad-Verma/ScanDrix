package stages

import (
	"context"
	"strings"

	"github.com/scandrix/backend/internal/codeanalysis"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/review/pipeline"
)

// FileFilterStage parses raw diffs and strips out generated files, lockfiles, and minified bundles.
type FileFilterStage struct{}

func NewFileFilterStage() *FileFilterStage {
	return &FileFilterStage{}
}

func (s *FileFilterStage) Name() string {
	return "file_filter"
}

func (s *FileFilterStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	patches, err := diff.ParseUnifiedDiff(strings.NewReader(pCtx.RawDiff))
	if err != nil {
		return err
	}
	pCtx.ParsedPatches = patches

	ignoreMatcher := codeanalysis.NewIgnoreMatcher(pCtx.ReviewParams.IgnorePatterns)

	filtered := make([]*diff.FilePatch, 0, len(patches))
	for _, f := range patches {
		// 1. Skip binary files
		if f.IsBinary {
			continue
		}

		// 2. Skip deleted files
		if f.IsDeleted {
			continue
		}

		// 3. Check ignore patterns (lockfiles, minified, vendor, custom)
		if ignoreMatcher.ShouldIgnore(f.NewPath) || ignoreMatcher.ShouldIgnore(f.OldPath) {
			continue
		}

		filtered = append(filtered, f)
	}

	pCtx.FilteredPatches = filtered
	return nil
}
