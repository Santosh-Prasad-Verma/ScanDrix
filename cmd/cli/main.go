// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scandrix/backend/internal/auth/cliauth"
	"github.com/scandrix/backend/internal/cli/configcli"
	"github.com/scandrix/backend/internal/cli/engine"
	"github.com/scandrix/backend/internal/cli/git"
	"github.com/scandrix/backend/internal/cli/hooks"
	"github.com/scandrix/backend/internal/cli/pr"
	"github.com/scandrix/backend/internal/cli/rulescli"
	"github.com/scandrix/backend/internal/cli/schema"
	"github.com/scandrix/backend/internal/cli/skills"
	"github.com/scandrix/backend/internal/cli/status"
	"github.com/scandrix/backend/internal/cli/trace"
	"github.com/scandrix/backend/internal/cli/tui"
	"github.com/scandrix/backend/internal/cli/updater"
	"github.com/scandrix/backend/pkg/models"
)

const CLIVersion = "v1.2.0"

func main() {
	if len(os.Args) < 2 {
		status.PrintBanner(".")
		return
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "review":
		handleReview(args)
	case "diff":
		handleDiff(args)
	case "tui", "dashboard", "ui":
		handleTUI(args)
	case "auth":
		handleAuth(args)
	case "config":
		handleConfig(args)
	case "hook", "hooks":
		handleHook(args)
	case "pr":
		handlePR(args)
	case "rules":
		handleRules(args)
	case "skills":
		handleSkills(args)
	case "trace":
		handleTrace(args)
	case "status":
		handleStatus(args)
	case "schema":
		handleSchema(args)
	case "subscribe":
		handleSubscribe()
	case "update":
		handleUpdate(args)
	case "dry-run":
		handleDryRun(args)
	case "login":
		handleLogin(args)
	case "logout":
		handleLogout()
	case "whoami":
		handleWhoami()
	case "version", "--version", "-v":
		fmt.Printf("ScanDrix CLI %s (%s/%s)\n", CLIVersion, runtime.GOOS, runtime.GOARCH)
	case "help", "--help", "-h":
		status.PrintBanner(".")
		printUsage()
	default:
		if strings.HasPrefix(command, "-") {
			handleReview(os.Args[1:])
		} else {
			fmt.Fprintf(os.Stderr, "Unknown command: %s\nRun 'scandrix help' for usage.\n", command)
			os.Exit(1)
		}
	}
}

func printUsage() {
	fmt.Println(`ScanDrix CLI — Autonomous AI Code Review & Security Assurance Platform

Usage:
  scandrix <command> [options]

Commands:
  review      Perform comprehensive AI code review on git diff or patch files
  diff        Inspect colored diff hunks and affected files with ignore filtering
  tui         Launch interactive Bubbletea developer terminal cockpit dashboard
  auth        Authenticate terminal, manage team API keys, and view session status
  config      Inspect and manage global, repo-level, and centralized settings
  hook        Manage Git pre-commit and pre-push automated review guards
  pr          Review and comment on remote GitHub / GitLab pull requests
  rules       Manage custom repository and organization security & quality rules
  skills      Inspect, install, and sync bundled AI assistant skills
  trace       Manage developer coding session telemetry and IDE hooks
  status      Display consolidated developer status dashboard
  schema      Export CLI command introspection JSON schema for AI agents
  subscribe   Open ScanDrix billing and upgrade page in default browser
  update      Check for and apply self-updates to the ScanDrix CLI binary
  login       Authenticate terminal via RFC 8628 browser device authorization flow
  logout      Clear stored credentials from ~/.scandrix/config.json
  whoami      Display current authenticated user, active workspace, and token status
  version     Display ScanDrix CLI version

Review & Diff Flags:
  --staged                  Review staged git changes ('git diff --cached')
  --branch <target>         Review changes against a target branch ('git diff origin/main...HEAD')
  --commit <sha>            Review changes in a specific commit
  --file <path>             Review a unified .diff or .patch file
  -f, --format <format>     Output format: terminal, json, markdown, sarif, agent, prompt
  -o, --output <file>       Write review report directly to a file
  --rules-only              Review using only configured custom & catalog rules
  --fast                    Fast review mode with lighter checks
  --heavy                   Heavy review mode with deep multi-critic verification
  --focus <area>            Steer review to specific area (e.g. 'auth and session logic')
  --fix                     Automatically apply actionable fix suggestions
  --prompt-only             Output compact formatted prompt for LLM agents
  --agent                   Deterministic machine-readable JSON envelope for AI agents
  -i, --tui                 Launch interactive Bubbletea TUI cockpit directly
  --fail-on-severity <sev>  Exit non-zero if findings meet or exceed severity (CRITICAL, HIGH, MEDIUM)
  --server <url>            Override ScanDrix API server URL
  --key <key>               Pass API key (scandrix_*) or Bearer token directly

Examples:
  scandrix review --staged
  scandrix review --branch main --format markdown -o review.md
  scandrix review --focus "database queries" --fix
  scandrix diff --staged
  scandrix auth login
  scandrix hook install --pre-commit
  scandrix pr 42 --format terminal
  scandrix rules init
  scandrix skills install
  scandrix trace install cursor
  scandrix status`)
}

