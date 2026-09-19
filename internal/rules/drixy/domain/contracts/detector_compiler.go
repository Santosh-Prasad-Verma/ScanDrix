// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: detector_compiler.go
// ═══════════════════════════════════════════════════════════════

package contracts

import (
	"context"

	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
)

// CompileResult summarizes whether a mechanical rule was successfully compiled into a T0 detector.
type CompileResult struct {
	Compiled      bool
	DeclineReason string
	Detector      *interfaces.DrixyRuleDetector
}

// IDrixyRuleDetectorCompiler compiles rules into gated T0 deterministic detectors.
type IDrixyRuleDetectorCompiler interface {
	CompileAndSave(
		ctx context.Context,
		organizationID string,
		teamID string,
		ruleUUID string,
		rule *interfaces.DrixyRule,
	) (CompileResult, error)
}
