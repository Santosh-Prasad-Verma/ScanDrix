package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var Version = "1.0.0-dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the CodeHound CLI version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("CodeHound CLI v%s (Linux/amd64)\n", Version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