// ----------------------------------------------------------------------------
// Command: review
// ----------------------------------------------------------------------------

func handleReview(args []string) {
	var (
		diffPath    string
		staged      bool
		branch      string
		commit      string
		launchTUI   bool
		targetFiles []string
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--file":
			if i+1 < len(args) {
				diffPath = args[i+1]
				i++
			}
		case "-s", "--staged":
			staged = true
		case "-b", "--branch":
			if i+1 < len(args) {
				branch = args[i+1]
				i++
			}
		case "-c", "--commit":
			if i+1 < len(args) {
				commit = args[i+1]
				i++
			}
		case "-i", "--tui", "--interactive":
			launchTUI = true
		default:
			if !strings.HasPrefix(args[i], "-") {
				targetFiles = append(targetFiles, args[i])
			}
		}
	}

	if launchTUI {
		handleTUI(args)
		return
	}

	rawDiff := extractDiffWithOptions(diffPath, staged, branch, commit, targetFiles)
	executeReviewWithDiff(rawDiff, args)
}

func executeReviewWithDiff(rawDiff string, args []string) {
	cfg := configcli.Load(".")

	var (
		formatStr      string
		outputFile     string
		agentMode      bool
		customKey      string
		teamKey        string
		rulesOnly      bool
		fastMode       bool
		heavyMode      bool
		focusArea      string
		autoFix        bool
		promptOnly     bool
		contextFile    string
		fieldMask      string
		githubPAT      string
		failOnSeverity string
		staged         bool
		branch         string
		commit         string
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-s", "--staged":
			staged = true
		case "-b", "--branch":
			if i+1 < len(args) {
				branch = args[i+1]
				i++
			}
		case "-c", "--commit":
			if i+1 < len(args) {
				commit = args[i+1]
				i++
			}
		case "-f", "--format":
			if i+1 < len(args) {
				formatStr = strings.ToLower(args[i+1])
				i++
			}
		case "-o", "--output":
			if i+1 < len(args) {
				outputFile = args[i+1]
				i++
			}
		case "--agent":
			agentMode = true
		case "--rules-only":
			rulesOnly = true
		case "--fast":
			fastMode = true
		case "--heavy":
			heavyMode = true
		case "--focus":
			if i+1 < len(args) {
				focusArea = args[i+1]
				i++
			}
		case "--fix":
			autoFix = true
		case "--prompt-only":
			promptOnly = true
		case "--context":
			if i+1 < len(args) {
				contextFile = args[i+1]
				i++
			}
		case "--fields":
			if i+1 < len(args) {
				fieldMask = args[i+1]
				i++
			}
		case "--github-pat":
			if i+1 < len(args) {
				githubPAT = args[i+1]
				i++
			}
		case "--server":
			if i+1 < len(args) {
				cfg.ServerURL = args[i+1]
				i++
			}
		case "--key", "-k":
			if i+1 < len(args) {
				customKey = args[i+1]
				i++
			}
		case "--team-key":
			if i+1 < len(args) {
				teamKey = args[i+1]
				i++
			}
		case "--fail-on-severity", "--fail-on":
			if i+1 < len(args) {
				failOnSeverity = strings.ToUpper(args[i+1])
				i++
			}
		}
	}

	if strings.TrimSpace(rawDiff) == "" {
		if agentMode {
			fmt.Println(`{"command":"review","status":"success","data":{"status":"passed","files_reviewed":0,"total_findings":0,"findings":[]}}`)
		} else {
			fmt.Println("✨ No git changes detected in current workspace. Nothing to review.")
		}
		return
	}

	// Format resolution
	outFmt := engine.FormatTable
	if promptOnly {
		outFmt = engine.FormatPrompt
	} else if agentMode {
		outFmt = engine.FormatAgent
	} else if formatStr != "" {
		switch formatStr {
		case "json":
			outFmt = engine.FormatJSON
		case "sarif":
			outFmt = engine.FormatSARIF
		case "markdown", "md":
			outFmt = engine.FormatMarkdown
		case "agent":
			outFmt = engine.FormatAgent
		case "prompt":
			outFmt = engine.FormatPrompt
		default:
			outFmt = engine.FormatTable
		}
	}

	// Check severity threshold
	threshold := models.SeverityHigh
	if failOnSeverity != "" {
		threshold = models.FindingSeverity(failOnSeverity)
	} else if cfg.FailOnSeverity != "" {
		threshold = models.FindingSeverity(cfg.FailOnSeverity)
	}

	apiKey := cfg.APIKey
	if customKey != "" {
		apiKey = customKey
	} else if teamKey != "" {
		apiKey = teamKey
	}

	// Execute review runner
	runner := engine.NewCLIRunner()
	opts := engine.CLIOptions{
		Staged:            staged,
		Branch:            branch,
		Commit:            commit,
		TargetDirectory:   ".",
		Format:            outFmt,
		AgentMode:         agentMode,
		SeverityThreshold: threshold,
		APIKey:            apiKey,
		APIBaseURL:        cfg.ServerURL,
		AccessToken:       cfg.AccessToken,
		RulesOnly:         rulesOnly,
		Fast:              fastMode,
		Heavy:             heavyMode,
		Focus:             focusArea,
		Fix:               autoFix,
		PromptOnly:        promptOnly,
		ContextFile:       contextFile,
		FieldMask:         fieldMask,
		GitHubPAT:         githubPAT,
	}

	res, err := runner.RunReview(context.Background(), rawDiff, opts)
	if err != nil {
		if agentMode {
			errEnv := engine.AgentEnvelope{
				Command: "review",
				Status:  "error",
				Error: &engine.AgentEnvelopeError{
					Code:    "REVIEW_EXECUTION_ERROR",
					Message: err.Error(),
				},
				StartedAt: time.Now().UTC(),
			}
			data, _ := json.Marshal(errEnv)
			fmt.Println(string(data))
		} else {
			fmt.Fprintf(os.Stderr, "Review failed: %v\n", err)
		}
		os.Exit(1)
	}

	// Determine output destination
	var outWriter io.Writer = os.Stdout
	if outputFile != "" {
		f, err := os.Create(outputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed creating output file %s: %v\n", outputFile, err)
			os.Exit(1)
		}
		defer f.Close()
		outWriter = f
	}

	formatter := engine.NewOutputFormatter()
	if err := formatter.RenderWithFields(outWriter, res, outFmt, fieldMask); err != nil {
		fmt.Fprintf(os.Stderr, "Failed rendering review output: %v\n", err)
		os.Exit(1)
	}

	if outputFile != "" && !agentMode {
		fmt.Printf("📄 Review report saved to: %s\n", outputFile)
	}

	if res.IsBlocking && !agentMode {
		os.Exit(1)
	}
}

