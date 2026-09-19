// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/ci"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/spf13/cobra"
)

var (
	ciFailOnFlag       string
	ciTargetBranchFlag string
	ciDiffFileFlag     string
	ciSarifOutFlag     string
	ciJUnitOutFlag     string
	ciGitLabOutFlag    string
	ciSummaryOutFlag   string
	ciSkipFlagsFlag    bool
	ciTokenRepoFlag    string
	ciTokenOrgFlag     string
	ciTokenTTLFlag     string
	ciFormatToFlag     string
	ciFormatInputFlag  string
	ciFormatOutputFlag string
)

// ciCmd is the root command for CI/CD pipeline automation workflows.
var ciCmd = &cobra.Command{
	Use:   "ci",
	Short: "Automated CI/CD pipeline code review runner and reporter",
	Long: `Run headless code reviews inside GitHub Actions, GitLab CI, Azure Pipelines,
Bitbucket Pipelines, and Forgejo CI. Enforce quality gates, export SARIF/JUnit reports,
and generate scoped pipeline authentication tokens.`,
}

// ciRunCmd executes the headless review runner.
var ciRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Execute automated review and enforce quality gates",
	RunE: func(cmd *cobra.Command, args []string) error {
		runner := ci.NewHeadlessRunner()
		env := runner.Environment()

		if !quietFlag && formatFlag != "json" {
			fmt.Printf("🛡️  ScanDrix CI Reviewer running in: %s\n", env.Platform)
			if env.RepoOwner != "" && env.RepoName != "" {
				fmt.Printf("   Repository: %s/%s\n", env.RepoOwner, env.RepoName)
			}
			if env.CommitSHA != "" {
				sha := env.CommitSHA
				if len(sha) > 8 {
					sha = sha[:8]
				}
				fmt.Printf("   Commit:     %s\n", sha)
			}
			if env.IsPR {
				fmt.Printf("   Pull Req:   #%d\n", env.PullRequestID)
			}
		}

		cfg := ci.HeadlessReviewConfig{
			FailOnSeverity: ci.SeverityThreshold(ciFailOnFlag),
			TargetBranch:   ciTargetBranchFlag,
			DiffFilePath:   ciDiffFileFlag,
			OutputSARIF:    ciSarifOutFlag,
			OutputJUnit:    ciJUnitOutFlag,
			OutputGitLab:   ciGitLabOutFlag,
			OutputMarkdown: ciSummaryOutFlag,
			SkipCommitFlag: ciSkipFlagsFlag,
		}

		res, err := runner.RunHeadlessReview(context.Background(), cfg, nil)
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			envPayload := utils.BuildAgentSuccessEnvelope("ci run", res, time.Now())
			return utils.EmitAgentEnvelope(envPayload, outputFlag)
		}

		// Print concise console summary
		fmt.Println("\n" + ci.FormatPRSummaryMarkdown(res))

		if !res.GatePassed {
			fmt.Fprintf(os.Stderr, "\n❌ CI Quality Gate Failed: %s (Exit Code: %d)\n", res.GateReason, res.ExitCode)
			os.Exit(int(res.ExitCode))
		}

		fmt.Println("\n✅ CI Quality Gate Passed.")
		return nil
	},
}

// ciStatusCmd prints the detected CI environment.
var ciStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Inspect detected CI runner vendor and pipeline variables",
	RunE: func(cmd *cobra.Command, args []string) error {
		env := ci.DetectCIEnvironment()

		if agentFlag || formatFlag == "json" {
			payload := utils.BuildAgentSuccessEnvelope("ci status", env, time.Now())
			return utils.EmitAgentEnvelope(payload, outputFlag)
		}

		out := cmd.OutOrStdout()
		fmt.Fprintln(out, "=== ScanDrix CI Environment Detection ===")
		fmt.Fprintf(out, "Platform:       %s\n", env.Platform)
		fmt.Fprintf(out, "Repository:     %s/%s\n", env.RepoOwner, env.RepoName)
		fmt.Fprintf(out, "Commit SHA:     %s\n", env.CommitSHA)
		fmt.Fprintf(out, "Base Ref:       %s\n", env.BaseRef)
		fmt.Fprintf(out, "Head Ref:       %s\n", env.HeadRef)
		fmt.Fprintf(out, "Is PR:          %v (#%d)\n", env.IsPR, env.PullRequestID)
		fmt.Fprintf(out, "Job ID:         %s\n", env.JobID)
		fmt.Fprintf(out, "Run ID:         %s\n", env.RunID)
		fmt.Fprintf(out, "Workspace Path: %s\n", env.WorkspacePath)
		return nil
	},
}

