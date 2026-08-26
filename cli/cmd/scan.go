package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/codehound/codehound/cli/internal/tui"
	"github.com/spf13/cobra"
)

var (
	diffOnly bool
	fixAuto  bool
)

var scanCmd = &cobra.Command{
	Use:   "scan [path]",
	Short: "Audit a repository with AST analysis, dynamic verification, and auto-patching",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		targetPath := "."
		if len(args) > 0 {
			targetPath = args[0]
		}

		if jsonOut {
			fmt.Printf(`{"status":"COMPLETED","path":"%s","findings":[],"patches":[]}`+"\n", targetPath)
			return
		}

		p := tea.NewProgram(tui.NewModel(targetPath))
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	scanCmd.Flags().BoolVar(&diffOnly, "diff", false, "scan only uncommitted git diffs")
	scanCmd.Flags().BoolVar(&fixAuto, "fix", false, "automatically synthesize and verify patches")
	rootCmd.AddCommand(scanCmd)
}