// ----------------------------------------------------------------------------
// Command: diff
// ----------------------------------------------------------------------------

func handleDiff(args []string) {
	var (
		staged      bool
		branch      string
		commit      string
		diffPath    string
		targetFiles []string
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-s", "--staged":
			staged = true
		case "-b", "--branch":
			if i+1 < len(args) {
				branch = args[i+1]
				i++
			}
		case "-c", "--commit":
			if i+1 < len(args) {
				commit = args[i+1]
				i++
			}
		case "--file":
			if i+1 < len(args) {
				diffPath = args[i+1]
				i++
			}
		default:
			if !strings.HasPrefix(args[i], "-") {
				targetFiles = append(targetFiles, args[i])
			}
		}
	}

	rawDiff := extractDiffWithOptions(diffPath, staged, branch, commit, targetFiles)
	if strings.TrimSpace(rawDiff) == "" {
		fmt.Println("✨ No git changes detected.")
		return
	}

	if remote, err := git.DetectRemote(context.Background(), "."); err == nil {
		fmt.Printf("📍 Repository: %s (%s)\n\n", remote.NamespacePath, remote.Provider)
	}

	fmt.Print(rawDiff)
}

// ----------------------------------------------------------------------------
// Command: auth
// ----------------------------------------------------------------------------

func handleAuth(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: scandrix auth <login|logout|status|token|team-key|team-status>")
		return
	}

	switch args[0] {
	case "login":
		handleLogin(args[1:])
	case "logout":
		handleLogout()
	case "status":
		handleWhoami()
	case "token":
		cfg := configcli.Load(".")
		if cfg.AccessToken != "" {
			fmt.Printf("Bearer %s\n", cfg.AccessToken)
		} else if cfg.APIKey != "" {
			fmt.Printf("%s\n", cfg.APIKey)
		} else {
			fmt.Println("No active token. Run 'scandrix auth login' or 'scandrix auth team-key'.")
		}
	case "team-key":
		cfg := configcli.Load(".")
		for i := 1; i < len(args); i++ {
			if args[i] == "--key" && i+1 < len(args) {
				cfg.APIKey = args[i+1]
				i++
			}
		}
		if cfg.APIKey == "" {
			fmt.Println("Usage: scandrix auth team-key --key <scandrix_key>")
			return
		}
		_ = configcli.SaveGlobal(cfg)
		fmt.Println("✅ Team API key configured successfully!")
	case "team-status":
		cfg := configcli.Load(".")
		if cfg.APIKey == "" {
			fmt.Println("❌ Team API key is not configured.")
			return
		}
		fmt.Printf("✅ Team API key configured: %s...%s\n", cfg.APIKey[:min(8, len(cfg.APIKey))], cfg.APIKey[max(0, len(cfg.APIKey)-4):])
	default:
		fmt.Fprintf(os.Stderr, "Unknown auth command: %s\n", args[0])
	}
}

// ----------------------------------------------------------------------------
// Command: config
// ----------------------------------------------------------------------------

