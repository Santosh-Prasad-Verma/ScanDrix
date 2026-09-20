// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scandrix/backend/internal/cli/engine"
	"github.com/scandrix/backend/internal/cli/services/git"
	"github.com/scandrix/backend/internal/cli/tui"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/spf13/cobra"
)

// TUI COMMAND FLAGS

var (
	tuiStaged   bool
	tuiBranch   string
	tuiCommit   string
	tuiDiffFile string
)

// TUI COMMAND SPECIFICATION (Interactive Bubbletea Terminal)

var tuiCmd = &cobra.Command{
	Use:     "tui",
	Aliases: []string{"dashboard", "ui"},
	Short:   "Launch interactive Bubbletea developer terminal cockpit dashboard",
	Long:    `Interactive terminal dashboard for reviewing findings, browsing unified diffs, and applying fixes.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var rawDiff string
		var err error

		if tuiDiffFile != "" {
			bytes, readErr := os.ReadFile(tuiDiffFile)
			if readErr != nil {
				return utils.NewCommandError("INVALID_INPUT", fmt.Sprintf("Failed to read diff file: %v", readErr))
			}
			rawDiff = string(bytes)
		} else {
			gitSvc := git.DefaultService()
			if gitSvc.IsGitRepository(context.Background()) {
				if tuiStaged {
					rawDiff, err = gitSvc.GetStagedDiff(context.Background())
				} else if tuiBranch != "" {
					rawDiff, err = gitSvc.GetBranchDiff(context.Background(), tuiBranch)
				} else if tuiCommit != "" {
					rawDiff, err = gitSvc.GetCommitDiff(context.Background(), tuiCommit)
				} else {
					rawDiff, err = gitSvc.GetWorkingTreeDiff(context.Background())
					if err == nil && strings.TrimSpace(rawDiff) == "" {
						rawDiff, err = gitSvc.GetStagedDiff(context.Background())
					}
				}
				if err != nil {
					return utils.NewCommandError("GIT_ERROR", fmt.Sprintf("Failed extracting diff: %v", err))
				}
			}
		}

		runner := engine.NewCLIRunner()
		m := tui.NewModel(rawDiff, runner)
		p := tea.NewProgram(m, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			return utils.NewCommandError("INTERNAL_ERROR", fmt.Sprintf("Error running ScanDrix TUI: %v", err))
		}

		return nil
	},
}

// COMMAND REGISTRATION & FLAG INITIALIZATION

func init() {
	tuiCmd.Flags().BoolVarP(&tuiStaged, "staged", "s", false, "Inspect staged git changes")
	tuiCmd.Flags().StringVarP(&tuiBranch, "branch", "b", "", "Inspect changes against a target branch")
	tuiCmd.Flags().StringVarP(&tuiCommit, "commit", "c", "", "Inspect changes in a specific commit")
	tuiCmd.Flags().StringVar(&tuiDiffFile, "file", "", "Inspect a unified .diff or .patch file")
}
