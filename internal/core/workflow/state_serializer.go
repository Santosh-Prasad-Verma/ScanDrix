package workflow

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"time"
)

// SerializationStrategy defines how pipeline context checkpoints are packed.
type SerializationStrategy string

const (
	StrategyFull       SerializationStrategy = "full"
	StrategyDelta      SerializationStrategy = "delta"
	StrategyMinimal    SerializationStrategy = "minimal"
	StrategyCompressed SerializationStrategy = "compressed"

	CompressionThresholdBytes = 50 * 1024 // 50 KB threshold
)

// SerializationOptions configures the serialization strategy and base checkpoint.
type SerializationOptions struct {
	Strategy      SerializationStrategy `json:"strategy"`
	PreviousState map[string]any        `json:"previousState,omitempty"`
}

// StateSerializer mirrors ScanDrix StateSerializerService: compresses and delta-encodes pipeline states.
type StateSerializer struct{}

// NewStateSerializer instantiates a state serializer.
func NewStateSerializer() *StateSerializer {
	return &StateSerializer{}
}

// Serialize encodes a pipeline state map according to the requested strategy.
func (s *StateSerializer) Serialize(state map[string]any, options SerializationOptions) (map[string]any, error) {
	if state == nil {
		return map[string]any{}, nil
	}

	strategy := options.Strategy
	if strategy == "" {
		strategy = StrategyFull
	}

	switch strategy {
	case StrategyDelta:
		return s.serializeDelta(state, options.PreviousState), nil
	case StrategyMinimal:
		return s.serializeMinimal(state), nil
	case StrategyCompressed:
		return s.serializeCompressed(state)
	case StrategyFull:
		fallthrough
	default:
		// Auto-compress if size exceeds threshold
		rawBytes, err := json.Marshal(state)
		if err == nil && len(rawBytes) > CompressionThresholdBytes {
			return s.serializeCompressed(state)
		}
		return state, nil
	}
}

// Deserialize unpacks a state map, automatically decompressing gzip base64 payloads if present.
func (s *StateSerializer) Deserialize(data map[string]any) (map[string]any, error) {
	if data == nil {
		return map[string]any{}, nil
	}

	isCompressed, _ := data["compressed"].(bool)
	compressedStr, hasData := data["data"].(string)

	if isCompressed && hasData {
		return s.deserializeCompressed(compressedStr)
	}

	return data, nil
}

func (s *StateSerializer) serializeCompressed(state map[string]any) (map[string]any, error) {
	rawBytes, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("failed encoding state for compression: %w", err)
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(rawBytes); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}

	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	return map[string]any{
		"compressed":        true,
		"data":              encoded,
		"originalSizeBytes": len(rawBytes),
		"compressedSize":    buf.Len(),
		"timestamp":         time.Now().UTC().UnixMilli(),
	}, nil
}

func (s *StateSerializer) deserializeCompressed(encoded string) (map[string]any, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed decoding base64 compressed state: %w", err)
	}

	gz, err := gzip.NewReader(bytes.NewReader(decoded))
	if err != nil {
		return nil, fmt.Errorf("failed creating gzip reader: %w", err)
	}
	defer gz.Close()

	decompressed, err := io.ReadAll(gz)
	if err != nil {
		return nil, fmt.Errorf("failed decompressing state: %w", err)
	}

	var state map[string]any
	if err := json.Unmarshal(decompressed, &state); err != nil {
		return nil, fmt.Errorf("failed parsing decompressed JSON state: %w", err)
	}

	return state, nil
}

func (s *StateSerializer) serializeMinimal(state map[string]any) map[string]any {
	minimal := map[string]any{
		"_strategy": StrategyMinimal,
		"timestamp": time.Now().UTC().UnixMilli(),
	}

	// Essential identifiers and counts to retain
	retainedKeys := []string{
		"workflowJobId",
		"currentStage",
		"correlationId",
		"automationExecutionId",
		"repositoryId",
		"prNumber",
		"suggestionsCount",
		"status",
	}

	for _, k := range retainedKeys {
		if val, exists := state[k]; exists {
			minimal[k] = val
		}
	}

	return minimal
}

func (s *StateSerializer) serializeDelta(current, previous map[string]any) map[string]any {
	if previous == nil {
		return current
	}

	delta := map[string]any{
		"_strategy": StrategyDelta,
		"timestamp": time.Now().UTC().UnixMilli(),
	}

	// Always retain primary IDs
	for _, idKey := range []string{"workflowJobId", "currentStage", "correlationId"} {
		if v, ok := current[idKey]; ok {
			delta[idKey] = v
		}
	}

	// Record only modified or newly added keys
	for k, currentVal := range current {
		prevVal, exists := previous[k]
		if !exists || !reflect.DeepEqual(currentVal, prevVal) {
			delta[k] = currentVal
		}
	}

	return delta
}