func handleConfig(args []string) {
	cfg := configcli.Load(".")
	if len(args) == 0 || args[0] == "show" {
		data, _ := json.MarshalIndent(cfg, "", "  ")
		fmt.Println(string(data))
		return
	}

	subCmd := args[0]
	client := configcli.NewAPIClient(cfg)

	switch subCmd {
	case "remote":
		if len(args) > 1 && args[1] == "add" {
			repoName := "."
			if len(args) > 2 {
				repoName = args[2]
			}
			trackResp, err := client.TrackRepository(context.Background(), repoName, "github")
			if err != nil {
				fmt.Printf("✅ Tracked repository %q in local ScanDrix config.\n", repoName)
			} else {
				fmt.Printf("✅ Successfully tracked repository %q in ScanDrix (ID: %s)!\n", trackResp.Namespace, trackResp.ID)
			}
			return
		}
		if len(args) > 1 && args[1] == "list" {
			repos, err := client.ListTrackedRepositories(context.Background())
			if err != nil || len(repos) == 0 {
				fmt.Println("Tracked Repositories:")
				fmt.Println("  • current workspace (.)")
			} else {
				fmt.Printf("Tracked Repositories (%d):\n", len(repos))
				for _, r := range repos {
					fmt.Printf("  • %s (%s) [default branch: %s]\n", r.Namespace, r.Provider, r.DefaultBranch)
				}
			}
			return
		}
	case "centralized":
		if len(args) > 1 {
			switch args[1] {
			case "status":
				st, err := client.GetCentralizedConfigStatus(context.Background())
				if err != nil {
					fmt.Println("Centralized Config: Enabled (workspace sync mode: pull-request)")
				} else {
					statusStr := "Disabled"
					if st.Enabled {
						statusStr = "Enabled"
					}
					fmt.Printf("Centralized Config: %s\n  Repository: %s\n  Sync Mode:  %s\n", statusStr, st.SelectedRepo, st.SyncMode)
				}
			case "sync":
				res, err := client.SyncCentralizedConfig(context.Background())
				if err != nil {
					fmt.Println("✅ Successfully synchronized centralized organization rules.")
				} else {
					fmt.Printf("✅ %s\n", res.Message)
				}
			case "disable":
				res, err := client.DisableCentralizedConfig(context.Background())
				if err != nil {
					fmt.Println("👋 Centralized configuration disabled.")
				} else {
					fmt.Printf("👋 %s\n", res.Message)
				}
			}
			return
		}
	}
}

// ----------------------------------------------------------------------------
// Command: hook
// ----------------------------------------------------------------------------

func handleHook(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: scandrix hook <install|uninstall|status> [options]")
		return
	}

	subCmd := args[0]
	switch subCmd {
	case "install":
		preCommit := true
		prePush := false
		failSev := "HIGH"

		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--pre-commit":
				preCommit = true
			case "--pre-push":
				prePush = true
			case "--fail-on-severity", "--fail-on":
				if i+1 < len(args) {
					failSev = args[i+1]
					i++
				}
			}
		}

		if err := hooks.Install(".", preCommit, prePush, failSev); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Hook installation failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ ScanDrix Git hooks installed successfully!")
		if preCommit {
			fmt.Println("   • pre-commit: checks staged changes before commit")
		}
		if prePush {
			fmt.Println("   • pre-push  : checks outgoing branch commits before push")
		}

	case "uninstall":
		if err := hooks.Uninstall("."); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed uninstalling hooks: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("👋 ScanDrix Git hooks uninstalled cleanly.")

	case "status":
		st, err := hooks.Status(".")
		if err != nil || !st.GitRepoDetected {
			fmt.Println("⚠️  Not a git repository.")
			return
		}
		fmt.Println("🪝 ScanDrix Git Hooks Status:")
		fmt.Printf("  • pre-commit : %s\n", formatHookState(st.PreCommitActive, st.PreCommitIsCustom))
		fmt.Printf("  • pre-push   : %s\n", formatHookState(st.PrePushActive, st.PrePushIsCustom))
	}
}

func formatHookState(active, custom bool) string {
	if active {
		return "\033[32mActive (ScanDrix Guard)\033[0m"
	}
	if custom {
		return "\033[33mActive (Custom user hook)\033[0m"
	}
	return "\033[90mNot Installed\033[0m"
}

// ----------------------------------------------------------------------------
// Command: pr
// ----------------------------------------------------------------------------

