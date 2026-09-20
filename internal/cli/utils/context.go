// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"time"
)

// COMMAND CONTEXT SPECIFICATION

// CommandContext encapsulates the execution environment for a CLI command.
type CommandContext struct {
	Command      string
	Mode         string // "agent" or "human"
	IsAgent      bool
	OutputFormat string // "terminal", "json", "markdown", "prompt", "sarif"
	StartedAt    time.Time
	OutputFile   string
	Verbose      bool
	Quiet        bool
}

// CONTEXT BUILDER

// NewCommandContext constructs a CommandContext from command options and global flags.
func NewCommandContext(command, format, output string, verbose, quiet, isAgent bool) *CommandContext {
	outFmt := format
	if outFmt == "" {
		outFmt = "terminal"
	}
	mode := "human"
	if isAgent {
		mode = "agent"
		outFmt = "json"
	}
	return &CommandContext{
		Command:      command,
		Mode:         mode,
		IsAgent:      isAgent,
		OutputFormat: outFmt,
		StartedAt:    time.Now().UTC(),
		OutputFile:   output,
		Verbose:      verbose,
		Quiet:        quiet,
	}
}
