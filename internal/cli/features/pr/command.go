// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package pr

import (
	"fmt"
	"os"

	"github.com/scandrix/backend/internal/cli/git"
	"github.com/scandrix/backend/internal/cli/pr"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/spf13/cobra"
)

// CreatePRCommand initializes the `scandrix pr` command tree.
func CreatePRCommand() *cobra.Command {
	var (
		serverURL string
		authToken string
	)

	prCmd := &cobra.Command{
		Use:   "pr <number|url>",
		Short: "Inspect, review, or fetch suggestions for pull requests",
		Long: `Review remote pull requests from GitHub or GitLab directly from your terminal,
or query suggestions and validate PR compliance against task specifications.

Examples:
  scandrix pr 42
  scandrix pr https://github.com/org/repo/pull/42
  scandrix pr suggestions --pr-number 42
  scandrix pr business-validation --task-id PROJ-101 --staged`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}

			input := args[0]
			namespace, prNum, err := pr.ParsePRInput(input)
			if err != nil {
				return err
			}

			utils.Info("Fetching pull request #%d (%s)...", prNum, namespace)

			// Attempt GitHub API or local git fetch
			diff, err := pr.FetchDiffFromGit(cmd.Context(), prNum, "main")
			if err != nil {
				token := os.Getenv("GITHUB_TOKEN")
				if token == "" {
					token = os.Getenv("GH_TOKEN")
				}
				var apiErr error
				diff, apiErr = pr.FetchDiffFromGitHubAPI(cmd.Context(), namespace, prNum, token)
				if apiErr != nil {
					return fmt.Errorf("could not fetch PR diff via Git (%v) or API: %w", err, apiErr)
				}
			}

			utils.Success("✔ Retrieved diff for PR #%d (%d bytes)", prNum, len(diff))
			fmt.Println("\nRun 'scandrix review' with git branch or commit to perform full AST and security analysis.")
			return nil
		},
	}

	prCmd.PersistentFlags().StringVar(&serverURL, "server", "", "ScanDrix API server URL override")
	prCmd.PersistentFlags().StringVar(&authToken, "token", "", "Authentication token or team key override")

	// 1. `pr suggestions` subcommand
	var (
		suggPRURL      string
		suggPRNumber   int
		suggRepoID     string
		suggSeverities string
		suggCategories string
		suggFields     string
		suggFormat     string
		suggOutput     string
		suggQuiet      bool
		suggAgent      bool
	)

	suggestionsCmd := &cobra.Command{
		Use:   "suggestions",
		Short: "Fetch and filter review suggestions for a pull request",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := pr.NewPRClient(serverURL, authToken)
			opts := SuggestionsOptions{
				PRURL:      suggPRURL,
				PRNumber:   suggPRNumber,
				RepoID:     suggRepoID,
				Severity:   ParseCSVList(suggSeverities),
				Category:   ParseCSVList(suggCategories),
				Fields:     suggFields,
				Format:     suggFormat,
				Output:     suggOutput,
				Quiet:      suggQuiet,
				IsAgent:    suggAgent,
				OutputFile: suggOutput,
			}
			return ExecuteSuggestionsAction(cmd.Context(), client, opts)
		},
	}

	suggestionsCmd.Flags().StringVar(&suggPRURL, "pr-url", "", "Pull request URL")
	suggestionsCmd.Flags().IntVar(&suggPRNumber, "pr-number", 0, "Pull request number")
	suggestionsCmd.Flags().StringVar(&suggRepoID, "repo-id", "", "Repository ID for pull request")
	suggestionsCmd.Flags().StringVar(&suggSeverities, "severity", "", "Comma-separated severities (info,warning,error,critical)")
	suggestionsCmd.Flags().StringVar(&suggCategories, "category", "", "Comma-separated categories (security_vulnerability,performance,...)")
	suggestionsCmd.Flags().StringVar(&suggFields, "fields", "", "Select response fields (JSON/agent mode only)")
	suggestionsCmd.Flags().StringVar(&suggFormat, "format", "terminal", "Output format (terminal, json, markdown, sarif)")
	suggestionsCmd.Flags().StringVarP(&suggOutput, "output", "o", "", "Output file path")
	suggestionsCmd.Flags().BoolVarP(&suggQuiet, "quiet", "q", false, "Silence progress output")
	suggestionsCmd.Flags().BoolVar(&suggAgent, "agent", false, "Emit structured machine envelope for AI agents")

	// 2. `pr business-validation` subcommand
	var (
		bvTaskURL string
		bvTaskID  string
		bvStaged  bool
		bvCommit  string
		bvBranch  string
		bvDryRun  bool
		bvJSON    bool
		bvQuiet   bool
	)

	businessValCmd := &cobra.Command{
		Use:   "business-validation [files...]",
		Short: "Validate code changes against product requirements or task specifications",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := pr.NewPRClient(serverURL, authToken)
			gitSvc := git.NewGitService(".")
			opts := BusinessValidationOptions{
				Files:      args,
				TaskURL:    bvTaskURL,
				TaskID:     bvTaskID,
				Staged:     bvStaged,
				Commit:     bvCommit,
				Branch:     bvBranch,
				DryRun:     bvDryRun,
				JSONOutput: bvJSON,
				Quiet:      bvQuiet,
			}
			code, err := ExecuteBusinessValidationAction(cmd.Context(), client, gitSvc, opts)
			if err != nil {
				return err
			}
			if code != 0 {
				os.Exit(code)
			}
			return nil
		},
	}

	businessValCmd.Flags().StringVar(&bvTaskURL, "task-url", "", "Task URL to append for requirement verification (Linear, Jira, etc.)")
	businessValCmd.Flags().StringVar(&bvTaskID, "task-id", "", "Task ID or issue key (e.g. KC-1441, PROJ-42)")
	businessValCmd.Flags().BoolVarP(&bvStaged, "staged", "s", false, "Validate staged changes only")
	businessValCmd.Flags().StringVarP(&bvCommit, "commit", "c", "", "Validate diff from a specific commit")
	businessValCmd.Flags().StringVarP(&bvBranch, "branch", "b", "", "Compare against a base branch")
	businessValCmd.Flags().BoolVar(&bvDryRun, "dry-run", false, "Print request payload without calling API")
	businessValCmd.Flags().BoolVar(&bvJSON, "json", false, "Output validation results in JSON")
	businessValCmd.Flags().BoolVarP(&bvQuiet, "quiet", "q", false, "Silence progress output")

	prCmd.AddCommand(suggestionsCmd)
	prCmd.AddCommand(businessValCmd)

	return prCmd
}
