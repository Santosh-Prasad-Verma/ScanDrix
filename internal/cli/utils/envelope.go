// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// AGENT ENVELOPE CONTRACT CONSTANTS & METADATA

const (
	CLIVersion    = "v1.2.0"
	SchemaVersion = "1.0"
)

// AgentEnvelopeMeta contains execution metadata for agent mode payloads.
type AgentEnvelopeMeta struct {
	SchemaVersion string `json:"schemaVersion"`
	CLIVersion    string `json:"cliVersion"`
	Mode          string `json:"mode"`
	DurationMs    int64  `json:"durationMs"`
}

// AgentErrorPayload details error specifics in agent mode.
type AgentErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// AgentSuccessEnvelope wraps successful results in a standardized envelope.
type AgentSuccessEnvelope struct {
	OK      bool               `json:"ok"`
	Command string             `json:"command"`
	Data    any                `json:"data"`
	Error   *AgentErrorPayload `json:"error"`
	Meta    AgentEnvelopeMeta  `json:"meta"`
}

// AgentErrorEnvelope wraps error results in a standardized envelope.
type AgentErrorEnvelope struct {
	OK      bool              `json:"ok"`
	Command string            `json:"command"`
	Data    any               `json:"data"`
	Error   AgentErrorPayload `json:"error"`
	Meta    AgentEnvelopeMeta `json:"meta"`
}

// ENVELOPE BUILDERS & EMITTERS

// BuildAgentSuccessEnvelope constructs an AgentSuccessEnvelope.
func BuildAgentSuccessEnvelope(command string, data any, startTime time.Time) AgentSuccessEnvelope {
	durationMs := time.Since(startTime).Milliseconds()
	if durationMs < 0 {
		durationMs = 0
	}
	return AgentSuccessEnvelope{
		OK:      true,
		Command: command,
		Data:    data,
		Error:   nil,
		Meta: AgentEnvelopeMeta{
			SchemaVersion: SchemaVersion,
			CLIVersion:    CLIVersion,
			Mode:          "agent",
			DurationMs:    durationMs,
		},
	}
}

// BuildAgentErrorEnvelope constructs an AgentErrorEnvelope.
func BuildAgentErrorEnvelope(command string, errPayload AgentErrorPayload, startTime time.Time) AgentErrorEnvelope {
	durationMs := time.Since(startTime).Milliseconds()
	if durationMs < 0 {
		durationMs = 0
	}
	return AgentErrorEnvelope{
		OK:      false,
		Command: command,
		Data:    nil,
		Error:   errPayload,
		Meta: AgentEnvelopeMeta{
			SchemaVersion: SchemaVersion,
			CLIVersion:    CLIVersion,
			Mode:          "agent",
			DurationMs:    durationMs,
		},
	}
}

// EmitAgentEnvelope writes the envelope to the specified output file or stdout.
func EmitAgentEnvelope(envelope any, outputFile string) error {
	data, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return fmt.Errorf("failed marshaling agent envelope: %w", err)
	}

	if outputFile != "" {
		if writeErr := os.WriteFile(outputFile, data, 0644); writeErr != nil {
			return fmt.Errorf("failed writing agent envelope to %s: %w", outputFile, writeErr)
		}
		return nil
	}

	fmt.Println(string(data))
	return nil
}