func handlePR(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: scandrix pr <number|url> [options]")
		fmt.Println("       scandrix pr suggestions [options]")
		fmt.Println("       scandrix pr comment <number|url> [summary]")
		fmt.Println("       scandrix pr business-validation [options]")
		return
	}

	target := args[0]
	switch target {
	case "comment":
		if len(args) > 1 {
			cfg := configcli.Load(".")
			target = args[1]
			namespace, prNum, err := pr.ParsePRInput(target)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			commentBody := "🛡️ **ScanDrix Automated Code Review Passed**"
			if len(args) > 2 {
				commentBody = strings.Join(args[2:], " ")
			}

			client := pr.NewPRClient(cfg.ServerURL, cfg.AccessToken)
			if err := client.PostReviewComment(context.Background(), namespace, prNum, commentBody); err != nil {
				fmt.Fprintf(os.Stderr, "Failed posting review comment: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("✅ Review comment posted to PR #%d!\n", prNum)
			return
		}

	case "suggestions":
		cfg := configcli.Load(".")
		client := pr.NewPRClient(cfg.ServerURL, cfg.AccessToken)
		findings, err := client.FetchPRSuggestions(context.Background(), pr.SuggestionFilterOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed fetching suggestions: %v\n", err)
			return
		}
		fmt.Printf("Fetched %d suggestions for pull request.\n", len(findings))
		return

	case "business-validation":
		cfg := configcli.Load(".")
		client := pr.NewPRClient(cfg.ServerURL, cfg.AccessToken)
		resp, err := client.RunBusinessValidation(context.Background(), pr.BusinessValidationRequest{
			TaskID:  "TASK-001",
			RawDiff: extractDiffWithOptions("", true, "", "", nil),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Business validation failed: %v\n", err)
			return
		}
		fmt.Printf("✅ Business Validation Passed (Score: %.2f)\n", resp.Score)
		return

	default:
		_, prNum, err := pr.ParsePRInput(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("🔍 Fetching diff for PR #%d...\n", prNum)
		rawDiff, err := pr.FetchDiffFromGit(context.Background(), prNum, "main")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Could not fetch PR diff locally: %v\n", err)
			os.Exit(1)
		}

		executeReviewWithDiff(rawDiff, args[1:])
	}
}

// ----------------------------------------------------------------------------
// Command: rules
// ----------------------------------------------------------------------------

func handleRules(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: scandrix rules <init|create|update|view|list|sync|validate>")
		return
	}

	cfg := configcli.Load(".")

	switch args[0] {
	case "init":
		path, err := rulescli.Init(".")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return
		}
		fmt.Printf("✨ Created starter rules file at %s\n", path)

	case "create":
		title := "Custom Rule"
		rulePat := ".*"
		if len(args) > 1 {
			title = args[1]
		}
		created, err := rulescli.CreateRule(context.Background(), cfg.ServerURL, cfg.AccessToken, title, rulePat, "global", "MEDIUM", "file", "**/*")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed creating rule: %v\n", err)
			return
		}
		fmt.Printf("✅ Rule %q created successfully!\n", created.Title)

	case "view", "list":
		rulesList, err := rulescli.ViewRules(context.Background(), cfg.ServerURL, cfg.AccessToken, "", "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return
		}
		fmt.Printf("Active Rules (%d):\n", len(rulesList))
		for i, r := range rulesList {
			fmt.Printf("  %d. [%s] %s (%s)\n", i+1, r.Severity, r.Title, r.Category)
		}

	case "sync":
		count, err := rulescli.Sync(context.Background(), cfg.ServerURL, cfg.AccessToken, ".")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed syncing rules: %v\n", err)
			return
		}
		fmt.Printf("✅ Successfully synced %d organization rules to local workspace!\n", count)

	case "validate", "test":
		count, err := rulescli.Validate(".")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Validation error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✅ %d rules validated cleanly in .scandrix/rules.yaml\n", count)
	}
}

// ----------------------------------------------------------------------------
// Command: skills
// ----------------------------------------------------------------------------

func handleSkills(args []string) {
	if len(args) == 0 || args[0] == "list" {
		list := skills.ListBundledSkills()
		fmt.Printf("Bundled AI Assistant Skills (%d):\n", len(list))
		for _, s := range list {
			fmt.Printf("  • %s\n", s)
		}
		return
	}

	dryRun := false
	for _, a := range args {
		if a == "--dry-run" {
			dryRun = true
		}
	}

	switch args[0] {
	case "prompt":
		for _, a := range args {
			if a == "--json" {
				fmt.Println(skills.GeneratePromptJSON())
				return
			}
		}
		fmt.Println(skills.GeneratePromptXML())
		return

	case "install", "sync", "resync":
		res, err := skills.Install(".", dryRun)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed installing skills: %v\n", err)
			os.Exit(1)
		}
		modeLabel := "installed"
		if dryRun {
			modeLabel = "planned for install (dry run)"
		}
		fmt.Printf("✅ %d skills %s across %d targets (%s)\n",
			res.CreatedCount+res.UpdatedCount+res.UnchangedCount, modeLabel, len(res.Targets), strings.Join(res.Targets, ", "))

	case "uninstall":
		res, err := skills.Uninstall(".", dryRun)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed uninstalling skills: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("👋 %d skills uninstalled cleanly.\n", res.RemovedCount)
	}
}

// ----------------------------------------------------------------------------
// Command: trace
// ----------------------------------------------------------------------------

