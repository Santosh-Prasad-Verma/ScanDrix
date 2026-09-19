// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// SCHEMA INTROSPECTION CONTRACTS

// OptionSchema details a command flag.
type OptionSchema struct {
	Flags       string `json:"flags"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Default     any    `json:"default,omitempty"`
}

// CommandSchema details a command and its subcommands.
type CommandSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Usage       string          `json:"usage"`
	Options     []OptionSchema  `json:"options,omitempty"`
	Subcommands []CommandSchema `json:"subcommands,omitempty"`
}

// COBRA COMMAND TREE INTROSPECTION

// BuildCommandSchema recursively introspects a Cobra command tree.
func BuildCommandSchema(cmd *cobra.Command) CommandSchema {
	schema := CommandSchema{
		Name:        cmd.Name(),
		Description: cmd.Short,
		Usage:       cmd.UseLine(),
	}

	if schema.Description == "" {
		schema.Description = cmd.Long
	}

	// Extract flags
	var options []OptionSchema
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		flagStr := "--" + f.Name
		if f.Shorthand != "" {
			flagStr = "-" + f.Shorthand + ", " + flagStr
		}
		if f.Value.Type() != "bool" {
			flagStr += " <" + f.Value.Type() + ">"
		}

		options = append(options, OptionSchema{
			Flags:       flagStr,
			Description: f.Usage,
			Required:    f.Annotations != nil && len(f.Annotations[cobra.BashCompOneRequiredFlag]) > 0,
			Default:     f.DefValue,
		})
	})
	schema.Options = options

	// Extract visible subcommands
	var subcommands []CommandSchema
	for _, sub := range cmd.Commands() {
		if !sub.Hidden {
			subcommands = append(subcommands, BuildCommandSchema(sub))
		}
	}
	schema.Subcommands = subcommands

	return schema
}

// FindCommandByPath finds a subcommand given a space-separated path (e.g. "pr suggestions").
func FindCommandByPath(root *cobra.Command, path string) *cobra.Command {
	parts := strings.Fields(path)
	if len(parts) == 0 {
		return root
	}

	current := root
	for _, part := range parts {
		var found *cobra.Command
		for _, sub := range current.Commands() {
			if sub.Name() == part {
				found = sub
				break
			}
			for _, alias := range sub.Aliases {
				if alias == part {
					found = sub
					break
				}
			}
		}
		if found == nil {
			return nil
		}
		current = found
	}
	return current
}
