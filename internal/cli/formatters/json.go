package formatters

import (
	"encoding/json"
	"io"

	"github.com/scandrix/backend/internal/cli/types"
)

// JSONFormatter formats review results into indented JSON or streaming NDJSON.
type JSONFormatter struct {
	Pretty bool
}

// NewJSONFormatter creates a new JSONFormatter.
func NewJSONFormatter(pretty bool) *JSONFormatter {
	return &JSONFormatter{
		Pretty: pretty,
	}
}

// Format writes the ReviewResult as JSON to writer w.
func (j *JSONFormatter) Format(w io.Writer, result *types.ReviewResult) error {
	encoder := json.NewEncoder(w)
	if j.Pretty {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(result)
}

// FormatNDJSON streams individual issues as newline-delimited JSON objects.
func (j *JSONFormatter) FormatNDJSON(w io.Writer, issues []types.ReviewIssue) error {
	encoder := json.NewEncoder(w)
	for _, issue := range issues {
		if err := encoder.Encode(issue); err != nil {
			return err
		}
	}
	return nil
}
