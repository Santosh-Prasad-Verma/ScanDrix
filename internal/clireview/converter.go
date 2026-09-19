package clireview

import (
	"github.com/scandrix/backend/internal/clireview/pipeline"
)

// CliInputConverter transforms CLI requests, diffs, and pipeline results.
type CliInputConverter = pipeline.CliInputConverter

// NewCliInputConverter instantiates a new converter.
func NewCliInputConverter() *CliInputConverter {
	return pipeline.NewCliInputConverter()
}