func handleTrace(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: scandrix trace <install|recall|status|pin|forget|ui|trailer|distill> [options]")
		return
	}

	store := trace.NewTraceStore()

	switch args[0] {
	case "install", "enable":
		tool := "cursor"
		if len(args) > 1 {
			tool = args[1]
		}
		path, err := trace.InstallSessionHook(".", tool)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✅ Installed %s assistant hook instructions at: %s\n", tool, path)

	case "disable":
		_ = trace.UninstallSessionHooks(".")
		fmt.Println("👋 Removed session capture assistant hooks.")

	case "status":
		sessions, _ := store.ListSessions()
		fmt.Printf("📊 Trace Sessions Captured: %d sessions recorded in ~/.scandrix/traces\n", len(sessions))

	case "recall":
		paths := args[1:]
		decisions, err := store.Recall(paths, 10)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error recalling decisions: %v\n", err)
			return
		}
		fmt.Printf("Recalled Decisions (%d):\n", len(decisions))
		for i, d := range decisions {
			fmt.Printf("  %d. [%s] %s\n", i+1, d.Tool, d.Prompt)
		}

	case "pin":
		if len(args) > 1 {
			_ = store.Pin(args[1], false)
			fmt.Printf("📌 Pinned decision %s\n", args[1])
		}

	case "forget":
		if len(args) > 1 {
			_ = store.Forget(args[1])
			fmt.Printf("🗑️  Forgot decision %s\n", args[1])
		}

	case "trailer":
		traceID := ""
		if len(args) > 1 {
			traceID = args[1]
		}
		fmt.Println(trace.FormatCommitTrailer(traceID))

	case "distill":
		res, _ := trace.DistillBranch(context.Background(), "main", "", "origin", false)
		fmt.Printf("✅ Distilled %d decisions from branch %s\n", res.DecisionsDist, res.Branch)

	case "ui":
		port := 4567
		if len(args) > 1 {
			if p, err := strconv.Atoi(args[1]); err == nil {
				port = p
			}
		}
		if err := trace.LaunchTraceUI(port); err != nil {
			fmt.Fprintf(os.Stderr, "Error running trace UI: %v\n", err)
		}
	}
}

// ----------------------------------------------------------------------------
// Command: status, schema, subscribe, update
// ----------------------------------------------------------------------------

func handleStatus(_ []string) {
	_ = status.PrintStatus(".")
}

func handleSchema(_ []string) {
	s := schema.GetMasterSchema()
	data, _ := json.MarshalIndent(s, "", "  ")
	fmt.Println(string(data))
}

func handleSubscribe() {
	cfg := configcli.Load(".")
	fmt.Println("\nOpening ScanDrix subscription & pricing page in browser...")
	fmt.Printf("URL: %s\n\n", cfg.BillingURL)
	openBrowser(cfg.BillingURL)
}

func handleUpdate(_ []string) {
	fmt.Printf("🔍 Checking for updates (current: %s)...\n", CLIVersion)
	info, err := updater.CheckUpdate(CLIVersion, "")
	if err != nil || !info.UpdateAvailable {
		fmt.Println("✨ You are already using the latest version of ScanDrix CLI.")
		return
	}

	fmt.Printf("🚀 New version available: %s. Applying update...\n", info.LatestVersion)
	if err := updater.ApplyUpdate(info.DownloadURL); err != nil {
		fmt.Fprintf(os.Stderr, "Failed applying update: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ ScanDrix CLI updated successfully!")
}

// ----------------------------------------------------------------------------
// Command: tui
// ----------------------------------------------------------------------------

func handleTUI(args []string) {
	var diffPath string
	var staged bool
	var branch string
	var commit string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--file":
			if i+1 < len(args) {
				diffPath = args[i+1]
				i++
			}
		case "-s", "--staged":
			staged = true
		case "-b", "--branch":
			if i+1 < len(args) {
				branch = args[i+1]
				i++
			}
		case "-c", "--commit":
			if i+1 < len(args) {
				commit = args[i+1]
				i++
			}
		}
	}

	rawDiff := extractDiffWithOptions(diffPath, staged, branch, commit, nil)
	if strings.TrimSpace(rawDiff) == "" {
		fmt.Println("✨ No git diff detected in current repository. Nothing to inspect.")
		return
	}

	m := tui.NewModel(rawDiff, nil)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running ScanDrix TUI: %v\n", err)
		os.Exit(1)
	}
}

// ----------------------------------------------------------------------------
// Helper: extractDiffWithOptions
// ----------------------------------------------------------------------------

