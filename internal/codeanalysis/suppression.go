package codeanalysis

import (
	"regexp"
	"strings"
)

// SuppressionFilter checks whether findings on a given line or file have been explicitly silenced by developers.
type SuppressionFilter struct {
	fileSuppressed bool
	lineRules      map[int]map[string]bool // line -> ruleID -> true
	allLineRules   map[int]bool            // line -> true (suppress all on this line)
}

var (
	ignoreAllRegex  = regexp.MustCompile(`(?i)(?:@scandrix-ignore-all|@scandrix-disable-file)`)
	ignoreLineRegex = regexp.MustCompile(`(?i)(?:@scandrix-ignore|@scandrix-disable|nolint|noqa)(?:\((.*?)\)|\s+([a-zA-Z0-9_-]+))?`)
)

// ParseSuppressions builds a SuppressionFilter by scanning lines in a file or patch.
func ParseSuppressions(lines []string) *SuppressionFilter {
	filter := &SuppressionFilter{
		lineRules:    make(map[int]map[string]bool),
		allLineRules: make(map[int]bool),
	}

	for idx, line := range lines {
		lineNo := idx + 1
		trimmed := strings.TrimSpace(line)

		if ignoreAllRegex.MatchString(trimmed) {
			filter.fileSuppressed = true
			return filter
		}

		if match := ignoreLineRegex.FindStringSubmatch(trimmed); len(match) > 0 {
			ruleID := ""
			if len(match) > 1 && match[1] != "" {
				ruleID = strings.TrimSpace(match[1])
			} else if len(match) > 2 && match[2] != "" {
				ruleID = strings.TrimSpace(match[2])
			}

			// Apply to the current line or next line if placed as comment header
			targetLines := []int{lineNo, lineNo + 1}
			for _, target := range targetLines {
				if ruleID == "" {
					filter.allLineRules[target] = true
				} else {
					if filter.lineRules[target] == nil {
						filter.lineRules[target] = make(map[string]bool)
					}
					filter.lineRules[target][ruleID] = true
				}
			}
		}
	}

	return filter
}

// IsSuppressed checks if a given finding is silenced.
func (f *SuppressionFilter) IsSuppressed(lineNo int, ruleID string) bool {
	if f.fileSuppressed {
		return true
	}
	if f.allLineRules[lineNo] {
		return true
	}
	if rules, exists := f.lineRules[lineNo]; exists {
		if rules[ruleID] || rules["all"] || rules["*"] {
			return true
		}
	}
	return false
}
