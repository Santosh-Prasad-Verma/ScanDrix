package verifier

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

// FindingVerdict represents the evaluation of a candidate code finding.
type FindingVerdict struct {
	FindingID  uuid.UUID `json:"finding_id"`
	Keep       bool      `json:"keep"`
	Rationale  string    `json:"rationale"`
	Confidence string    `json:"confidence"` // high, medium, low
}

// CandidateFinding represents an unverified finding from the Finder agent.
type CandidateFinding struct {
	ID          uuid.UUID `json:"id"`
	FilePath    string    `json:"file_path"`
	LineNumber  int       `json:"line_number"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Snippet     string    `json:"snippet"`
	Severity    string    `json:"severity"`
}

// SecondPassVerifier acts as an adversarial filter to eliminate false positives.
type SecondPassVerifier struct{}

func NewSecondPassVerifier() *SecondPassVerifier {
	return &SecondPassVerifier{}
}

// VerifyFinding independently assesses a candidate finding against false-positive patterns.
func (v *SecondPassVerifier) VerifyFinding(ctx context.Context, finding CandidateFinding, surroundingContext string) FindingVerdict {
	// Rule 1: Refute SQL Injection if parameterized or using ORM placeholder
	if strings.Contains(strings.ToLower(finding.Title), "sql injection") {
		if strings.Contains(finding.Snippet, "$1") ||
			strings.Contains(finding.Snippet, "?") ||
			strings.Contains(finding.Snippet, "ExecContext") && strings.Contains(finding.Snippet, ",") {
			return FindingVerdict{
				FindingID:  finding.ID,
				Keep:       false,
				Rationale:  "Refuted: Query uses parameterized placeholders, preventing SQL injection.",
				Confidence: "high",
			}
		}
	}

	// Rule 2: Refute XSS if HTML escaped
	if strings.Contains(strings.ToLower(finding.Title), "xss") || strings.Contains(strings.ToLower(finding.Title), "cross-site scripting") {
		if strings.Contains(surroundingContext, "html.EscapeString") || strings.Contains(surroundingContext, "template.HTMLEscape") {
			return FindingVerdict{
				FindingID:  finding.ID,
				Keep:       false,
				Rationale:  "Refuted: Output is explicitly sanitized with HTML escape encoders.",
				Confidence: "high",
			}
		}
	}

	// Rule 3: Refute Path Traversal if filepath.Clean and strings.HasPrefix are present
	if strings.Contains(strings.ToLower(finding.Title), "path traversal") {
		if strings.Contains(surroundingContext, "filepath.Clean") && strings.Contains(surroundingContext, "HasPrefix") {
			return FindingVerdict{
				FindingID:  finding.ID,
				Keep:       false,
				Rationale:  "Refuted: Base directory containment check is enforced via filepath.Clean and prefix matching.",
				Confidence: "high",
			}
		}
	}

	// Unrefuted genuine finding -> keep
	conf := "medium"
	if finding.Severity == "critical" || finding.Severity == "high" {
		conf = "high"
	}

	return FindingVerdict{
		FindingID:  finding.ID,
		Keep:       true,
		Rationale:  "Confirmed: Vulnerability or code defect cannot be refuted with surrounding context.",
		Confidence: conf,
	}
}

// FilterCandidates applies second-pass verification to a batch of candidate findings.
func (v *SecondPassVerifier) FilterCandidates(ctx context.Context, candidates []CandidateFinding, fileContexts map[string]string) []CandidateFinding {
	var verified []CandidateFinding

	for _, cand := range candidates {
		contextText := fileContexts[cand.FilePath]
		verdict := v.VerifyFinding(ctx, cand, contextText)
		if verdict.Keep {
			verified = append(verified, cand)
		}
	}

	return verified
}
