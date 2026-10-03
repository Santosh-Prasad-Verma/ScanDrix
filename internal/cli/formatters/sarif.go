package formatters

import (
	"encoding/json"
	"io"

	"github.com/scandrix/backend/internal/cli/types"
)

// SarifLog represents the root of a SARIF v2.1.0 document.
type SarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []SarifRun `json:"runs"`
}

// SarifRun represents a single analysis run.
type SarifRun struct {
	Tool        SarifTool         `json:"tool"`
	Results     []SarifResult     `json:"results"`
	Invocations []SarifInvocation `json:"invocations,omitempty"`
}

// SarifTool describes the analysis tool.
type SarifTool struct {
	Driver SarifDriver `json:"driver"`
}

// SarifDriver contains tool metadata and rule definitions.
type SarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version"`
	InformationURI string      `json:"informationUri"`
	Rules          []SarifRule `json:"rules"`
}

// SarifRule defines a static analysis rule.
type SarifRule struct {
	ID               string              `json:"id"`
	Name             string              `json:"name,omitempty"`
	ShortDescription SarifMultiformatMsg `json:"shortDescription"`
	FullDescription  SarifMultiformatMsg `json:"fullDescription,omitempty"`
	DefaultConfig    SarifDefaultConfig  `json:"defaultConfiguration"`
	HelpURI          string              `json:"helpUri,omitempty"`
}

// SarifDefaultConfig defines default level for a rule.
type SarifDefaultConfig struct {
	Level string `json:"level"` // error, warning, note, none
}

// SarifResult represents a single finding instance.
type SarifResult struct {
	RuleID    string              `json:"ruleId"`
	RuleIndex int                 `json:"ruleIndex"`
	Level     string              `json:"level"`
	Message   SarifMultiformatMsg `json:"message"`
	Locations []SarifLocation     `json:"locations"`
	Fixes     []SarifFix          `json:"fixes,omitempty"`
}

// SarifMultiformatMsg holds text message.
type SarifMultiformatMsg struct {
	Text string `json:"text"`
}

// SarifLocation points to the source code coordinates.
type SarifLocation struct {
	PhysicalLocation SarifPhysicalLocation `json:"physicalLocation"`
}

// SarifPhysicalLocation holds artifact location and region.
type SarifPhysicalLocation struct {
	ArtifactLocation SarifArtifactLocation `json:"artifactLocation"`
	Region           SarifRegion           `json:"region"`
}

// SarifArtifactLocation holds URI path.
type SarifArtifactLocation struct {
	URI string `json:"uri"`
}

// SarifRegion specifies line and column coordinates.
type SarifRegion struct {
	StartLine   int `json:"startLine"`
	EndLine     int `json:"endLine,omitempty"`
	StartColumn int `json:"startColumn,omitempty"`
	EndColumn   int `json:"endColumn,omitempty"`
}

// SarifFix represents an automated replacement fix.
type SarifFix struct {
	Description     SarifMultiformatMsg   `json:"description"`
	ArtifactChanges []SarifArtifactChange `json:"artifactChanges"`
}

// SarifArtifactChange defines changes to a specific file.
type SarifArtifactChange struct {
	ArtifactLocation SarifArtifactLocation `json:"artifactLocation"`
	Replacements     []SarifReplacement    `json:"replacements"`
}

// SarifReplacement specifies replacement content.
type SarifReplacement struct {
	DeletedRegion   SarifRegion  `json:"deletedRegion"`
	InsertedContent SarifContent `json:"insertedContent"`
}

// SarifContent holds replacement text.
type SarifContent struct {
	Text string `json:"text"`
}

// SarifInvocation holds runtime invocation metadata.
type SarifInvocation struct {
	ExecutionSuccessful bool   `json:"executionSuccessful"`
	CommandLine         string `json:"commandLine,omitempty"`
}

// SarifFormatter outputs findings in SARIF v2.1.0 standard JSON.
type SarifFormatter struct{}

