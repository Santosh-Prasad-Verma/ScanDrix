// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package ci

import (
	"encoding/json"
)

// SARIFReport represents the root OASIS SARIF v2.1.0 document.
type SARIFReport struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []SARIFRun `json:"runs"`
}

type SARIFRun struct {
	Tool    SARIFTool     `json:"tool"`
	Results []SARIFResult `json:"results"`
}

type SARIFTool struct {
	Driver SARIFDriver `json:"driver"`
}

type SARIFDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version"`
	InformationURI string      `json:"informationUri"`
	Rules          []SARIFRule `json:"rules"`
}

type SARIFRule struct {
	ID               string               `json:"id"`
	Name             string               `json:"name"`
	ShortDescription SARIFMultiformatMsg  `json:"shortDescription"`
	FullDescription  *SARIFMultiformatMsg `json:"fullDescription,omitempty"`
	HelpURI          string               `json:"helpUri,omitempty"`
}

type SARIFResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"` // "error", "warning", "note"
	Message   SARIFMessage    `json:"message"`
	Locations []SARIFLocation `json:"locations"`
}

type SARIFMessage struct {
	Text string `json:"text"`
}

type SARIFMultiformatMsg struct {
	Text string `json:"text"`
}

type SARIFLocation struct {
	PhysicalLocation SARIFPhysicalLocation `json:"physicalLocation"`
}

type SARIFPhysicalLocation struct {
	ArtifactLocation SARIFArtifactLocation `json:"artifactLocation"`
	Region           SARIFRegion           `json:"region"`
}

type SARIFArtifactLocation struct {
	URI string `json:"uri"`
}

type SARIFRegion struct {
	StartLine   int `json:"startLine"`
	EndLine     int `json:"endLine,omitempty"`
	StartColumn int `json:"startColumn,omitempty"`
}

// FormatSARIF serializes findings into the OASIS SARIF v2.1.0 standard schema.
func FormatSARIF(findings []CIFinding) ([]byte, error) {
	ruleMap := make(map[string]SARIFRule)
	results := make([]SARIFResult, 0, len(findings))

	for _, f := range findings {
		ruleID := f.RuleID
		if ruleID == "" {
			ruleID = "SD-" + f.Category
		}

		if _, exists := ruleMap[ruleID]; !exists {
			ruleMap[ruleID] = SARIFRule{
				ID:   ruleID,
				Name: f.RuleTitle,
				ShortDescription: SARIFMultiformatMsg{
					Text: f.RuleTitle,
				},
				HelpURI: "https://scandrix.dev/rules/" + ruleID,
			}
		}

		level := "note"
		switch f.Severity {
		case FailCritical, FailError:
			level = "error"
		case FailWarning:
			level = "warning"
		case FailInfo:
			level = "note"
		}

		startLine := f.StartLine
		if startLine <= 0 {
			startLine = 1
		}
		endLine := f.EndLine
		if endLine < startLine {
			endLine = startLine
		}

		res := SARIFResult{
			RuleID: ruleID,
			Level:  level,
			Message: SARIFMessage{
				Text: f.Message,
			},
			Locations: []SARIFLocation{
				{
					PhysicalLocation: SARIFPhysicalLocation{
						ArtifactLocation: SARIFArtifactLocation{
							URI: f.FilePath,
						},
						Region: SARIFRegion{
							StartLine:   startLine,
							EndLine:     endLine,
							StartColumn: 1,
						},
					},
				},
			},
		}
		results = append(results, res)
	}

	rules := make([]SARIFRule, 0, len(ruleMap))
	for _, r := range ruleMap {
		rules = append(rules, r)
	}

	report := SARIFReport{
		Schema:  "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json",
		Version: "2.1.0",
		Runs: []SARIFRun{
			{
				Tool: SARIFTool{
					Driver: SARIFDriver{
						Name:           "ScanDrix",
						Version:        "1.0.0",
						InformationURI: "https://scandrix.dev",
						Rules:          rules,
					},
				},
				Results: results,
			},
		},
	}

	return json.MarshalIndent(report, "", "  ")
}