// ciFormatCmd transforms findings JSON into standard formats.
var ciFormatCmd = &cobra.Command{
	Use:   "format",
	Short: "Transform findings JSON into SARIF, JUnit, GitLab, or Markdown",
	RunE: func(cmd *cobra.Command, args []string) error {
		if ciFormatInputFlag == "" {
			return fmt.Errorf("--input file path is required")
		}

		raw, err := os.ReadFile(ciFormatInputFlag)
		if err != nil {
			return fmt.Errorf("failed to read input file: %w", err)
		}

		var findings []ci.CIFinding
		if err := json.Unmarshal(raw, &findings); err != nil {
			return fmt.Errorf("invalid findings JSON: %w", err)
		}

		var output []byte
		switch ciFormatToFlag {
		case "sarif":
			output, err = ci.FormatSARIF(findings)
		case "junit":
			output, err = ci.FormatJUnitXML(findings, true, 1*time.Second)
		case "gitlab":
			output, err = ci.FormatGitLabCodeQuality(findings)
		case "markdown", "summary":
			res := &ci.HeadlessReviewResult{
				GatePassed:    true,
				Findings:      findings,
				TotalFindings: len(findings),
			}
			output = []byte(ci.FormatPRSummaryMarkdown(res))
		default:
			return fmt.Errorf("unsupported target format: %s (choose: sarif, junit, gitlab, markdown)", ciFormatToFlag)
		}

		if err != nil {
			return fmt.Errorf("format conversion failed: %w", err)
		}

		if ciFormatOutputFlag != "" {
			return os.WriteFile(ciFormatOutputFlag, output, 0644)
		}

		fmt.Println(string(output))
		return nil
	},
}

// ciTokenCmd issues scoped CI tokens for pipeline workflows.
var ciTokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Mint a scoped CI pipeline token for automated reviews",
	RunE: func(cmd *cobra.Command, args []string) error {
		secretKey := os.Getenv("SCANDRIX_CI_SIGNING_KEY")
		svc := ci.NewCITokenService(secretKey)

		var orgID uuid.UUID
		if ciTokenOrgFlag != "" {
			orgID, _ = uuid.Parse(ciTokenOrgFlag)
		}
		if orgID == uuid.Nil {
			orgID = uuid.New()
		}

		repoName := ciTokenRepoFlag
		if repoName == "" {
			env := ci.DetectCIEnvironment()
			if env.RepoOwner != "" && env.RepoName != "" {
				repoName = fmt.Sprintf("%s/%s", env.RepoOwner, env.RepoName)
			}
		}

		ttl, _ := time.ParseDuration(ciTokenTTLFlag)
		if ttl <= 0 {
			ttl = 2 * time.Hour
		}

		permissions := []string{"ci:review", "ci:checks", "ci:autofix"}
		tokenStr, claims, err := svc.IssueToken(orgID, repoName, repoName, permissions, ttl)
		if err != nil {
			return fmt.Errorf("failed to issue token: %w", err)
		}

		if agentFlag || formatFlag == "json" {
			data := map[string]any{
				"token":      tokenStr,
				"claims":     claims,
				"expires_at": time.Unix(claims.ExpiresAt, 0).Format(time.RFC3339),
			}
			payload := utils.BuildAgentSuccessEnvelope("ci token", data, time.Now())
			return utils.EmitAgentEnvelope(payload, outputFlag)
		}

		out := cmd.OutOrStdout()
		fmt.Fprintln(out, "🛡️  ScanDrix Scoped CI Pipeline Token Generated:")
		fmt.Fprintf(out, "Token:       %s\n", tokenStr)
		fmt.Fprintf(out, "Repository:  %s\n", claims.RepoFullName)
		fmt.Fprintf(out, "Expires:     %s\n", time.Unix(claims.ExpiresAt, 0).Format(time.RFC3339))
		fmt.Fprintf(out, "Usage:       export SCANDRIX_CI_TOKEN=\"%s\"\n", tokenStr)
		return nil
	},
}

func init() {
	ciRunCmd.Flags().StringVar(&ciFailOnFlag, "fail-on", "error", "Severity threshold that fails the quality gate (critical|error|warning|info|never)")
	ciRunCmd.Flags().StringVar(&ciTargetBranchFlag, "target-branch", "main", "Target base branch for PR diff")
	ciRunCmd.Flags().StringVar(&ciDiffFileFlag, "diff-file", "", "Explicit diff file to review")
	ciRunCmd.Flags().StringVar(&ciSarifOutFlag, "sarif", "", "Write SARIF v2.1.0 output to file path")
	ciRunCmd.Flags().StringVar(&ciJUnitOutFlag, "junit", "", "Write JUnit XML output to file path")
	ciRunCmd.Flags().StringVar(&ciGitLabOutFlag, "gitlab", "", "Write GitLab Code Quality JSON output to file path")
	ciRunCmd.Flags().StringVar(&ciSummaryOutFlag, "summary", "", "Write GitHub Step Summary Markdown to file path")
	ciRunCmd.Flags().BoolVar(&ciSkipFlagsFlag, "skip-flags", true, "Respect [skip scandrix] in commit messages")

	ciFormatCmd.Flags().StringVarP(&ciFormatInputFlag, "input", "i", "", "Input findings JSON file path")
	ciFormatCmd.Flags().StringVarP(&ciFormatToFlag, "to", "t", "sarif", "Target format: sarif, junit, gitlab, markdown")
	ciFormatCmd.Flags().StringVarP(&ciFormatOutputFlag, "output", "o", "", "Output destination file path")

	ciTokenCmd.Flags().StringVar(&ciTokenRepoFlag, "repo", "", "Target repository full name (e.g. org/repo)")
	ciTokenCmd.Flags().StringVar(&ciTokenOrgFlag, "org", "", "Organization UUID")
	ciTokenCmd.Flags().StringVar(&ciTokenTTLFlag, "ttl", "2h", "Token lifetime duration (e.g. 2h, 24h)")

	ciCmd.AddCommand(ciRunCmd)
	ciCmd.AddCommand(ciStatusCmd)
	ciCmd.AddCommand(ciFormatCmd)
	ciCmd.AddCommand(ciTokenCmd)
}
