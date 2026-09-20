// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package knowledge

import (
	"math"
	"path/filepath"
	"sort"
	"time"
)

// BlastRadiusCalculator evaluates the upstream downstream impact of code changes.
type BlastRadiusCalculator struct{}

// NewBlastRadiusCalculator constructs a new blast radius analyzer.
func NewBlastRadiusCalculator() *BlastRadiusCalculator {
	return &BlastRadiusCalculator{}
}

// CalculateBlastRadius determines direct, Tier 1, and Tier 2 impacted files and symbols for a PR.
func (c *BlastRadiusCalculator) CalculateBlastRadius(
	changedFiles []string,
	changedLineRanges map[string][][2]int,
	index *SymbolIndex,
	graph *CallGraph,
) *BlastRadiusReport {
	report := &BlastRadiusReport{
		DirectlyModifiedFiles: make([]string, 0, len(changedFiles)),
		GeneratedAt:           time.Now().UTC(),
	}

	for _, f := range changedFiles {
		report.DirectlyModifiedFiles = append(report.DirectlyModifiedFiles, filepath.ToSlash(filepath.Clean(f)))
	}

	if index == nil || graph == nil {
		report.TotalImpactedFiles = len(report.DirectlyModifiedFiles)
		report.ImpactScore = 0.1
		report.RiskRating = RiskLow
		return report
	}

	// 1. Identify directly modified symbols based on changed line ranges
	directSymbols := make(map[string]struct{})
	directFilesSet := make(map[string]struct{})
	for _, f := range report.DirectlyModifiedFiles {
		directFilesSet[f] = struct{}{}
		defs := index.GetDefinitionsInFile(f)
		ranges := changedLineRanges[f]

		for _, def := range defs {
			if len(ranges) == 0 {
				// Whole file modified or no range constraint
				directSymbols[def.Name] = struct{}{}
				continue
			}

			// Check if symbol overlaps with any changed range
			for _, r := range ranges {
				start := r[0]
				end := r[1]
				if def.Location.StartLine <= end && def.Location.EndLine >= start {
					directSymbols[def.Name] = struct{}{}
					break
				}
			}
		}
	}

	for sym := range directSymbols {
		report.DirectlyModifiedSymbols = append(report.DirectlyModifiedSymbols, sym)
	}
	sort.Strings(report.DirectlyModifiedSymbols)

	// 2. Identify Tier 1 impact (direct callers)
	tier1Symbols := make(map[string]struct{})
	tier1Files := make(map[string]struct{})

	for _, sym := range report.DirectlyModifiedSymbols {
		callers := graph.GetCallers(sym)
		for _, edge := range callers {
			caller := edge.CallerName
			if _, isDirect := directSymbols[caller]; !isDirect && caller != "" {
				tier1Symbols[caller] = struct{}{}
				if edge.CallerFile != "" {
					tier1Files[edge.CallerFile] = struct{}{}
				}
			}
		}
	}

	for sym := range tier1Symbols {
		report.Tier1ImpactedSymbols = append(report.Tier1ImpactedSymbols, sym)
	}
	sort.Strings(report.Tier1ImpactedSymbols)

	for f := range tier1Files {
		if _, isDirect := directFilesSet[f]; !isDirect {
			report.Tier1ImpactedFiles = append(report.Tier1ImpactedFiles, f)
		}
	}
	sort.Strings(report.Tier1ImpactedFiles)

	// 3. Identify Tier 2 impact (transitive callers of Tier 1)
	tier2Files := make(map[string]struct{})
	for _, sym := range report.Tier1ImpactedSymbols {
		callers := graph.GetCallers(sym)
		for _, edge := range callers {
			callerFile := edge.CallerFile
			if callerFile != "" {
				if _, isDirect := directFilesSet[callerFile]; !isDirect {
					if _, isTier1 := tier1Files[callerFile]; !isTier1 {
						tier2Files[callerFile] = struct{}{}
					}
				}
			}
		}
	}

	for f := range tier2Files {
		report.Tier2ImpactedFiles = append(report.Tier2ImpactedFiles, f)
	}
	sort.Strings(report.Tier2ImpactedFiles)

	// 4. Calculate total impacted files and impact score
	allImpactedFiles := make(map[string]struct{})
	for _, f := range report.DirectlyModifiedFiles {
		allImpactedFiles[f] = struct{}{}
	}
	for _, f := range report.Tier1ImpactedFiles {
		allImpactedFiles[f] = struct{}{}
	}
	for _, f := range report.Tier2ImpactedFiles {
		allImpactedFiles[f] = struct{}{}
	}
	report.TotalImpactedFiles = len(allImpactedFiles)

	// Formula for impact score (0.0 to 1.0)
	// Base weight: directly modified files (0.05 each)
	// Tier 1 weight: 0.08 each
	// Tier 2 weight: 0.03 each
	// Tier 1 symbols weight: 0.02 each
	rawScore := float64(len(report.DirectlyModifiedFiles))*0.05 +
		float64(len(report.Tier1ImpactedFiles))*0.08 +
		float64(len(report.Tier2ImpactedFiles))*0.03 +
		float64(len(report.Tier1ImpactedSymbols))*0.02

	// Check if any directly modified symbol is exported public API
	exportedCount := 0
	for _, sym := range report.DirectlyModifiedSymbols {
		defs, ok := index.GetDefinition(sym)
		if ok && len(defs) > 0 && defs[0].Exported {
			exportedCount++
		}
	}
	if exportedCount > 0 {
		rawScore += float64(exportedCount) * 0.05
	}

	report.ImpactScore = math.Min(1.0, math.Round(rawScore*100)/100)

	// Risk Rating thresholds
	if report.ImpactScore >= 0.70 || report.TotalImpactedFiles >= 15 {
		report.RiskRating = RiskCritical
	} else if report.ImpactScore >= 0.45 || report.TotalImpactedFiles >= 8 {
		report.RiskRating = RiskHigh
	} else if report.ImpactScore >= 0.20 || report.TotalImpactedFiles >= 3 {
		report.RiskRating = RiskMedium
	} else {
		report.RiskRating = RiskLow
	}

	return report
}