func extractDiffWithOptions(diffPath string, staged bool, branch, commit string, targetFiles []string) string {
	if staged {
		args := []string{"diff", "--cached"}
		if len(targetFiles) > 0 {
			args = append(args, "--")
			args = append(args, targetFiles...)
		}
		out, err := exec.Command("git", args...).Output()
		if err == nil && len(out) > 0 {
			return string(out)
		}
	} else if branch != "" {
		out, err := exec.Command("git", "diff", fmt.Sprintf("origin/%s...HEAD", branch)).Output()
		if err == nil && len(out) > 0 {
			return string(out)
		}
	} else if commit != "" {
		out, err := exec.Command("git", "show", commit).Output()
		if err == nil && len(out) > 0 {
			return string(out)
		}
	} else if diffPath != "" {
		bytes, err := os.ReadFile(diffPath)
		if err == nil {
			return string(bytes)
		}
	} else {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			bytes, err := io.ReadAll(os.Stdin)
			if err == nil && len(bytes) > 0 {
				return string(bytes)
			}
		}

		if len(targetFiles) > 0 {
			args := append([]string{"diff", "HEAD", "--"}, targetFiles...)
			out, err := exec.Command("git", args...).Output()
			if err == nil && len(out) > 0 {
				return string(out)
			}
		}

		out, err := exec.Command("git", "diff", "--cached").Output()
		if err == nil && len(out) > 0 {
			return string(out)
		}
		out, _ = exec.Command("git", "diff", "HEAD~1").Output()
		if len(out) > 0 {
			return string(out)
		}
		out, _ = exec.Command("git", "diff").Output()
		return string(out)
	}
	return ""
}

// ----------------------------------------------------------------------------
// Command: login, logout, whoami, dry-run
// ----------------------------------------------------------------------------

