package formatters

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/scandrix/backend/internal/cli/types"
)

// GitLabCodeQualityIssue models the standard GitLab CI code quality artifact schema.
type GitLabCodeQualityIssue struct {
	Description string                  `json:"description"`
	CheckName   string                  `json:"check_name"`
	Fingerprint string                  `json:"fingerprint"`
	Severity    string                  `json:"severity"` // info, minor, major, critical, blocker
	Location    GitLabCodeQualityLoc    `json:"location"`
}

// GitLabCodeQualityLoc points to the issue's file and line range.
type GitLabCodeQualityLoc struct {
	Path  string               `json:"path"`
	Lines GitLabCodeQualityLines `json:"lines"`
}

// GitLabCodeQualityLines defines start and optional end line numbers.
type GitLabCodeQualityLines struct {
	Begin int `json:"begin"`
	End   int `json:"end,omitempty"`
}

// GitLabCIFormatter converts types.ReviewResult into a GitLab Code Quality JSON artifact.
type GitLabCIFormatter struct{}

// NewGitLabCIFormatter instantiates a GitLab CI code quality formatter.
func NewGitLabCIFormatter() *GitLabCIFormatter {
	return &GitLabCIFormatter{}
}

// Format writes the GitLab Code Quality JSON report to w.
func (f *GitLabCIFormatter) Format(w io.Writer, result *types.ReviewResult) error {
	if result == nil {
		_, err := w.Write([]byte("[]\n"))
		return err
	}

	issues := make([]GitLabCodeQualityIssue, 0, len(result.Issues))
	for _, iss := range result.Issues {
		glSev := mapGitLabSeverity(iss.Severity)
		fp := computeFingerprint(iss)

		endLine := iss.EndLine
		if endLine <= 0 {
			endLine = iss.Line
		}

		checkName := iss.RuleID
		if checkName == "" {
			checkName = fmt.Sprintf("scandrix/%s", iss.Category)
		}

		issues = append(issues, GitLabCodeQualityIssue{
			Description: iss.Message,
			CheckName:   checkName,
			Fingerprint: fp,
			Severity:    glSev,
			Location: GitLabCodeQualityLoc{
				Path: iss.File,
				Lines: GitLabCodeQualityLines{
					Begin: iss.Line,
					End:   endLine,
				},
			},
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(issues)
}

func mapGitLabSeverity(sev types.Severity) string {
	switch types.Severity(strings.ToLower(string(sev))) {
	case types.SeverityCritical:
		return "blocker"
	case types.SeverityError:
		return "critical"
	case types.SeverityWarning:
		return "major"
	case types.SeverityInfo:
		return "info"
	default:
		return "minor"
	}
}

func computeFingerprint(iss types.ReviewIssue) string {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%s:%d:%s:%s", iss.File, iss.Line, iss.Category, iss.Message)))
	return hex.EncodeToString(h.Sum(nil))
}
