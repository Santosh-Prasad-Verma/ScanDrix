package engine

import (
	"time"

	"github.com/scandrix/backend/pkg/models"
)

// OutputFormat defines the CLI report rendering target.
type OutputFormat string

const (
	FormatTable OutputFormat = "table"
	FormatJSON  OutputFormat = "json"
	FormatSARIF OutputFormat = "sarif"
)

// CLIOptions encapsulates flags passed to the CLI binary.
type CLIOptions struct {
	Staged            bool                   `json:"staged"`
	Branch            string                 `json:"branch"`
	CommitRange       string                 `json:"commit_range"`
	TargetDirectory   string                 `json:"target_directory"`
	Offline           bool                   `json:"offline"`
	DryRun            bool                   `json:"dry_run"`
	Format            OutputFormat           `json:"format"`
	SeverityThreshold models.FindingSeverity `json:"severity_threshold"`
	APIKey            string                 `json:"api_key,omitempty"`
	APIBaseURL        string                 `json:"api_base_url,omitempty"`
}

// CLIResult aggregates findings, counts, and exit code.
type CLIResult struct {
	FilesReviewed  int                  `json:"files_reviewed"`
	TotalFindings  int                  `json:"total_findings"`
	CriticalCount  int                  `json:"critical_count"`
	HighCount      int                  `json:"high_count"`
	MediumCount    int                  `json:"medium_count"`
	LowCount       int                  `json:"low_count"`
	Findings       []models.CodeFinding `json:"findings"`
	ExitCode       int                  `json:"exit_code"`
	Duration       time.Duration        `json:"duration"`
	IsBlocking     bool                 `json:"is_blocking"`
}

// SARIFLog represents the root of a SARIF v2.1.0 report for CI integrations.
type SARIFLog struct {
	Version string     `json:"version"`
	Schema  string     `json:"$schema"`
	Runs    []SARIFRun `json:"runs"`
}

// SARIFRun details tool execution and results in SARIF.
type SARIFRun struct {
	Tool    SARIFTool     `json:"tool"`
	Results []SARIFResult `json:"results"`
}

// SARIFTool defines driver metadata.
type SARIFTool struct {
	Driver SARIFDriver `json:"driver"`
}

// SARIFDriver encapsulates tool identification.
type SARIFDriver struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	InformationURI string `json:"informationUri"`
}

// SARIFResult specifies an individual finding in SARIF.
type SARIFResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"` // "error", "warning", "note"
	Message   SARIFMessage    `json:"message"`
	Locations []SARIFLocation `json:"locations"`
}

// SARIFMessage holds message text.
type SARIFMessage struct {
	Text string `json:"text"`
}

// SARIFLocation maps the finding to a physical file and line.
type SARIFLocation struct {
	PhysicalLocation SARIFPhysicalLocation `json:"physicalLocation"`
}

// SARIFPhysicalLocation points to the file URI and region.
type SARIFPhysicalLocation struct {
	ArtifactLocation SARIFArtifactLocation `json:"artifactLocation"`
	Region           SARIFRegion           `json:"region"`
}

// SARIFArtifactLocation holds the relative file path.
type SARIFArtifactLocation struct {
	URI string `json:"uri"`
}

// SARIFRegion points to line numbers.
type SARIFRegion struct {
	StartLine int `json:"startLine"`
	EndLine   int `json:"endLine,omitempty"`
}