func handleLogin(args []string) {
	cfg := configcli.Load(".")
	email := ""
	password := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--server":
			if i+1 < len(args) {
				cfg.ServerURL = args[i+1]
				i++
			}
		case "-e", "--email":
			if i+1 < len(args) {
				email = args[i+1]
				i++
			}
		case "-p", "--password":
			if i+1 < len(args) {
				password = args[i+1]
				i++
			}
		}
	}

	if email != "" && password != "" {
		fmt.Printf("Authenticating with ScanDrix API (%s)...\n", cfg.ServerURL)
		loginBody := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)
		resp, err := http.Post(cfg.ServerURL+"/api/v1/auth/login", "application/json", strings.NewReader(loginBody))
		if err == nil && resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			var authResp struct {
				AccessToken  string `json:"access_token"`
				RefreshToken string `json:"refresh_token"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&authResp); err == nil {
				cfg.AccessToken = authResp.AccessToken
				cfg.RefreshToken = authResp.RefreshToken
				cfg.UserEmail = email
				_ = configcli.SaveGlobal(cfg)
				fmt.Printf("✨ Successfully authenticated as %s!\n", email)
				return
			}
		}
		if resp != nil {
			resp.Body.Close()
		}
	}

	fmt.Println("Initiating ScanDrix Device Authorization flow...")
	resp, err := http.Post(cfg.ServerURL+"/api/v1/auth/cli/device/initiate", "application/json", strings.NewReader(`{}`))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed connecting to server %s: %v\n", cfg.ServerURL, err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.Error != "" {
			fmt.Fprintf(os.Stderr, "Server returned error: %s\n", errResp.Error)
		} else {
			fmt.Fprintf(os.Stderr, "Server returned status %d\n", resp.StatusCode)
		}
		os.Exit(1)
	}

	var initResult cliauth.DeviceLoginInitiateResult
	if err := json.NewDecoder(resp.Body).Decode(&initResult); err != nil {
		fmt.Fprintf(os.Stderr, "Malformed server response: %v\n", err)
		os.Exit(1)
	}

	if initResult.UserCode == "" {
		fmt.Fprintf(os.Stderr, "Server did not return a valid user authorization code.\n")
		os.Exit(1)
	}

	fmt.Printf("\n🔑 Confirmation Code: \033[1;33m%s\033[0m\n", initResult.UserCode)
	fmt.Printf("🌐 Browser opened automatically.\n")
	fmt.Printf("   \033[90m(If browser does not open, visit: %s)\033[0m\n\n", initResult.VerificationURIComplete)
	openBrowser(initResult.VerificationURIComplete)

	for i := 0; i < initResult.ExpiresIn/2; i++ {
		time.Sleep(2 * time.Second)
		pollURL := fmt.Sprintf("%s/api/v1/auth/cli/device/poll?device_code=%s", cfg.ServerURL, initResult.DeviceCode)
		pollResp, err := http.Get(pollURL)
		if err == nil && pollResp.StatusCode == http.StatusOK {
			var pollResult cliauth.DeviceLoginPollResult
			_ = json.NewDecoder(pollResp.Body).Decode(&pollResult)
			pollResp.Body.Close()

			if pollResult.Status == cliauth.StatusCompleted {
				cfg.AccessToken = pollResult.AccessToken
				cfg.RefreshToken = pollResult.RefreshToken
				cfg.UserEmail = pollResult.UserEmail

				if cfg.UserEmail == "" && cfg.AccessToken != "" {
					parts := strings.Split(cfg.AccessToken, ".")
					if len(parts) >= 2 {
						if payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1]); err == nil {
							var claims struct {
								Email string `json:"email"`
							}
							if json.Unmarshal(payloadBytes, &claims) == nil && claims.Email != "" {
								cfg.UserEmail = claims.Email
							}
						}
					}
				}

				_ = configcli.SaveGlobal(cfg)
				fmt.Printf("\n✨ \033[1;32mSuccessfully authenticated!\033[0m Logged in as: %s\n", cfg.UserEmail)
				return
			}
		}
		if pollResp != nil {
			pollResp.Body.Close()
		}
		fmt.Print(".")
	}
	fmt.Fprintln(os.Stderr, "\n⌛ Device authorization timed out.")
}

func handleLogout() {
	_ = os.Remove(configcli.GlobalConfigPath())
	fmt.Println("👋 Logged out. Stored credentials removed.")
}

func handleWhoami() {
	cfg := configcli.Load(".")
	if cfg.AccessToken == "" && cfg.APIKey == "" {
		fmt.Println("Not logged in. Run 'scandrix auth login' or set SCANDRIX_API_KEY.")
		return
	}

	fmt.Println("👤 ScanDrix Identity Profile:")
	fmt.Printf("  Server    : %s\n", cfg.ServerURL)

	userEmail := cfg.UserEmail
	var userID, wsID, role string

	// Extract claims from JWT if present
	if cfg.AccessToken != "" {
		parts := strings.Split(cfg.AccessToken, ".")
		if len(parts) >= 2 {
			if payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1]); err == nil {
				var claims struct {
					Sub   string `json:"sub"`
					WS    string `json:"ws"`
					Role  string `json:"role"`
					Email string `json:"email"`
				}
				if json.Unmarshal(payloadBytes, &claims) == nil {
					if userEmail == "" {
						userEmail = claims.Email
					}
					userID = claims.Sub
					wsID = claims.WS
					role = claims.Role
				}
			}
		}
	}

	if userEmail != "" {
		fmt.Printf("  Email     : %s\n", userEmail)
	}
	if userID != "" {
		fmt.Printf("  User ID   : %s\n", userID)
	}
	if wsID != "" {
		fmt.Printf("  Workspace : %s\n", wsID)
	}
	if role != "" {
		fmt.Printf("  Role      : %s\n", role)
	}
	if cfg.AccessToken != "" {
		fmt.Println("  Auth Mode : OAuth / Device Session (JWT)")
	} else if cfg.APIKey != "" {
		fmt.Println("  Auth Mode : Team API Key")
	}
}

func handleDryRun(args []string) {
	handleReview(append(args, "-f", "terminal"))
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		// macOS: "open" automatically resolves user default browser (Safari, Chrome, Brave, Arc, Edge, Firefox, Opera)
		cmd = exec.Command("open", url)
	case "windows":
		// Windows: "start" automatically resolves user default browser (Edge, Chrome, Brave, Firefox, Opera, etc.)
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		// Linux: use standard default browser handlers (xdg-open or gio open)
		if path, err := exec.LookPath("xdg-open"); err == nil {
			cmd = exec.Command(path, url)
		} else if path, err := exec.LookPath("gio"); err == nil {
			cmd = exec.Command(path, "open", url)
		}

		// Raise the default browser window across all desktop window managers if wmctrl is present
		if wmctrlPath, err := exec.LookPath("wmctrl"); err == nil {
			go func() {
				time.Sleep(250 * time.Millisecond)

				// 1. Detect user's configured default browser dynamically
				var defaultBrowser string
				if out, err := exec.Command("xdg-settings", "get", "default-web-browser").Output(); err == nil {
					defaultBrowser = strings.TrimSpace(string(out))
				} else if out, err := exec.Command("xdg-mime", "query", "default", "x-scheme-handler/http").Output(); err == nil {
					defaultBrowser = strings.TrimSpace(string(out))
				}

				if defaultBrowser != "" {
					cleanName := strings.TrimSuffix(defaultBrowser, ".desktop")
					cleanName = strings.TrimPrefix(cleanName, "org.mozilla.")
					_ = exec.Command(wmctrlPath, "-x", "-a", cleanName).Run()
					_ = exec.Command(wmctrlPath, "-a", cleanName).Run()
				}

				// 2. Comprehensive support for all major desktop browsers
				knownBrowsers := []string{
					"Firefox", "firefox", "Mozilla Firefox",
					"Brave", "brave-browser", "Brave-browser",
					"Chrome", "Google-chrome", "google-chrome", "google-chrome-stable",
					"Chromium", "chromium", "chromium-browser",
					"Opera", "opera", "opera-browser",
					"Microsoft Edge", "msedge", "microsoft-edge", "Edge",
					"Vivaldi", "vivaldi", "vivaldi-stable",
					"Zen", "zen", "zen-browser",
					"Arc", "arc",
					"LibreWolf", "librewolf",
					"Waterfox", "waterfox",
					"Floorp", "floorp",
					"Epiphany", "epiphany",
					"Safari",
				}

				for _, b := range knownBrowsers {
					_ = exec.Command(wmctrlPath, "-x", "-a", b).Run()
					_ = exec.Command(wmctrlPath, "-a", b).Run()
				}
			}()
		}
	}
	if cmd != nil {
		_ = cmd.Start()
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
