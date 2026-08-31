package tui

import (
	"github.com/scandrix/backend/internal/cli/engine"
	"github.com/scandrix/backend/pkg/models"
)

// ApplyFindingFix delegates to engine.ApplyFindingFix.
func ApplyFindingFix(finding models.CodeFinding) (string, error) {
	return engine.ApplyFindingFix(".", finding)
}
