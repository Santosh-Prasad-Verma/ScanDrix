package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	cfgFile string
	verbose bool
	jsonOut bool
)

var rootCmd = &cobra.Command{
	Use:   "codehound",
	Short: "CodeHound: Autonomous Code Intelligence, Security, and Dynamic Verification Engine",
	Long: `CodeHound is a next-generation DevSecOps and code intelligence platform.
It combines multilingual Tree-sitter AST parsing, taint analysis, and
microVM sandboxes to detect, prove, and auto-patch vulnerabilities with zero hallucinations.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.codehound.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose debug logging")
	rootCmd.PersistentFlags().BoolVar(&jsonOut, "json", false, "output results in structured JSON format")
}
