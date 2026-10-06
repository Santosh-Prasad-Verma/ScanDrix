package review

import (
	"context"
	"errors"
	"path"
	"strings"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

type repositorySettingsReader interface {
	GetRepositoryReviewSettings(context.Context, uuid.UUID, uuid.UUID) (models.RepositoryReviewSettings, error)
}

func repositoryPolicyReason(cfg models.RepositoryReviewSettings, task ExecutionTask) (string, error) {
	if !cfg.Active {
		return "monitoring paused", nil
	}
	if !cfg.AutoReviewEnabled && !task.Manual {
		return "automatic reviews disabled", nil
	}
	author := strings.ToLower(task.Author)
	if cfg.IgnoreBots {
		if author == "" {
			return "", errors.New("pull request author is required for repository policy")
		}
		if strings.HasSuffix(author, "[bot]") || author == "dependabot" || author == "renovate" {
			return "bot review excluded", nil
		}
	}
	if len(cfg.BranchesMonitored) > 0 {
		if task.BaseBranch == "" {
			return "", errors.New("target branch is required for repository policy")
		}
		for _, pattern := range cfg.BranchesMonitored {
			if matched, _ := path.Match(pattern, task.BaseBranch); matched {
				return "", nil
			}
			if strings.HasSuffix(pattern, "/**") && models.MatchesRepositoryPattern(task.BaseBranch, pattern) {
				return "", nil
			}
		}
		return "target branch excluded", nil
	}
	return "", nil
}

// Preserve the original bytes for every included file so ignored paths cannot
// reach static analysis or the LLM through the unfiltered raw diff.
func filterRepositoryDiff(raw string, patterns []string) string {
	if len(patterns) == 0 {
		return raw
	}
	blocks := strings.Split(strings.TrimPrefix(raw, "diff --git "), "\ndiff --git ")
	var result strings.Builder
	for index, block := range blocks {
		if index < len(blocks)-1 {
			block += "\n"
		}
		header := strings.SplitN(block, "\n", 2)[0]
		paths := strings.SplitN(header, " b/", 2)
		if len(paths) != 2 {
			continue
		}
		oldPath, newPath := strings.TrimPrefix(paths[0], "a/"), paths[1]
		// Header delimiters are ambiguous when a filename contains " b/".
		// File metadata before the first hunk identifies the actual paths.
		for _, line := range strings.Split(block, "\n")[1:] {
			if strings.HasPrefix(line, "@@") {
				break
			}
			switch {
			case strings.HasPrefix(line, "--- a/"):
				oldPath = strings.TrimSuffix(strings.TrimPrefix(line, "--- a/"), "\t")
			case strings.HasPrefix(line, "+++ b/"):
				newPath = strings.TrimSuffix(strings.TrimPrefix(line, "+++ b/"), "\t")
			case strings.HasPrefix(line, "rename from "):
				oldPath = strings.TrimPrefix(line, "rename from ")
			case strings.HasPrefix(line, "rename to "):
				newPath = strings.TrimPrefix(line, "rename to ")
			}
		}
		excluded := false
		for _, pattern := range patterns {
			if models.MatchesRepositoryPattern(oldPath, pattern) || models.MatchesRepositoryPattern(newPath, pattern) {
				excluded = true
				break
			}
		}
		if !excluded {
			result.WriteString("diff --git ")
			result.WriteString(block)
		}
	}
	return result.String()
}