// NewSarifFormatter creates a new SarifFormatter.
func NewSarifFormatter() *SarifFormatter {
	return &SarifFormatter{}
}

func (s *SarifFormatter) mapSeverityToLevel(sev types.Severity) string {
	switch sev {
	case types.SeverityCritical, types.SeverityError:
		return "error"
	case types.SeverityWarning:
		return "warning"
	case types.SeverityInfo:
		return "note"
	default:
		return "warning"
	}
}

// Format transforms ReviewResult into standard SARIF JSON.
func (s *SarifFormatter) Format(w io.Writer, result *types.ReviewResult) error {
	rulesMap := make(map[string]int)
	var sarifRules []SarifRule
	var sarifResults []SarifResult

	for _, issue := range result.Issues {
		ruleID := issue.RuleID
		if ruleID == "" {
			ruleID = "scandrix/" + issue.Category
			if issue.Category == "" {
				ruleID = "scandrix/general-issue"
			}
		}

		ruleIdx, exists := rulesMap[ruleID]
		if !exists {
			ruleIdx = len(sarifRules)
			rulesMap[ruleID] = ruleIdx
			sarifRules = append(sarifRules, SarifRule{
				ID:   ruleID,
				Name: ruleID,
				ShortDescription: SarifMultiformatMsg{
					Text: issue.Message,
				},
				DefaultConfig: SarifDefaultConfig{
					Level: s.mapSeverityToLevel(issue.Severity),
				},
				// No per-rule documentation page exists, so there is no valid
				// HelpURI to give. It used to be built by concatenation and
				// 404'd for every rule. The field is omitempty, so leaving it
				// empty omits it rather than publishing a dead link.
				// AUDIT_REMEDIATION.md F-45.
				HelpURI: "",
			})
		}

		startLine := issue.Line
		if startLine <= 0 {
			startLine = 1
		}
		endLine := issue.EndLine
		if endLine < startLine {
			endLine = startLine
		}

		res := SarifResult{
			RuleID:    ruleID,
			RuleIndex: ruleIdx,
			Level:     s.mapSeverityToLevel(issue.Severity),
			Message: SarifMultiformatMsg{
				Text: issue.Message,
			},
			Locations: []SarifLocation{
				{
					PhysicalLocation: SarifPhysicalLocation{
						ArtifactLocation: SarifArtifactLocation{
							URI: issue.File,
						},
						Region: SarifRegion{
							StartLine:   startLine,
							EndLine:     endLine,
							StartColumn: issue.Column,
							EndColumn:   issue.EndColumn,
						},
					},
				},
			},
		}

		// Attach fix if present
		if issue.Fixable && issue.Fix != nil {
			res.Fixes = []SarifFix{
				{
					Description: SarifMultiformatMsg{
						Text: issue.Fix.Explanation,
					},
					ArtifactChanges: []SarifArtifactChange{
						{
							ArtifactLocation: SarifArtifactLocation{
								URI: issue.File,
							},
							Replacements: []SarifReplacement{
								{
									DeletedRegion: SarifRegion{
										StartLine: issue.Fix.StartLine,
										EndLine:   issue.Fix.EndLine,
									},
									InsertedContent: SarifContent{
										Text: issue.Fix.NewCode,
									},
								},
							},
						},
					},
				},
			}
		}

		sarifResults = append(sarifResults, res)
	}

	log := SarifLog{
		Schema:  "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json",
		Version: "2.1.0",
		Runs: []SarifRun{
			{
				Tool: SarifTool{
					Driver: SarifDriver{
						Name:           "ScanDrix",
						Version:        "1.0.0",
						InformationURI: "https://scandrix.dev",
						Rules:          sarifRules,
					},
				},
				Results: sarifResults,
				Invocations: []SarifInvocation{
					{
						ExecutionSuccessful: true,
					},
				},
			},
		},
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(log)
}
