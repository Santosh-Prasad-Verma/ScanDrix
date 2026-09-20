// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Business Rules Validation Agent
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package businessrules

import (
	"fmt"
	"strings"
)

// BuildBusinessRulesContractViolationFeedback creates user-facing feedback when
// input or output contracts for business rules validation fail validation.
func BuildBusinessRulesContractViolationFeedback(
	_userLanguage string,
	phase string,
	missingFields []string,
) string {
	fields := "unknown"
	if len(missingFields) > 0 {
		fields = strings.Join(missingFields, ", ")
	}

	if phase == "input" {
		return fmt.Sprintf(`## ⚠️ Missing Validation Context

I couldn't start the skill because required context fields are missing: `+"`%s`"+`.

### How to fix
- Ensure the event includes organization, team, repository, and pull request number.
- Run again: `+"`@drixy -v business-logic`", fields)
	}

	return fmt.Sprintf(`## ⚠️ Invalid Skill Response

The analysis step returned an incomplete response and failed output contract validation.

Missing fields: `+"`%s`"+`.

Please try again in a moment.`, fields)
}
