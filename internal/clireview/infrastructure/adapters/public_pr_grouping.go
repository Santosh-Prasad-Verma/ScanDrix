package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/scandrix/backend/internal/clireview/domain"
)

const maxGroupingDiffChars = 80000

// PublicPrGroupingService clusters changed files by architectural intent.
type PublicPrGroupingService struct {
	llmRunner func(ctx context.Context, prompt string) (string, error)
}

// NewPublicPrGroupingService creates a new grouping service.
func NewPublicPrGroupingService(runner func(ctx context.Context, prompt string) (string, error)) *PublicPrGroupingService {
	return &PublicPrGroupingService{
		llmRunner: runner,
	}
}

// Generate organizes changed files into intent-based clusters.
func (s *PublicPrGroupingService) Generate(
	ctx context.Context,
	pr *domain.PublicPrMetadata,
	diff string,
	changedFiles []string,
) ([]domain.PublicPrGrouping, error) {
	if len(changedFiles) < 2 {
		return nil, nil
	}

	if s.llmRunner != nil {
		truncatedDiff := diff
		if len(diff) > maxGroupingDiffChars {
			truncatedDiff = diff[:maxGroupingDiffChars]
		}
		prompt := s.buildPrompt(pr, truncatedDiff, changedFiles)
		if text, err := s.llmRunner(ctx, prompt); err == nil {
			var resp struct {
				Groups []domain.PublicPrGrouping `json:"groups"`
			}
			if err := json.Unmarshal([]byte(text), &resp); err == nil && len(resp.Groups) > 0 {
				return resp.Groups, nil
			}
		}
	}

	// Heuristic clustering by top-level directory or file extension
	return s.clusterByPath(changedFiles), nil
}

func (s *PublicPrGroupingService) clusterByPath(files []string) []domain.PublicPrGrouping {
	groupsMap := make(map[string][]string)
	for _, f := range files {
		clean := filepath.Clean(f)
		parts := strings.Split(clean, string(filepath.Separator))
		groupKey := "Core"
		if len(parts) > 1 {
			groupKey = parts[0]
		} else {
			ext := filepath.Ext(clean)
			if ext != "" {
				groupKey = strings.ToUpper(strings.TrimPrefix(ext, ".")) + " Files"
			}
		}
		groupsMap[groupKey] = append(groupsMap[groupKey], f)
	}

	var groupings []domain.PublicPrGrouping
	for title, flist := range groupsMap {
		groupings = append(groupings, domain.PublicPrGrouping{
			Title:       fmt.Sprintf("%s Changes", strings.Title(title)),
			Explanation: fmt.Sprintf("Modifications to %d file(s) in %s subsystem.", len(flist), title),
			Files:       flist,
		})
	}

	return groupings
}

func (s *PublicPrGroupingService) buildPrompt(pr *domain.PublicPrMetadata, diff string, files []string) string {
	return fmt.Sprintf(`Cluster these files changed in pull request "%s" into 1-8 logical groupings by developer intent:
Files:
%s

Diff snippet:
%s

Output JSON format:
{"groups": [{"title": "Short title", "explanation": "One sentence explanation", "files": ["path1", "path2"]}]}`,
		pr.Title,
		strings.Join(files, "\n"),
		diff,
	)
}
