// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package ast

import (
	"fmt"
)

// Standard threshold constants for maintainability and code health
const (
	MaxRecommendedComplexity = 15
	MaxRecommendedDepth      = 4
	MaxRecommendedLines      = 80
)

// DetectCodeSmells checks extracted functions against standard software design metrics.
func DetectCodeSmells(analysis *FileAnalysis) []CodeSmell {
	var smells []CodeSmell

	for _, fn := range analysis.Functions {
		// 1. High Cyclomatic / Cognitive Complexity
		if fn.CyclomaticComplexity > MaxRecommendedComplexity {
			smells = append(smells, CodeSmell{
				Type:         SmellHighComplexity,
				FilePath:     analysis.FilePath,
				FunctionName: fn.Name,
				StartLine:    fn.StartLine,
				EndLine:      fn.EndLine,
				Severity:     "warning",
				Description: fmt.Sprintf("Function %q has cyclomatic complexity of %d (recommended max: %d)",
					fn.Name, fn.CyclomaticComplexity, MaxRecommendedComplexity),
				Recommendation: "Break down into smaller helper functions or simplify branching decision trees.",
			})
		}

		// 2. Deep Nesting
		if fn.NestingDepth > MaxRecommendedDepth {
			smells = append(smells, CodeSmell{
				Type:         SmellDeepNesting,
				FilePath:     analysis.FilePath,
				FunctionName: fn.Name,
				StartLine:    fn.StartLine,
				EndLine:      fn.EndLine,
				Severity:     "warning",
				Description: fmt.Sprintf("Function %q exceeds maximum nesting depth of %d (depth: %d)",
					fn.Name, MaxRecommendedDepth, fn.NestingDepth),
				Recommendation: "Use guard clauses, early returns, or extract deeply nested loops into sub-methods.",
			})
		}

		// 3. Excessively Long Function
		if fn.LinesOfCode > MaxRecommendedLines {
			smells = append(smells, CodeSmell{
				Type:         SmellLongFunction,
				FilePath:     analysis.FilePath,
				FunctionName: fn.Name,
				StartLine:    fn.StartLine,
				EndLine:      fn.EndLine,
				Severity:     "info",
				Description: fmt.Sprintf("Function %q is %d lines long (recommended max: %d lines)",
					fn.Name, fn.LinesOfCode, MaxRecommendedLines),
				Recommendation: "Extract cohesive logic blocks into modular helper functions.",
			})
		}
	}

	return smells
}
