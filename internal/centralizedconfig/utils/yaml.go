// Package utils provides deterministic YAML serialization for centralized configurations and rules.
package utils

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// DumpCentralizedYAML serializes any data structure into formatted, deterministic YAML with 2-space indentation.
func DumpCentralizedYAML(data any) (string, error) {
	if data == nil {
		return "", nil
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)

	if err := enc.Encode(data); err != nil {
		return "", fmt.Errorf("failed to encode centralized yaml: %w", err)
	}

	return buf.String(), nil
}
