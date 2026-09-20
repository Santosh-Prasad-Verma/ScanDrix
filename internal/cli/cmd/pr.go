// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd

import (
	"fmt"
	"time"

	"github.com/scandrix/backend/internal/cli/pr"
	"github.com/scandrix/backend/internal/cli/services/api"
	"github.com/scandrix/backend/internal/cli/services/git"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/spf13/cobra"
)

// PR COMMAND FLAGS

var (
	prURLFlag        string
	prNumberFlag     int
	prRepoIDFlag     string
	prSeverityFilter string
	prCategoryFilter string
	prFieldsFlag     string
	taskURLFlag      string
	taskIDFlag       string
	prBVStagedFlag   bool
	prBVCommitFlag   string
	prBVBranchFlag   string
	prBVDryRunFlag   bool
)

// PR PARENT COMMAND (Direct Review by PR Number or URL)

var prCmd = &cobra.Command{
	Use:   "pr [number|url]",
	Short: "Review remote pull requests, query suggestions, and validate business rules",
	Long: `Fetch suggestions for remote pull requests or validate local diffs against
task specifications, Jira tickets, and acceptance criteria.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}

		// Direct PR review shortcut: scandrix pr 42 or scandrix pr https://github.com/owner/repo/pull/42
		target := args[0]
		namespace, prNum, err := pr.ParsePRInput(target)
		if err != nil {
			return err
		}

		utils.Info("Reviewing Pull Request #%d (%s)...", prNum, namespace)
		diff, err := pr.FetchDiffFromGit(cmd.Context(), prNum, "main")
		if err != nil {
			// Fallback: fetch via GitHub API if token available
			diff, err = pr.FetchDiffFromGitHubAPI(cmd.Context(), namespace, prNum, "")
			if err != nil {
				return err
			}
		}

		// Run review on extracted diff
		reviewOpts.File = ""
		reviewOpts.Files = nil
		client := api.NewClient("", "", "")
		resp, err := client.SubmitReview(cmd.Context(), api.ReviewRequest{
			Diff: diff,
		})
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("pr", resp, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		utils.Success("✔ PR #%d review complete. Total findings: %d", prNum, len(resp.Findings))
		return nil
	},
}

// PR SUGGESTIONS COMMAND

var prSuggestionsCmd = &cobra.Command{
	Use:   "suggestions",
	Short: "Fetch automated review suggestions for a pull request",
	RunE: func(cmd *cobra.Command, args []string) error {
		prNum := prNumberFlag
		if prNum == 0 && prURLFlag != "" {
			_, parsedNum, err := pr.ParsePRInput(prURLFlag)
			if err == nil {
				prNum = parsedNum
			}
		}
		if prNum == 0 {
			return fmt.Errorf("must specify --pr-number <num> or --pr-url <url>")
		}

		client := api.NewClient("", "", "")
		findings, err := client.FetchSuggestions(cmd.Context(), prRepoIDFlag, prNum)
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			data := any(findings)
			if prFieldsFlag != "" {
				fields := utils.ParseFieldList(prFieldsFlag)
				masked, _ := utils.ApplyFieldMask(findings, fields)
				data = masked
			}
			env := utils.BuildAgentSuccessEnvelope("pr suggestions", data, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		fmt.Printf("Pull Request #%d Suggestions (%d):\n", prNum, len(findings))
		for i, f := range findings {
			fmt.Printf("  %d. [%s] %s: %s:%d\n     %s\n", i+1, f.Severity, f.Title, f.FilePath, f.StartLine, f.Description)
		}
		return nil
	},
}

// PR BUSINESS RULES VALIDATION (Linear / Jira Criteria)

var prBusinessValidationCmd = &cobra.Command{
	Use:   "business-validation [files...]",
	Short: "Validate local diff changes against project management task specifications",
	RunE: func(cmd *cobra.Command, args []string) error {
		gitSrv := git.DefaultService()
		var diff string
		var err error

		if prBVStagedFlag {
			diff, err = gitSrv.GetStagedDiff(cmd.Context())
		} else if prBVBranchFlag != "" {
			diff, err = gitSrv.GetBranchDiff(cmd.Context(), prBVBranchFlag)
		} else if prBVCommitFlag != "" {
			diff, err = gitSrv.GetCommitDiff(cmd.Context(), prBVCommitFlag)
		} else if len(args) > 0 {
			diff, err = gitSrv.GetFilesDiff(cmd.Context(), args, false)
		} else {
			diff, err = gitSrv.GetWorkingTreeDiff(cmd.Context())
		}

		if err != nil {
			return err
		}

		req := pr.BusinessValidationRequest{
			TaskURL: taskURLFlag,
			TaskID:  taskIDFlag,
			RawDiff: diff,
		}

		if prBVDryRunFlag {
			fmt.Println("=== Business Validation Payload (Dry Run) ===")
			fmt.Printf("Task ID:   %s\n", req.TaskID)
			fmt.Printf("Task URL:  %s\n", req.TaskURL)
			fmt.Printf("Diff Size: %d bytes\n", len(req.RawDiff))
			return nil
		}

		prClient := pr.NewPRClient("", "")
		resp, err := prClient.RunBusinessValidation(cmd.Context(), req)
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("pr business-validation", resp, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		statusLabel := "\033[32mPASSED\033[0m"
		if !resp.Valid {
			statusLabel = "\033[31mACTION REQUIRED\033[0m"
		}
		fmt.Printf("\n=== Business Rules Validation: %s ===\n", statusLabel)
		fmt.Printf("Compliance Score: %.1f%%\n", resp.Score*100)
		if resp.TaskTitle != "" {
			fmt.Printf("Task:             %s\n", resp.TaskTitle)
		}

		if len(resp.Requirements) > 0 {
			fmt.Println("\nMet Requirements:")
			for _, r := range resp.Requirements {
				fmt.Printf("  ✔ %s\n", r)
			}
		}

		if len(resp.MissingDetails) > 0 {
			fmt.Println("\nMissing Acceptance Criteria:")
			for _, m := range resp.MissingDetails {
				fmt.Printf("  ✖ %s\n", m)
			}
		}

		return nil
	},
}

// COMMAND REGISTRATION & FLAG INITIALIZATION

func init() {
	prSuggestionsCmd.Flags().StringVar(&prURLFlag, "pr-url", "", "Pull request URL")
	prSuggestionsCmd.Flags().IntVar(&prNumberFlag, "pr-number", 0, "Pull request number")
	prSuggestionsCmd.Flags().StringVar(&prRepoIDFlag, "repo-id", "", "Repository ID")
	prSuggestionsCmd.Flags().StringVar(&prSeverityFilter, "severity", "", "Comma-separated severity filter")
	prSuggestionsCmd.Flags().StringVar(&prCategoryFilter, "category", "", "Comma-separated category filter")
	prSuggestionsCmd.Flags().StringVar(&prFieldsFlag, "fields", "", "Select response fields (JSON/agent mode only)")

	prBusinessValidationCmd.Flags().StringVar(&taskURLFlag, "task-url", "", "Task URL to validate against (Linear / Jira)")
	prBusinessValidationCmd.Flags().StringVar(&taskIDFlag, "task-id", "", "Task key (e.g. KC-1441)")
	prBusinessValidationCmd.Flags().BoolVarP(&prBVStagedFlag, "staged", "s", false, "Use only staged git changes")
	prBusinessValidationCmd.Flags().StringVarP(&prBVCommitFlag, "commit", "c", "", "Use diff from commit SHA")
	prBusinessValidationCmd.Flags().StringVarP(&prBVBranchFlag, "branch", "b", "", "Compare against base branch")
	prBusinessValidationCmd.Flags().BoolVar(&prBVDryRunFlag, "dry-run", false, "Print payload without calling API")

	prCmd.AddCommand(prSuggestionsCmd)
	prCmd.AddCommand(prBusinessValidationCmd)
}
