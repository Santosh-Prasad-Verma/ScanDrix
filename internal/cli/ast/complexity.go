// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package ast

import (
	"strings"
)

// CalculateComplexity computes cyclomatic complexity, cognitive complexity, and nesting depth for code snippet lines.
func CalculateComplexity(lines []string) (cyclomatic int, cognitive int, maxDepth int) {
	cyclomatic = 1
	cognitive = 0
	maxDepth = 0
	currentDepth := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		// Track indentation / block depth
		openBraces := strings.Count(line, "{")
		closeBraces := strings.Count(line, "}")
		currentDepth += (openBraces - closeBraces)
		if currentDepth < 0 {
			currentDepth = 0
		}
		if currentDepth > maxDepth {
			maxDepth = currentDepth
		}

		// Decision points
		lower := strings.ToLower(trimmed)

		isDecision := false
		if strings.HasPrefix(lower, "if ") || strings.HasPrefix(lower, "if(") ||
			strings.HasPrefix(lower, "else if") || strings.HasPrefix(lower, "elif ") {
			isDecision = true
			cyclomatic++
			cognitive += (1 + currentDepth)
		} else if strings.HasPrefix(lower, "for ") || strings.HasPrefix(lower, "for(") ||
			strings.HasPrefix(lower, "while ") || strings.HasPrefix(lower, "while(") {
			isDecision = true
			cyclomatic++
			cognitive += (1 + currentDepth)
		} else if strings.HasPrefix(lower, "case ") || strings.HasPrefix(lower, "case:") {
			isDecision = true
			cyclomatic++
			cognitive += 1
		} else if strings.HasPrefix(lower, "catch ") || strings.HasPrefix(lower, "except ") {
			isDecision = true
			cyclomatic++
			cognitive += (1 + currentDepth)
		}

		// Boolean operators increment cyclomatic complexity
		andCount := strings.Count(line, "&&")
		orCount := strings.Count(line, "||")
		cyclomatic += (andCount + orCount)
		if andCount > 0 || orCount > 0 {
			cognitive += (andCount + orCount)
		}

		_ = isDecision
	}

	return cyclomatic, cognitive, maxDepth
}
