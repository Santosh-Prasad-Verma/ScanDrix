package classification

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// PRCategory classifies pull requests for analytics and risk profiling.
type PRCategory string

const (
	CategoryFeature  PRCategory = "feature"
	CategoryBugFix   PRCategory = "bug_fix"
	CategorySecurity PRCategory = "security"
	CategoryRefactor PRCategory = "refactor"
	CategoryTest     PRCategory = "test"
	CategoryChore    PRCategory = "chore"
	CategoryUnknown  PRCategory = "unknown"
)

var (
	featRegex     = regexp.MustCompile(`(?i)^(feat|feature)(\(.*\))?:\s*`)
	fixRegex      = regexp.MustCompile(`(?i)^(fix|bugfix|patch)(\(.*\))?:\s*`)
	secRegex      = regexp.MustCompile(`(?i)^(sec|security|cve)(\(.*\))?:\s*`)
	refactorRegex = regexp.MustCompile(`(?i)^(refactor|clean|perf)(\(.*\))?:\s*`)
	testRegex     = regexp.MustCompile(`(?i)^(test|tests|spec)(\(.*\))?:\s*`)
	choreRegex    = regexp.MustCompile(`(?i)^(chore|docs|ci|build)(\(.*\))?:\s*`)
)

// ClassifiedPR records the categorized pull request metadata.
type ClassifiedPR struct {
	PRID           uuid.UUID  `json:"pr_id"`
	WorkspaceID    uuid.UUID  `json:"workspace_id"`
	PRNumber       int        `json:"pr_number"`
	Title          string     `json:"title"`
	Category       PRCategory `json:"category"`
	Confidence     float64    `json:"confidence"`
	ClassifiedAt   time.Time  `json:"classified_at"`
}

// PRClassifier assigns semantic categories to PRs for DORA metrics and bug ratio analysis.
type PRClassifier struct{}

func NewPRClassifier() *PRClassifier {
	return &PRClassifier{}
}

// ClassifyPR categorizes a pull request based on conventional commit prefixes, keywords, and title patterns.
func (c *PRClassifier) ClassifyPR(prID, workspaceID uuid.UUID, prNumber int, title string) ClassifiedPR {
	trimmed := strings.TrimSpace(title)
	lower := strings.ToLower(trimmed)

	cat := CategoryUnknown
	confidence := 0.5

	switch {
	case secRegex.MatchString(lower):
		cat = CategorySecurity
		confidence = 0.98
	case fixRegex.MatchString(lower):
		cat = CategoryBugFix
		confidence = 0.95
	case testRegex.MatchString(lower):
		cat = CategoryTest
		confidence = 0.95
	case choreRegex.MatchString(lower):
		cat = CategoryChore
		confidence = 0.90
	case refactorRegex.MatchString(lower):
		cat = CategoryRefactor
		confidence = 0.90
	case featRegex.MatchString(lower):
		cat = CategoryFeature
		confidence = 0.92
	case strings.Contains(lower, "cve-") || strings.Contains(lower, "vulnerability"):
		cat = CategorySecurity
		confidence = 0.90
	case strings.Contains(lower, "fix") || strings.Contains(lower, "resolve"):
		cat = CategoryBugFix
		confidence = 0.85
	case strings.Contains(lower, "add") || strings.Contains(lower, "support"):
		cat = CategoryFeature
		confidence = 0.85
	default:
		cat = CategoryFeature // Default engineering assumption
		confidence = 0.60
	}

	return ClassifiedPR{
		PRID:         prID,
		WorkspaceID:  workspaceID,
		PRNumber:     prNumber,
		Title:        title,
		Category:     cat,
		Confidence:   confidence,
		ClassifiedAt: time.Now().UTC(),
	}
}
