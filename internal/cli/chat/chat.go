// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/scandrix/backend/internal/cli/configcli"
	"github.com/scandrix/backend/internal/cli/engine"
	"github.com/scandrix/backend/internal/cli/git"
	"github.com/scandrix/backend/internal/cli/hooks"
	"github.com/scandrix/backend/internal/cli/pr"
	"github.com/scandrix/backend/internal/cli/rulescli"
	"github.com/scandrix/backend/internal/cli/schema"
	"github.com/scandrix/backend/internal/cli/skills"
	"github.com/scandrix/backend/internal/cli/status"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/prompts"
	"github.com/scandrix/backend/internal/rules/catalog"
	"github.com/scandrix/backend/pkg/models"
)

// ChatSession manages the interactive terminal copilot loop.
type ChatSession struct {
	gateway       *llm.Gateway
	runner        *engine.CLIRunner
	history       []llm.ChatMessage
	activeModel   string
	activePersona ReviewerPersona
	workspacePath string
	displayPath   string
	gitBranch     string
	repoName      string
	userEmail     string
}

// StartInteractiveSession initializes and launches the natural scrolling CLI session.
func StartInteractiveSession(targetDir string, initialPrompt string) {
	if targetDir == "" {
		targetDir = "."
	}

	cfg := configcli.Load(targetDir)
	if cfg.AccessToken == "" && cfg.APIKey == "" {
		fmt.Println()
		fmt.Println(lipgloss.NewStyle().Foreground(ColorWarning).Bold(true).Render("[AUTH REQUIRED] You must be logged in to use ScanDrix."))
		fmt.Println()
		fmt.Println("Please authenticate before continuing:")
		fmt.Println("  scandrix auth login       # Login with GitHub/GitLab browser OAuth")
		fmt.Println("  scandrix auth team-key    # Authenticate with team key")
		fmt.Println()
		return
	}

	gw := engine.InitLocalGateway()
	runner := engine.NewCLIRunner()

	branch := "main"
	if out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
		branch = strings.TrimSpace(string(out))
	}

	repoName := "local-workspace"
	if rem, err := git.DetectRemote(context.Background(), targetDir); err == nil && rem != nil {
		repoName = rem.NamespacePath
	}

	modelName := ""
	if m := os.Getenv("API_LLM_PROVIDER_MODEL"); m != "" {
		modelName = m
	} else if m := os.Getenv("SCANDRIX_MODEL"); m != "" {
		modelName = m
	} else if m := os.Getenv("AI_MODEL_DEFAULT"); m != "" {
		modelName = m
	} else if m := os.Getenv("OPENAI_MODEL"); m != "" {
		modelName = m
	} else if m := os.Getenv("STRIX_LLM"); m != "" {
		modelName = strings.TrimPrefix(m, "openai/")
	}
	if modelName == "" {
		plan := resolvePlanInfo(cfg)
		modelName = plan.ReviewEngine
	}

	absPath, _ := filepath.Abs(targetDir)
	homeDir, _ := os.UserHomeDir()
	displayPath := absPath
	if homeDir != "" && strings.HasPrefix(absPath, homeDir) {
		displayPath = "~" + strings.TrimPrefix(absPath, homeDir)
	}

	s := &ChatSession{
		gateway:       gw,
		runner:        runner,
		history:       make([]llm.ChatMessage, 0),
		activeModel:   modelName,
		activePersona: AvailablePersonas[0],
		workspacePath: targetDir,
		displayPath:   displayPath,
		gitBranch:     branch,
		repoName:      repoName,
		userEmail:     cfg.UserEmail,
	}

	s.printCommandCodeBanner()

	if initialPrompt != "" {
		s.handleInput(initialPrompt)
	}

	reader := NewLineReader(">")
	for {
		line, err := reader.ReadLine()
		if err != nil {
			if err == io.EOF {
				fmt.Println("\n[INFO] Session ended. Goodbye!")
			}
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if line == "/exit" || line == "/quit" || line == "/q" || line == "exit" || line == "quit" {
			fmt.Println("\n[INFO] Session ended. Goodbye!")
			break
		}

		if line == "/clear" || line == "/cls" || line == "clear" {
			fmt.Print("\033[H\033[2J")
			s.history = nil
			s.printCommandCodeBanner()
			continue
		}

		s.handleInput(line)
	}
}

func (s *ChatSession) printCommandCodeBanner() {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorLogo).
		Padding(0, 1)

	header := fmt.Sprintf("%s %s  ·  %s\n%s",
		lipgloss.NewStyle().Bold(true).Foreground(ColorLogo).Render("ScanDrix CLI"),
		lipgloss.NewStyle().Foreground(ColorShortcut).Render(status.CLIVersion),
		lipgloss.NewStyle().Foreground(ColorMetaText).Render("Autonomous AI Code Review & Security Engine"),
		lipgloss.NewStyle().Foreground(ColorShortcut).Render("Zero-hallucination AST code intelligence · Enforcing rules & OWASP Top 10"),
	)

	fmt.Println()
	fmt.Println(boxStyle.Render(header))
	fmt.Println()

	statusLabel := "Authenticated"
	if s.userEmail != "" {
		statusLabel = fmt.Sprintf("Connected (%s)", s.userEmail)
	}

	fmt.Printf("  %s %s   %s %s\n",
		lipgloss.NewStyle().Foreground(ColorShortcut).Render("Status:"),
		lipgloss.NewStyle().Foreground(ColorSuccess).Render("● "+statusLabel),
		lipgloss.NewStyle().Foreground(ColorShortcut).Render("Workspace:"),
		lipgloss.NewStyle().Foreground(ColorMetaText).Render(s.displayPath),
	)
	fmt.Printf("  %s  %s\n\n",
		lipgloss.NewStyle().Foreground(ColorShortcut).Render("Model: "),
		lipgloss.NewStyle().Foreground(ColorMetaPill).Render(fmt.Sprintf("%s (%s)", s.activeModel, s.activePersona.Name)),
	)
	fmt.Printf("  %s\n\n", lipgloss.NewStyle().Foreground(ColorShortcut).Render("Type /help for commands or ask any code/security question directly."))
}

func startSpinner(msg string) func() {
	stop := make(chan struct{})
	done := make(chan struct{})
	frames := []string{"[.  ]", "[.. ]", "[...]", "[ ..]", "[  .]", "[   ]"}
	style := lipgloss.NewStyle().Foreground(ColorShortcut)

	go func() {
		defer close(done)
		idx := 0
		ticker := time.NewTicker(120 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-stop:
				fmt.Print("\r\033[K")
				return
			case <-ticker.C:
				frame := frames[idx%len(frames)]
				idx++
				fmt.Printf("\r\033[K%s %s", style.Render(frame), style.Render(msg))
			}
		}
	}()

	return func() {
		close(stop)
		<-done
	}
}

func (s *ChatSession) handleInput(line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return
	}

	lower := strings.ToLower(trimmed)
	if lower == "model" || lower == "models" || lower == "plan" {
		s.printPlanAndModelInfo()
		return
	}

	if trimmed == "?" || strings.HasPrefix(trimmed, "/") {
		s.handleSlashCommand(trimmed)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	isScan := isScanIntent(line)
	var stopSpinner func()
	if !isScan {
		stopSpinner = startSpinner("Thinking...")
	}

	reply, err := s.handleAgenticQuery(ctx, line)
	if stopSpinner != nil {
		stopSpinner()
	}

	if err != nil {
		reply = fmt.Sprintf("[ERROR] **AI Provider Error**: %v\n\nPlease check your `OPENAI_API_KEY`, `OPENAI_BASE_URL` (`%s`), or active model (`%s`).",
			err, os.Getenv("OPENAI_BASE_URL"), s.activeModel)
	} else if strings.TrimSpace(reply) == "" {
		reply = "[INFO] Query returned an empty response from AI model."
	}

	s.history = append(s.history,
		llm.ChatMessage{Role: "user", Content: line},
		llm.ChatMessage{Role: "assistant", Content: reply},
	)

	fmt.Println()
	fmt.Println(renderMarkdown(reply))
	fmt.Println()
}

func (s *ChatSession) handleSlashCommand(input string) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return
	}
	cmd := strings.ToLower(parts[0])

	switch {
	case cmd == "/" || cmd == "/?" || cmd == "/help" || cmd == "/h" || cmd == "?" || cmd == "help":
		fmt.Println()
		fmt.Println(formatHelpCard())
		fmt.Println()

	case cmd == "/plan" || cmd == "/m" || cmd == "/mo" || cmd == "/mod" || cmd == "/model" || cmd == "/models":
		s.printPlanAndModelInfo()

	case cmd == "/a" || cmd == "/ag" || cmd == "/age" || cmd == "/agent" || cmd == "/agents" || cmd == "/persona" || cmd == "/personas":
		if len(parts) > 1 {
			s.switchPersona(parts[1])
		} else {
			s.printPersonaMenu()
		}

	case cmd == "/st" || cmd == "/sta" || cmd == "/stat" || cmd == "/status" || cmd == "/info":
		fmt.Println()
		fmt.Println(formatStatusCard(s))
		fmt.Println()

	case cmd == "/ru" || cmd == "/rul" || cmd == "/rule" || cmd == "/rules":
		fmt.Println()
		cfg := configcli.Load(s.workspacePath)
		rulesList, _ := rulescli.ViewRules(context.Background(), cfg.ServerURL, cfg.AccessToken, "", "")
		fmt.Println(formatRulesCard(rulesList))
		fmt.Println()

	case cmd == "/sk" || cmd == "/ski" || cmd == "/skill" || cmd == "/skills":
		fmt.Println()
		subCmd := "list"
		if len(parts) > 1 {
			subCmd = strings.ToLower(parts[1])
		}
		if subCmd == "install" || subCmd == "sync" {
			res, err := skills.Install(s.workspacePath, false)
			if err != nil {
				fmt.Printf("[ERROR] Failed installing skills: %v\n\n", err)
			} else {
				fmt.Printf("[OK] **ScanDrix AI Skills Installed & Synchronized**\n  * Created: %d files\n  * Updated: %d files\n  * Unchanged: %d files\n  * Target Directories: %s\n\n",
					res.CreatedCount, res.UpdatedCount, res.UnchangedCount, strings.Join(res.Targets, ", "))
			}
		} else {
			fmt.Println(formatSkillsCard())
			fmt.Println()
		}

	case cmd == "/tr" || cmd == "/tra" || cmd == "/trace" || cmd == "/traces":
		fmt.Println()
		home, _ := os.UserHomeDir()
		traceDir := filepath.Join(home, ".scandrix", "traces")
		entries, _ := os.ReadDir(traceDir)
		fmt.Printf("[TELEMETRY] **ScanDrix Developer Session Telemetry**\n  * Active Store: `%s`\n  * Recorded Sessions: %d\n  * Run `/trace ui` to launch local decision trace cockpit\n\n", traceDir, len(entries))

	case cmd == "/dr" || cmd == "/dry" || cmd == "/dry-run" || cmd == "/dryrun":
		fmt.Println()
		fmt.Println(lipgloss.NewStyle().Foreground(ColorShortcut).Render("[DRY-RUN] Running review against active security rules..."))
		rawDiffBytes, _ := exec.Command("git", "diff").Output()
		res, err := s.runner.RunReview(context.Background(), string(rawDiffBytes), engine.CLIOptions{
			TargetDirectory: s.workspacePath,
			RulesOnly:       true,
			Format:          engine.FormatJSON,
		})
		if err != nil {
			fmt.Printf("[ERROR] Dry run failed: %v\n\n", err)
		} else {
			fmt.Println(formatFindingsReport("Dry Run Rule Evaluation", s.workspacePath, res.FilesReviewed, res.TotalLines, res.Findings, 0))
			fmt.Println()
		}

	case cmd == "/sch" || cmd == "/sche" || cmd == "/schema":
		fmt.Println()
		data, _ := json.MarshalIndent(schema.GetMasterSchema(), "", "  ")
		fmt.Println(string(data))
		fmt.Println()

	case cmd == "/d" || cmd == "/di" || cmd == "/dif" || cmd == "/diff":
		fmt.Println()
		staged := false
		branchTarget := ""
		for i, p := range parts {
			if p == "--staged" || p == "-s" {
				staged = true
			}
			if p == "--branch" && i+1 < len(parts) {
				branchTarget = parts[i+1]
			}
		}
		args := []string{"diff"}
		if staged {
			args = append(args, "--cached")
		} else if branchTarget != "" {
			args = append(args, "origin/"+branchTarget+"...HEAD")
		}
		out, err := exec.Command("git", args...).Output()
		if err != nil || len(out) == 0 {
			fmt.Println("[INFO] No local git changes detected in working tree.")
		} else {
			fmt.Println(renderMarkdown(fmt.Sprintf("```diff\n%s\n```", string(out))))
		}
		fmt.Println()

	case cmd == "/r" || cmd == "/re" || cmd == "/rev" || cmd == "/review":
		fmt.Println()
		staged := false
		fast := false
		heavy := false
		fix := false
		focus := ""
		branchTarget := ""

		for i, p := range parts {
			if p == "--staged" || p == "-s" {
				staged = true
			}
			if p == "--fast" {
				fast = true
			}
			if p == "--heavy" {
				heavy = true
			}
			if p == "--fix" {
				fix = true
			}
			if p == "--focus" && i+1 < len(parts) {
				focus = parts[i+1]
			}
			if p == "--branch" && i+1 < len(parts) {
				branchTarget = parts[i+1]
			}
		}

		diffArgs := []string{"diff"}
		if staged {
			diffArgs = append(diffArgs, "--cached")
		} else if branchTarget != "" {
			diffArgs = append(diffArgs, "origin/"+branchTarget+"...HEAD")
		}

		rawDiffBytes, _ := exec.Command("git", diffArgs...).Output()
		rawDiff := string(rawDiffBytes)
		if strings.TrimSpace(rawDiff) == "" {
			fmt.Println("[INFO] No git diff detected to review. Stage files with `git add` or make local changes.")
			fmt.Println()
			return
		}

		modeNote := ""
		if fast {
			modeNote = " [FAST Mode]"
		} else if heavy {
			modeNote = " [HEAVY Mode]"
		}
		if focus != "" {
			modeNote += fmt.Sprintf(" [FOCUS: %s]", focus)
		}

		fmt.Println(lipgloss.NewStyle().Foreground(ColorShortcut).Render("[REVIEW] Running AI code review on git diff" + modeNote + "..."))
		res, err := s.runner.RunReview(context.Background(), rawDiff, engine.CLIOptions{
			TargetDirectory: s.workspacePath,
			Fast:            fast,
			Heavy:           heavy,
			Focus:           focus,
			Fix:             fix,
			Format:          engine.FormatJSON,
		})
		if err != nil {
			fmt.Printf("[ERROR] Review failed: %v\n\n", err)
			return
		}
		fmt.Println(formatFindingsReport("AI Code Review Summary", s.workspacePath, res.FilesReviewed, res.TotalLines, res.Findings, res.FixesApplied))
		fmt.Println()

	case cmd == "/s" || cmd == "/sc" || cmd == "/sca" || cmd == "/scan" || cmd == "/sec":
		target := "."
		fix := false
		for _, p := range parts[1:] {
			if p == "--fix" {
				fix = true
			} else if !strings.HasPrefix(p, "-") {
				target = p
			}
		}

		tracker := NewTaskProgressTracker(target)
		tracker.Start(fmt.Sprintf("Exploring %s workspace files", target))

		opts := engine.CLIOptions{
			TargetDirectory: target,
			Fix:             fix,
			Format:          engine.FormatJSON,
			OnProgress:      tracker.OnProgress,
			OnStatus:        tracker.OnStatus,
		}

		res, err := s.runner.RunScan(context.Background(), target, opts)
		tracker.Finish()
		if err != nil {
			fmt.Printf("[ERROR] Scan failed: %v\n\n", err)
			return
		}
		fmt.Println(formatFindingsReport("Deep Security Scan Results", target, res.FilesReviewed, res.TotalLines, res.Findings, 0))
		fmt.Println()

	case cmd == "/f" || cmd == "/fi" || cmd == "/fix" || cmd == "/patch":
		fmt.Println()
		rawDiffBytes, _ := exec.Command("git", "diff").Output()
		res, err := s.runner.RunReview(context.Background(), string(rawDiffBytes), engine.CLIOptions{
			TargetDirectory: s.workspacePath,
			Fix:             true,
		})
		if err != nil {
			fmt.Printf("[ERROR] Auto-fix failed: %v\n\n", err)
		} else if res.FixesApplied > 0 {
			fmt.Printf("[OK] Applied **%d** automated security remediations across source files!\n\n", res.FixesApplied)
		} else {
			fmt.Println("[OK] No automated fixes required. Workspace is fully compliant with active rules.")
		}

	case cmd == "/p" || cmd == "/pr" || cmd == "/pull" || cmd == "/pulls":
		fmt.Println()
		if len(parts) > 1 {
			prInput := parts[1]
			fmt.Println(lipgloss.NewStyle().Foreground(ColorShortcut).Render(fmt.Sprintf("[PR] Fetching and reviewing PR #%s...", prInput)))
			_, prNum, err := pr.ParsePRInput(prInput)
			if err != nil {
				fmt.Printf("[ERROR] Invalid PR reference '%s': %v\n\n", prInput, err)
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			diffContent, err := pr.FetchDiffFromGit(ctx, prNum, "main")
			if err != nil || strings.TrimSpace(diffContent) == "" {
				fmt.Printf("[ERROR] Failed fetching git diff for PR #%d: %v\n\n", prNum, err)
				return
			}
			res, err := s.runner.RunReview(ctx, diffContent, engine.CLIOptions{
				TargetDirectory: s.workspacePath,
				Format:          engine.FormatJSON,
			})
			if err != nil {
				fmt.Printf("[ERROR] Review failed on PR #%d: %v\n\n", prNum, err)
				return
			}
			fmt.Println(formatFindingsReport(fmt.Sprintf("Pull Request #%d Review Summary", prNum), s.workspacePath, res.FilesReviewed, res.TotalLines, res.Findings, res.FixesApplied))
			fmt.Println()
		} else {
			fmt.Printf("[PR] **Pull Request Management**\n  * Active Branch: %s\n  * Usage: `/pr <number>` to review PR diff\n\n", s.gitBranch)
		}

	case cmd == "/h" || cmd == "/ho" || cmd == "/hoo" || cmd == "/hook" || cmd == "/hooks":
		fmt.Println()
		subCmd := "status"
		if len(parts) > 1 {
			subCmd = parts[1]
		}
		if subCmd == "install" {
			if err := hooks.Install(s.workspacePath, true, false, "HIGH"); err != nil {
				fmt.Printf("[ERROR] Failed installing pre-commit hook: %v\n\n", err)
			} else {
				fmt.Println("[OK] **Git Pre-Commit Hook Installed!**\nScanDrix will automatically review staged changes before every `git commit`.")
			}
		} else if subCmd == "remove" || subCmd == "uninstall" {
			if err := hooks.Uninstall(s.workspacePath); err != nil {
				fmt.Printf("[ERROR] Failed removing pre-commit hook: %v\n\n", err)
			} else {
				fmt.Println("[INFO] ScanDrix pre-commit review guard removed.")
			}
		} else {
			st, _ := hooks.Status(s.workspacePath)
			hookStatus := "Disabled"
			if st != nil && st.PreCommitActive {
				hookStatus = "Enabled (Active)"
			}
			fmt.Printf("[HOOKS] **Git Pre-Commit Guard**: %s\n  * Run `/hooks install` to enable automated pre-commit scanning\n  * Run `/hooks remove` to disable\n\n", hookStatus)
		}

	case cmd == "/c" || cmd == "/cf" || cmd == "/cfg" || cmd == "/co" || cmd == "/con" || cmd == "/conf" || cmd == "/config" || cmd == "/settings":
		fmt.Println()
		cfg := configcli.Load(s.workspacePath)
		if len(parts) > 2 && strings.ToLower(parts[1]) == "set" {
			kv := strings.SplitN(parts[2], "=", 2)
			if len(kv) == 2 {
				key, val := strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])
				switch strings.ToLower(key) {
				case "server_url", "server":
					cfg.ServerURL = val
				case "format":
					cfg.DefaultFormat = val
				case "fail_on_severity", "severity":
					cfg.FailOnSeverity = val
				case "api_key", "key":
					cfg.APIKey = val
				}
				_ = configcli.SaveRepo(s.workspacePath, cfg)
				fmt.Printf("[OK] Config field `%s` set to `%s`\n\n", key, val)
				return
			}
		}
		fmt.Println(formatConfigCard(cfg, s))
		fmt.Println()

	case cmd == "/au" || cmd == "/aut" || cmd == "/auth" || cmd == "/whoami" || cmd == "/login":
		fmt.Println()
		cfg := configcli.Load(s.workspacePath)
		fmt.Println(formatAuthCard(cfg))
		fmt.Println()

	case cmd == "/pe" || cmd == "/pen" || cmd == "/pent" || cmd == "/pentest" || cmd == "/str" || cmd == "/stri" || cmd == "/strix":
		fmt.Println()
		target := s.workspacePath
		instruction := "Focus on authentication, JWT tokens, and privilege escalation"
		if len(parts) > 1 {
			target = parts[1]
		}
		if len(parts) > 2 {
			instruction = strings.Join(parts[2:], " ")
		}
		if _, err := exec.LookPath("strix"); err == nil {
			fmt.Println(lipgloss.NewStyle().Foreground(ColorShortcut).Render(fmt.Sprintf("[PENTEST] Launching Strix autonomous dynamic pentesting on `%s`...", target)))
			out, err := exec.Command("strix", "--target", target, "--instruction", instruction, "-n", "-m", "quick").CombinedOutput()
			if err != nil && len(out) == 0 {
				fmt.Printf("[ERROR] Strix execution error: %v\n\n", err)
			} else {
				fmt.Println(string(out))
			}
		} else {
			fmt.Printf("[PENTEST] **Strix Dynamic AI Penetration Testing Bridge Ready**\nTargeting: `%s`\nInstruction: `%s`\n```bash\nstrix --target %s --instruction \"%s\"\n```\n", target, instruction, target, instruction)
		}
		fmt.Println()

	case cmd == "/exp" || cmd == "/expo" || cmd == "/export":
		fmt.Println()
		formatStr := "sarif"
		if len(parts) > 1 {
			formatStr = strings.ToLower(parts[1])
		}
		opts := engine.CLIOptions{
			TargetDirectory: s.workspacePath,
			Offline:         true,
		}
		switch formatStr {
		case "json":
			opts.Format = engine.FormatJSON
		case "csv":
			opts.Format = engine.FormatCSV
		case "markdown", "md":
			opts.Format = engine.FormatMarkdown
		default:
			opts.Format = engine.FormatSARIF
		}
		res, err := s.runner.RunScan(context.Background(), s.workspacePath, opts)
		if err != nil {
			fmt.Printf("[ERROR] Export scan failed: %v\n\n", err)
			return
		}
		reportsDir := filepath.Join(s.workspacePath, ".scandrix", "reports")
		_ = os.MkdirAll(reportsDir, 0755)
		ext := formatStr
		if ext == "markdown" {
			ext = "md"
		}
		outFile := filepath.Join(reportsDir, fmt.Sprintf("scandrix_export_%d.%s", time.Now().Unix(), ext))
		f, err := os.Create(outFile)
		if err != nil {
			fmt.Printf("[ERROR] Failed creating export file: %v\n\n", err)
			return
		}
		defer f.Close()
		formatter := engine.NewOutputFormatter()
		if err := formatter.Render(f, res, opts.Format); err != nil {
			fmt.Printf("[ERROR] Failed rendering export: %v\n\n", err)
			return
		}
		fmt.Printf("[OK] **Export Completed!**\n  * Total Findings: %d\n  * Destination: %s\n  * Format: %s\n\n", len(res.Findings), outFile, strings.ToUpper(formatStr))

	case cmd == "/his" || cmd == "/hist" || cmd == "/history":
		fmt.Println()
		if len(s.history) == 0 {
			fmt.Println("[HISTORY] Current interactive session has no prior turns.")
		} else {
			fmt.Printf("[HISTORY] **Interactive Chat Session Log (%d turns)**:\n", len(s.history))
			for i, m := range s.history {
				roleBadge := lipgloss.NewStyle().Bold(true).Foreground(ColorLogo).Render(strings.ToUpper(m.Role))
				preview := strings.ReplaceAll(m.Content, "\n", " ")
				if len(preview) > 90 {
					preview = preview[:87] + "..."
				}
				fmt.Printf("  [%02d] %s: %s\n", i+1, roleBadge, lipgloss.NewStyle().Foreground(ColorMetaText).Render(preview))
			}
		}
		fmt.Println()

	default:
		fmt.Printf("Unknown command `%s`. Type `/help` or `?` for shortcuts.\n\n", parts[0])
	}
}

func (s *ChatSession) printPlanAndModelInfo() {
	cfg := configcli.Load(s.workspacePath)
	plan := resolvePlanInfo(cfg)

	fmt.Println()
	fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(ColorLogo).Render("[SUBSCRIPTION & AI ENGINES]"))
	fmt.Println()
	fmt.Printf("  Plan Tier:       %s\n", lipgloss.NewStyle().Bold(true).Foreground(ColorHighlight).Render(plan.Name))
	fmt.Printf("  Review Engine:   %s\n", lipgloss.NewStyle().Bold(true).Foreground(ColorSuccess).Render(plan.ReviewEngine))
	fmt.Printf("  Security Engine: %s\n", lipgloss.NewStyle().Bold(true).Foreground(ColorSuccess).Render(plan.SecurityEngine))
	fmt.Printf("  Context Window:  %s\n", lipgloss.NewStyle().Foreground(ColorShortcut).Render(plan.ContextWindow))
	fmt.Printf("  Description:     %s\n", lipgloss.NewStyle().Foreground(ColorMetaText).Render(plan.Description))
	fmt.Println()
	fmt.Printf("  %s\n\n", lipgloss.NewStyle().Foreground(ColorShortcut).Render("AI models are provisioned based on your subscription tier. Upgrade with: scandrix subscribe"))
}

func (s *ChatSession) printPersonaMenu() {
	fmt.Println()
	fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(ColorWarning).Render("[PERSONAS] Available Reviewer Personas:"))
	fmt.Println()

	for i, p := range AvailablePersonas {
		activeMark := ""
		if p.ID == s.activePersona.ID {
			activeMark = lipgloss.NewStyle().Bold(true).Foreground(ColorSuccess).Render(" [Active]")
		}
		fmt.Printf("  [%d] %-6s %-30s%s\n      %s\n",
			i+1,
			p.Tag,
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F8FAFC")).Render(p.Name),
			activeMark,
			lipgloss.NewStyle().Foreground(ColorShortcut).Render(p.Description),
		)
	}
	fmt.Printf("\n%s\n\n", lipgloss.NewStyle().Foreground(ColorShortcut).Render("  Switch persona using: /agent <number or name> (e.g. /agent 2 or /agent stride)"))
}

func (s *ChatSession) switchPersona(arg string) {
	argLower := strings.ToLower(arg)
	var matched *ReviewerPersona

	if idx, err := strconv.Atoi(argLower); err == nil && idx >= 1 && idx <= len(AvailablePersonas) {
		matched = &AvailablePersonas[idx-1]
	}
	if matched == nil {
		for _, p := range AvailablePersonas {
			if strings.Contains(strings.ToLower(p.ID), argLower) || strings.Contains(strings.ToLower(p.Name), argLower) {
				matched = &p
				break
			}
		}
	}

	if matched != nil {
		s.activePersona = *matched
		fmt.Printf("\n[OK] Switched Reviewer Persona to %s **%s**\n\n", matched.Tag, matched.Name)
	} else {
		fmt.Printf("\n[ERROR] Unknown persona '%s'. Type `/agent` to see all available options.\n\n", arg)
	}
}

func (s *ChatSession) handleAgenticQuery(ctx context.Context, prompt string) (string, error) {
	trimmed := strings.TrimSpace(prompt)
	lower := strings.ToLower(trimmed)

	isQuestion := strings.HasSuffix(trimmed, "?") ||
		strings.HasPrefix(lower, "why ") ||
		strings.HasPrefix(lower, "what ") ||
		strings.HasPrefix(lower, "how ") ||
		strings.HasPrefix(lower, "tell me ") ||
		strings.HasPrefix(lower, "can you ") ||
		strings.HasPrefix(lower, "explain ") ||
		strings.HasPrefix(lower, "is this ") ||
		strings.HasPrefix(lower, "is it ")

	// 1. Autonomous Scan Intent (only if explicit scan command, not a general question)
	if !isQuestion && isScanIntent(prompt) {
		targetDir := s.workspacePath
		if targetDir == "" {
			targetDir = "."
		}
		words := strings.Fields(prompt)
		for _, w := range words {
			cleanW := strings.Trim(w, "\"'`.,:;()[]{}")
			if cleanW == "" || strings.EqualFold(cleanW, "scan") || strings.EqualFold(cleanW, "deep") ||
				strings.EqualFold(cleanW, "in") || strings.EqualFold(cleanW, "directory") ||
				strings.EqualFold(cleanW, "dir") || strings.EqualFold(cleanW, "repo") ||
				strings.EqualFold(cleanW, "codebase") || strings.EqualFold(cleanW, "files") ||
				strings.EqualFold(cleanW, "folder") || strings.EqualFold(cleanW, "the") ||
				strings.EqualFold(cleanW, "my") || strings.EqualFold(cleanW, "all") ||
				strings.EqualFold(cleanW, "code") || strings.EqualFold(cleanW, "whole") {
				continue
			}
			if _, err := os.Stat(cleanW); err == nil {
				targetDir = cleanW
				break
			}
			relPath := filepath.Join(s.workspacePath, cleanW)
			if _, err := os.Stat(relPath); err == nil {
				targetDir = relPath
				break
			}
			// Case-insensitive check in workspace directory
			entries, _ := os.ReadDir(s.workspacePath)
			found := false
			for _, e := range entries {
				if strings.EqualFold(e.Name(), cleanW) {
					targetDir = filepath.Join(s.workspacePath, e.Name())
					found = true
					break
				}
			}
			if found {
				break
			}
		}

		tracker := NewTaskProgressTracker(targetDir)
		tracker.Start(fmt.Sprintf("Exploring %s codebase files", targetDir))

		res, err := s.runner.RunScan(ctx, targetDir, engine.CLIOptions{
			TargetDirectory: targetDir,
			Format:          engine.FormatJSON,
			OnProgress:      tracker.OnProgress,
			OnStatus:        tracker.OnStatus,
		})
		tracker.Finish()
		if err != nil {
			return fmt.Sprintf("[ERROR] Scan failed on `%s`: %v", targetDir, err), nil
		}
		report := formatFindingsReport("Deep Security Scan Results", targetDir, res.FilesReviewed, res.TotalLines, res.Findings, 0)
		return report, nil
	}

	// 2. Autonomous Review Intent (only if explicit command to review diff, not an explanation question)
	if !isQuestion && strings.Contains(lower, "review") && (strings.Contains(lower, "diff") || strings.Contains(lower, "git") || strings.Contains(lower, "change") || strings.Contains(lower, "staged") || strings.Contains(lower, "pr")) {
		staged := strings.Contains(lower, "staged")
		diffArgs := []string{"diff"}
		if staged {
			diffArgs = append(diffArgs, "--cached")
		}
		rawDiffBytes, _ := exec.Command("git", diffArgs...).Output()
		rawDiff := string(rawDiffBytes)
		if strings.TrimSpace(rawDiff) == "" {
			return "[INFO] No git changes detected in working tree to review. Stage files with `git add` or make local changes.", nil
		}

		res, err := s.runner.RunReview(ctx, rawDiff, engine.CLIOptions{
			TargetDirectory: s.workspacePath,
			Format:          engine.FormatJSON,
		})
		if err != nil {
			return fmt.Sprintf("[ERROR] Review failed: %v", err), nil
		}
		return formatFindingsReport("AI Code Review Summary", s.workspacePath, res.FilesReviewed, res.TotalLines, res.Findings, res.FixesApplied), nil
	}

	// 3. Autonomous Fix Intent (only if explicit command to fix, not a question)
	if !isQuestion && strings.Contains(lower, "fix") && (strings.Contains(lower, "auto") || strings.Contains(lower, "remediat") || strings.Contains(lower, "patch") || strings.Contains(lower, "issue") || strings.Contains(lower, "vulnerabilit") || strings.Contains(lower, "all")) {
		rawDiffBytes, _ := exec.Command("git", "diff").Output()
		res, err := s.runner.RunReview(ctx, string(rawDiffBytes), engine.CLIOptions{
			TargetDirectory: s.workspacePath,
			Fix:             true,
		})
		if err != nil {
			return fmt.Sprintf("[ERROR] Auto-fix failed: %v", err), nil
		}
		if res.FixesApplied > 0 {
			return fmt.Sprintf("[OK] Applied **%d** automated security remediations across source files!", res.FixesApplied), nil
		}
		return "[OK] No automated fixes required. Workspace is fully compliant with active rules.", nil
	}

	// 4. File Context Extraction
	var injectedFileContext string
	words := strings.Fields(prompt)
	for _, w := range words {
		cleanW := strings.Trim(w, "\"'`.,:;()[]{}")
		if strings.Contains(cleanW, ".") && !strings.HasPrefix(cleanW, "http") {
			candidate := cleanW
			if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() && fi.Size() < 500*1024 {
				if content, readErr := os.ReadFile(candidate); readErr == nil {
					if len(content) > 16*1024 {
						content = append(content[:16*1024], []byte("\n... [truncated for context limit] ...")...)
					}
					injectedFileContext = fmt.Sprintf("\n\n[Referenced File: %s]\n```\n%s\n```", candidate, string(content))
					break
				}
			}
			relCandidate := filepath.Join(s.workspacePath, cleanW)
			if fi, err := os.Stat(relCandidate); err == nil && !fi.IsDir() && fi.Size() < 500*1024 {
				if content, readErr := os.ReadFile(relCandidate); readErr == nil {
					if len(content) > 16*1024 {
						content = append(content[:16*1024], []byte("\n... [truncated for context limit] ...")...)
					}
					injectedFileContext = fmt.Sprintf("\n\n[Referenced File: %s]\n```\n%s\n```", cleanW, string(content))
					break
				}
			}
		}
	}

	// 5. Natural Conversational Query via LLM Gateway
	rawSystemPrompt := fmt.Sprintf(`%s

## Environment Context
- Workspace: %s
- Repository: %s
- Branch: %s
- Active Model: %s
- Active Persona: %s
- Slash commands available: /scan (static scan), /review (git diff review), /fix (apply AST fixes), /rules (manage rules), /plan (view models/tier), /status (diagnostics)%s`,
		s.activePersona.Prompt,
		s.workspacePath,
		s.repoName,
		s.gitBranch,
		s.activeModel,
		s.activePersona.Name,
		injectedFileContext,
	)

	systemPrompt := prompts.ApplyFoundations(rawSystemPrompt)

	if s.gateway != nil {
		reply, err := s.gateway.GenerateChatResponseWithModel(ctx, s.activeModel, systemPrompt, s.history, prompt)
		if err != nil {
			return "", err
		}
		return reply, nil
	}

	return "", fmt.Errorf("AI Gateway is not initialized. Please set OPENROUTER_API_KEY, OPENAI_API_KEY, or DEEPSEEK_API_KEY in .env to enable conversational AI.\nYou can still use local commands: /scan, /review, /fix, /diff, /rules, /skills, /status.")
}

func containsWord(text, target string) bool {
	for _, f := range strings.Fields(strings.ToLower(text)) {
		clean := strings.Trim(f, "\"'`.,:;()[]{}!?/")
		if clean == target {
			return true
		}
	}
	return false
}

func isScanIntent(prompt string) bool {
	trimmed := strings.TrimSpace(prompt)
	lower := strings.ToLower(trimmed)

	// If it is a question or explanation request, it is NOT an autonomous scan trigger
	if strings.HasSuffix(trimmed, "?") ||
		strings.HasPrefix(lower, "why ") ||
		strings.HasPrefix(lower, "what ") ||
		strings.HasPrefix(lower, "how ") ||
		strings.HasPrefix(lower, "is this ") ||
		strings.HasPrefix(lower, "is it ") ||
		strings.HasPrefix(lower, "tell me ") ||
		strings.HasPrefix(lower, "explain ") ||
		strings.HasPrefix(lower, "can you explain ") ||
		strings.Contains(lower, "why did") ||
		strings.Contains(lower, "why is") {
		return false
	}

	// Imperative scan commands
	return strings.HasPrefix(lower, "scan") ||
		strings.HasPrefix(lower, "run scan") ||
		strings.HasPrefix(lower, "do a scan") ||
		strings.HasPrefix(lower, "start scan") ||
		strings.HasPrefix(lower, "audit ") ||
		strings.HasPrefix(lower, "deep scan") ||
		strings.Contains(lower, "scan directory") ||
		strings.Contains(lower, "scan repo") ||
		strings.Contains(lower, "scan folder") ||
		strings.Contains(lower, "scan the ") ||
		strings.Contains(lower, "scan my ")
}

func writeMarkdownReport(target string, filesCount int, totalLines int, findings []models.CodeFinding, fixesApplied int) (string, error) {
	outPath := "scandrix-security-scan-report.md"
	if target != "" && target != "." {
		if fi, err := os.Stat(target); err == nil && fi.IsDir() {
			outPath = filepath.Join(target, "scandrix-security-scan-report.md")
		}
	}

	var md strings.Builder
	md.WriteString("# 🛡️ ScanDrix Deep Security & Vulnerability Audit Report\n\n")
	md.WriteString(fmt.Sprintf("**Target**: `%s` | **Generated**: `%s`\n\n", target, time.Now().Format("2006-01-02 15:04:05 UTC")))
	if totalLines > 0 {
		md.WriteString(fmt.Sprintf("**Scope**: `%d` files scanned (%d lines of code analyzed)\n\n", filesCount, totalLines))
	} else {
		md.WriteString(fmt.Sprintf("**Scope**: `%d` files scanned\n\n", filesCount))
	}

	var crit, high, med, low int
	for _, f := range findings {
		switch f.Severity {
		case models.SeverityCritical:
			crit++
		case models.SeverityHigh:
			high++
		case models.SeverityMedium:
			med++
		case models.SeverityLow:
			low++
		default:
			med++
		}
	}

	md.WriteString("### Executive Summary\n\n")
	md.WriteString("| Severity | Count | Gate Status |\n")
	md.WriteString("|:---|:---:|:---|\n")
	gateCrit := "✅ PASS"
	if crit > 0 {
		gateCrit = "❌ BLOCKS SHIP"
	}
	gateHigh := "✅ PASS"
	if high > 0 {
		gateHigh = "⚠️ REQUIRES APPROVAL"
	}
	md.WriteString(fmt.Sprintf("| 🔴 **CRITICAL** | %d | %s |\n", crit, gateCrit))
	md.WriteString(fmt.Sprintf("| 🟠 **HIGH** | %d | %s |\n", high, gateHigh))
	md.WriteString(fmt.Sprintf("| 🟡 **MEDIUM** | %d | %s |\n", med, "ℹ️ ADVISORY"))
	md.WriteString(fmt.Sprintf("| 🔵 **LOW** | %d | %s |\n\n", low, "ℹ️ INFORMATIONAL"))

	if fixesApplied > 0 {
		md.WriteString(fmt.Sprintf("> 💡 **Auto-Remediations**: `%d` automated AST fixes were applied to resolve findings.\n\n", fixesApplied))
	}

	md.WriteString("### Complete Vulnerability Inventory\n\n")
	md.WriteString("| # | Severity | Finding | Category | Location |\n")
	md.WriteString("|:---:|:---|:---|:---|:---|\n")
	for i, f := range findings {
		icon := "🟡"
		switch f.Severity {
		case models.SeverityCritical:
			icon = "🔴"
		case models.SeverityHigh:
			icon = "🟠"
		case models.SeverityLow:
			icon = "🔵"
		}
		md.WriteString(fmt.Sprintf("| %d | %s **%s** | %s | `%s` | `%s:%d` |\n",
			i+1, icon, f.Severity, f.Title, f.Category, f.FilePath, f.StartLine))
	}
	md.WriteString("\n---\n\n")

	md.WriteString("### Detailed Finding Analysis & Remediations\n\n")
	for i, f := range findings {
		icon := "🟡"
		switch f.Severity {
		case models.SeverityCritical:
			icon = "🔴"
		case models.SeverityHigh:
			icon = "🟠"
		case models.SeverityLow:
			icon = "🔵"
		}
		md.WriteString(fmt.Sprintf("#### %d. %s [%s] %s\n\n", i+1, icon, f.Severity, f.Title))
		md.WriteString(fmt.Sprintf("- **File**: `%s:%d`\n", f.FilePath, f.StartLine))
		md.WriteString(fmt.Sprintf("- **Category / Rule**: `%s`\n", f.Category))
		md.WriteString(fmt.Sprintf("- **Description**: %s\n\n", f.Description))
		if f.Remediation != "" {
			md.WriteString(fmt.Sprintf("**Remediation**:\n```text\n%s\n```\n\n", f.Remediation))
		}
		if f.SuggestedDiff != "" {
			md.WriteString("**Suggested Fix Diff**:\n```diff\n" + f.SuggestedDiff + "\n```\n\n")
		}
		md.WriteString("---\n\n")
	}

	err := os.WriteFile(outPath, []byte(md.String()), 0644)
	return outPath, err
}

func severityRank(sev models.FindingSeverity) int {
	switch strings.ToUpper(string(sev)) {
	case "CRITICAL":
		return 1
	case "HIGH":
		return 2
	case "MEDIUM":
		return 3
	case "LOW":
		return 4
	case "INFO":
		return 5
	default:
		return 3
	}
}

func formatFindingsReport(title string, target string, filesCount int, totalLines int, findings []models.CodeFinding, fixesApplied int) string {
	if len(findings) == 0 {
		return fmt.Sprintf("[SEC] %s\n\n[OK] Workspace Clean: 0 security vulnerabilities detected across %d files.", title, filesCount)
	}

	var criticalCount, highCount, mediumCount, lowCount int
	for _, f := range findings {
		switch f.Severity {
		case models.SeverityCritical:
			criticalCount++
		case models.SeverityHigh:
			highCount++
		case models.SeverityMedium:
			mediumCount++
		case models.SeverityLow:
			lowCount++
		default:
			mediumCount++
		}
	}

	// 1. Stable Sort: AI semantic/architectural findings FIRST, then CRITICAL > HIGH > MEDIUM > LOW > INFO
	sortedFindings := make([]models.CodeFinding, len(findings))
	copy(sortedFindings, findings)
	sort.SliceStable(sortedFindings, func(i, j int) bool {
		isArchI := strings.Contains(strings.ToLower(sortedFindings[i].Category), "architect") || strings.Contains(strings.ToLower(sortedFindings[i].Category), "ai")
		isArchJ := strings.Contains(strings.ToLower(sortedFindings[j].Category), "architect") || strings.Contains(strings.ToLower(sortedFindings[j].Category), "ai")
		if isArchI && !isArchJ {
			return true
		}
		if !isArchI && isArchJ {
			return false
		}
		rI := severityRank(sortedFindings[i].Severity)
		rJ := severityRank(sortedFindings[j].Severity)
		if rI != rJ {
			return rI < rJ
		}
		return sortedFindings[i].FilePath < sortedFindings[j].FilePath
	})

	var card strings.Builder
	card.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorLogo).Render("[SEC] "+title) + "\n\n")

	badgeCrit := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#DC2626")).Padding(0, 1).Render(fmt.Sprintf("[CRITICAL] %d", criticalCount))
	badgeHigh := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#EA580C")).Padding(0, 1).Render(fmt.Sprintf("[HIGH] %d", highCount))
	badgeMed := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0F172A")).Background(lipgloss.Color("#FACC15")).Padding(0, 1).Render(fmt.Sprintf("[MEDIUM] %d", mediumCount))
	badgeLow := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#0284C7")).Padding(0, 1).Render(fmt.Sprintf("[LOW] %d", lowCount))

	if totalLines > 0 {
		card.WriteString(fmt.Sprintf("[REPORT] Scanned %d files (%d lines of code) | %d total vulnerabilities detected\n", filesCount, totalLines, len(findings)))
	} else {
		card.WriteString(fmt.Sprintf("[REPORT] Scanned %d files | %d total vulnerabilities detected\n", filesCount, len(findings)))
	}
	card.WriteString(fmt.Sprintf("%s  %s  %s  %s\n\n", badgeCrit, badgeHigh, badgeMed, badgeLow))

	if fixesApplied > 0 {
		card.WriteString(lipgloss.NewStyle().Foreground(ColorSuccess).Render(fmt.Sprintf("[FIX] Auto-Remediations Available: %d (type `/fix` to apply)\n\n", fixesApplied)))
	}

	// 2. Intelligent deduplication: AI findings ALWAYS shown; static rules show max 2 examples per title
	ruleOccurrence := make(map[string]int)
	groupedSmells := make(map[string]int)
	var featuredFindings []models.CodeFinding

	for _, f := range sortedFindings {
		isAI := strings.Contains(strings.ToLower(f.Category), "ai") || strings.Contains(strings.ToLower(f.Category), "architect")
		if isAI {
			featuredFindings = append(featuredFindings, f)
			continue
		}

		ruleOccurrence[f.Title]++
		// Show at most 2 occurrences per static rule title in terminal
		if ruleOccurrence[f.Title] <= 2 {
			featuredFindings = append(featuredFindings, f)
		} else {
			groupedSmells[f.Title]++
		}
	}

	card.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F8FAFC")).Render(fmt.Sprintf("Primary Security & Architecture Findings (%d featured):", len(featuredFindings))) + "\n\n")

	for i, f := range featuredFindings {
		badgeSev := "[CRITICAL]"
		colorSev := ColorDanger
		if f.Severity == models.SeverityHigh {
			badgeSev = "[HIGH]"
			colorSev = lipgloss.Color("#FB923C")
		} else if f.Severity == models.SeverityMedium {
			badgeSev = "[MEDIUM]"
			colorSev = ColorWarning
		} else if f.Severity == models.SeverityLow {
			badgeSev = "[LOW]"
			colorSev = ColorLogo
		}

		isAI := strings.Contains(strings.ToLower(f.Category), "ai") || strings.Contains(strings.ToLower(f.Category), "architect")
		aiTag := ""
		if isAI {
			aiTag = lipgloss.NewStyle().Bold(true).Foreground(ColorHighlight).Render("✦ [AI SEMANTIC FINDING] ")
		}

		card.WriteString(fmt.Sprintf("%s%s %s %s\n",
			aiTag,
			lipgloss.NewStyle().Bold(true).Foreground(colorSev).Render(badgeSev),
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Render(fmt.Sprintf("[%d] %s", i+1, f.Title)),
			lipgloss.NewStyle().Foreground(ColorShortcut).Render(fmt.Sprintf("* %s:%d", f.FilePath, f.StartLine)),
		))
		card.WriteString(fmt.Sprintf("   %s\n", lipgloss.NewStyle().Foreground(ColorMetaText).Render(f.Description)))
		if f.Remediation != "" {
			card.WriteString(fmt.Sprintf("   %s %s\n", lipgloss.NewStyle().Bold(true).Foreground(ColorSuccess).Render("Fix:"), f.Remediation))
		}
		if f.SuggestedDiff != "" {
			card.WriteString(fmt.Sprintf("   %s\n%s\n", lipgloss.NewStyle().Foreground(ColorShortcut).Render("Code Remediation Diff:"), f.SuggestedDiff))
		}
		card.WriteString("\n")
	}

	// 3. Show grouped code smells/linter findings
	if len(groupedSmells) > 0 {
		card.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#94A3B8")).Render("── Grouped Code Smells & Linter Observations ───────────────────────") + "\n\n")
		for rule, cnt := range groupedSmells {
			card.WriteString(fmt.Sprintf("   %s %s: %d additional occurrences\n",
				lipgloss.NewStyle().Bold(true).Foreground(ColorWarning).Render("ℹ️"),
				lipgloss.NewStyle().Foreground(lipgloss.Color("#CBD5E1")).Render(rule),
				cnt,
			))
		}
		card.WriteString("\n" + lipgloss.NewStyle().Foreground(ColorShortcut).Render("   👉 All individual occurrences are fully detailed in the Markdown report below.") + "\n\n")
	}

	reportPath, err := writeMarkdownReport(target, filesCount, totalLines, findings, fixesApplied)
	if err == nil && reportPath != "" {
		card.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorSuccess).Render(fmt.Sprintf("📄 Full Security Audit Report saved to: %s\n", reportPath)))
	}

	return card.String()
}

func formatSkillsCard() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorLogo).Render("[SKILLS] Bundled AI Assistant Skills Catalog") + "\n\n")
	catalog := skills.BundledSkillsCatalog()
	for _, s := range catalog {
		b.WriteString(fmt.Sprintf("  * %s\n    %s\n",
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F8FAFC")).Render(s.Name),
			lipgloss.NewStyle().Foreground(ColorMetaText).Render(s.Description),
		))
	}
	b.WriteString("\n" + lipgloss.NewStyle().Foreground(ColorShortcut).Render("  Run `/skills install` to synchronize skills into .claude, .cursor/rules, and .agents/skills"))
	return b.String()
}

func formatStatusCard(s *ChatSession) string {
	res, err := status.GetStatus(s.workspacePath)
	if err == nil && res != nil {
		var b strings.Builder
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorLogo).Render("[STATUS] ScanDrix System & Developer Environment") + "\n\n")
		b.WriteString(fmt.Sprintf("  * CLI Version:     %s\n", lipgloss.NewStyle().Foreground(ColorMetaPill).Render(res.Version)))
		b.WriteString(fmt.Sprintf("  * Active Model:    %s\n", lipgloss.NewStyle().Foreground(ColorMetaPill).Render(s.activeModel)))
		b.WriteString(fmt.Sprintf("  * Review Persona:  %s %s\n", s.activePersona.Tag, lipgloss.NewStyle().Foreground(lipgloss.Color("#F1F5F9")).Render(s.activePersona.Name)))
		b.WriteString(fmt.Sprintf("  * Repository:      %s (Branch: %s)\n", lipgloss.NewStyle().Foreground(ColorSuccess).Render(res.Repository), res.CurrentBranch))
		b.WriteString(fmt.Sprintf("  * Pre-Commit Guard:%s\n", res.PreCommitHook))
		b.WriteString(fmt.Sprintf("  * Pre-Push Guard:  %s\n", res.PrePushHook))
		b.WriteString(fmt.Sprintf("  * Assistant Skills:%s\n", res.AssistantHooks))
		b.WriteString(fmt.Sprintf("  * Server API:      %s\n", lipgloss.NewStyle().Foreground(ColorHighlight).Render(res.ServerURL)))
		return b.String()
	}

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorLogo).Render("[STATUS] ScanDrix System Telemetry") + "\n\n")
	b.WriteString(fmt.Sprintf("  * Model:     %s\n", lipgloss.NewStyle().Foreground(ColorMetaPill).Render(s.activeModel)))
	b.WriteString(fmt.Sprintf("  * Persona:   %s %s\n", s.activePersona.Tag, lipgloss.NewStyle().Foreground(lipgloss.Color("#F1F5F9")).Render(s.activePersona.Name)))
	b.WriteString(fmt.Sprintf("  * Branch:    %s\n", lipgloss.NewStyle().Foreground(ColorSuccess).Render(s.gitBranch)))
	b.WriteString(fmt.Sprintf("  * Workspace: %s\n", lipgloss.NewStyle().Foreground(ColorShortcut).Render(s.displayPath)))
	return b.String()
}

func formatHelpCard() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorLogo).Render("[?] Shortcuts & Commands:") + "\n\n")

	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorHighlight).Render("  Security & Code Review:") + "\n")
	b.WriteString("    /scan [path]        [scan]     Deep scan files for vulnerabilities, smells & secrets\n")
	b.WriteString("    /review [options]   [review]   AI code review (--staged, --branch <branch>, --fast, --heavy, --focus <area>, --fix)\n")
	b.WriteString("    /diff [--staged]    [diff]     Inspect colorized git diff\n")
	b.WriteString("    /fix                [fix]      Automatically apply actionable AST security fixes\n")
	b.WriteString("    /dry-run            [dry-run]  Dry run review against active rules\n")
	b.WriteString("    /pentest [target]   [pentest]  Trigger Strix dynamic penetration testing & exploit verification\n\n")

	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorHighlight).Render("  Plan & Reviewer Persona:") + "\n")
	b.WriteString("    /plan               [plan]     View subscription plan tier and assigned AI models\n")
	b.WriteString("    /agent [name]       [agent]    Switch reviewer persona (Security, Threat, Perf, Staff, Arch)\n\n")

	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorHighlight).Render("  Integrations & Toolchain:") + "\n")
	b.WriteString("    /pr <number>        [pr]       Fetch and review remote GitHub/GitLab Pull Request\n")
	b.WriteString("    /hooks [install]    [hooks]    Install or remove automated Git pre-commit guards\n")
	b.WriteString("    /rules [list|sync]  [rules]    Display active workspace security rules & custom guardrails\n")
	b.WriteString("    /skills [install]   [skills]   Install bundled AI skills for Claude Code, Cursor, AGY\n")
	b.WriteString("    /trace [status]     [trace]    Developer telemetry and session decision memory\n\n")

	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorHighlight).Render("  Configuration & Session:") + "\n")
	b.WriteString("    /config [set k=v]   [config]   View and modify workspace configuration & API settings\n")
	b.WriteString("    /auth               [auth]     Display authentication session, active user, and device token\n")
	b.WriteString("    /export [format]    [export]   Export findings as SARIF, JSON, CSV, or Markdown\n")
	b.WriteString("    /history            [history]  Display interactive conversation turns and recall log\n")
	b.WriteString("    /status             [status]   Full environment telemetry dashboard\n")
	b.WriteString("    /schema             [schema]   Export CLI JSON schema for AI agents\n")
	b.WriteString("    /clear              [clear]    Clear screen and reset session history\n")
	b.WriteString("    /exit               [exit]     Exit live chat session\n")

	return b.String()
}

func formatRulesCard(rulesList []rulescli.RuleModel) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorLogo).Render("[RULES] Active Security Policies & Guardrails") + "\n\n")

	if len(rulesList) > 0 {
		for i, r := range rulesList {
			sevColor := ColorDanger
			if r.Severity == "HIGH" {
				sevColor = ColorWarning
			} else if r.Severity == "MEDIUM" {
				sevColor = ColorMetaPill
			}
			b.WriteString(fmt.Sprintf("  * %s %s %s\n",
				lipgloss.NewStyle().Bold(true).Foreground(sevColor).Render(fmt.Sprintf("[%s]", r.Severity)),
				lipgloss.NewStyle().Foreground(lipgloss.Color("#F1F5F9")).Render(r.Title),
				lipgloss.NewStyle().Foreground(ColorShortcut).Render("("+r.Category+")"),
			))
			if i >= 9 && len(rulesList) > 10 {
				b.WriteString(lipgloss.NewStyle().Foreground(ColorShortcut).Render(fmt.Sprintf("  ...and %d more custom rules\n", len(rulesList)-10)))
				break
			}
		}
	} else {
		owaspRules := catalog.GetOWASPRules()
		goRules := catalog.GetGolangRules()
		total := len(owaspRules) + len(goRules)

		b.WriteString(fmt.Sprintf("  [RULES] **%d Built-in Security & Quality Guardrails Active**\n\n", total))
		for i, r := range owaspRules {
			sevColor := ColorDanger
			if r.Severity == models.SeverityHigh {
				sevColor = ColorWarning
			} else if r.Severity == models.SeverityMedium {
				sevColor = ColorMetaPill
			}
			b.WriteString(fmt.Sprintf("  * %s %s %s\n",
				lipgloss.NewStyle().Bold(true).Foreground(sevColor).Render(fmt.Sprintf("[%s]", r.Severity)),
				lipgloss.NewStyle().Foreground(lipgloss.Color("#F1F5F9")).Render(r.Name),
				lipgloss.NewStyle().Foreground(ColorShortcut).Render("("+r.OWASP+")"),
			))
			if i >= 6 {
				b.WriteString(lipgloss.NewStyle().Foreground(ColorShortcut).Render(fmt.Sprintf("  ...and %d more rules in catalog\n", total-7)))
				break
			}
		}
	}
	return b.String()
}

func formatConfigCard(cfg *configcli.CLIConfig, s *ChatSession) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorLogo).Render("[CONFIG] ScanDrix Configuration & Settings") + "\n\n")
	b.WriteString(fmt.Sprintf("  * Active Model:    %s\n", lipgloss.NewStyle().Foreground(ColorMetaPill).Render(s.activeModel)))
	b.WriteString(fmt.Sprintf("  * Fallback Model:  %s\n", lipgloss.NewStyle().Foreground(ColorMetaText).Render(os.Getenv("AI_MODEL_FALLBACK"))))
	if cfg != nil {
		b.WriteString(fmt.Sprintf("  * API Server URL:  %s\n", lipgloss.NewStyle().Foreground(ColorHighlight).Render(cfg.ServerURL)))
		if cfg.FailOnSeverity != "" {
			b.WriteString(fmt.Sprintf("  * Fail Severity:   %s\n", lipgloss.NewStyle().Foreground(ColorWarning).Render(cfg.FailOnSeverity)))
		}
	}
	b.WriteString(fmt.Sprintf("  * OpenAI/APInex:   %s\n", lipgloss.NewStyle().Foreground(ColorSuccess).Render(os.Getenv("OPENAI_BASE_URL"))))
	b.WriteString(fmt.Sprintf("  * Local Workspace: %s\n", lipgloss.NewStyle().Foreground(ColorShortcut).Render(s.displayPath)))
	return b.String()
}

func formatAuthCard(cfg *configcli.CLIConfig) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorLogo).Render("[AUTH] Authentication & Session Status") + "\n\n")
	if cfg != nil && cfg.UserEmail != "" {
		b.WriteString(fmt.Sprintf("  * User:     %s\n", lipgloss.NewStyle().Bold(true).Foreground(ColorSuccess).Render(cfg.UserEmail)))
	} else {
		b.WriteString(fmt.Sprintf("  * User:     %s\n", lipgloss.NewStyle().Foreground(ColorShortcut).Render("Anonymous / Local CLI Session")))
	}
	if cfg != nil && cfg.AccessToken != "" {
		b.WriteString(fmt.Sprintf("  * JWT:      %s\n", lipgloss.NewStyle().Foreground(ColorSuccess).Render("Active Authenticated Session")))
	} else if cfg != nil && cfg.APIKey != "" {
		b.WriteString(fmt.Sprintf("  * API Key:  %s\n", lipgloss.NewStyle().Foreground(ColorSuccess).Render("Configured (scandrix_*)")))
	} else {
		b.WriteString(fmt.Sprintf("  * Auth:     %s\n", lipgloss.NewStyle().Foreground(ColorWarning).Render("Running in Offline/Local Mode (No cloud account required)")))
	}
	if cfg != nil && cfg.ServerURL != "" {
		b.WriteString(fmt.Sprintf("  * Server:   %s\n", lipgloss.NewStyle().Foreground(ColorShortcut).Render(cfg.ServerURL)))
	}
	return b.String()
}

func cleanMarkdownText(s string) string {
	styleBold := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF"))
	styleCode := lipgloss.NewStyle().Foreground(ColorMetaPill)

	for {
		start := strings.Index(s, "**")
		if start == -1 {
			break
		}
		end := strings.Index(s[start+2:], "**")
		if end == -1 {
			s = s[:start] + s[start+2:]
			break
		}
		boldText := s[start+2 : start+2+end]
		s = s[:start] + styleBold.Render(boldText) + s[start+2+end+2:]
	}

	for {
		start := strings.Index(s, "`")
		if start == -1 {
			break
		}
		end := strings.Index(s[start+1:], "`")
		if end == -1 {
			s = s[:start] + s[start+1:]
			break
		}
		codeText := s[start+1 : start+1+end]
		s = s[:start] + styleCode.Render(codeText) + s[start+1+end+1:]
	}

	return s
}

func renderMarkdown(text string) string {
	lines := strings.Split(text, "\n")
	var b strings.Builder
	inCodeBlock := false
	codeBlockLang := ""

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			if inCodeBlock {
				codeBlockLang = strings.TrimPrefix(trimmed, "```")
				if codeBlockLang == "" {
					codeBlockLang = "Code"
				}
				if codeBlockLang != "ascii" && codeBlockLang != "raw" {
					b.WriteString(lipgloss.NewStyle().Foreground(ColorDivider).Render(fmt.Sprintf("+-- [ %s ] ", codeBlockLang)) + lipgloss.NewStyle().Foreground(ColorDivider).Render(strings.Repeat("-", 60)) + "\n")
				}
			} else {
				if codeBlockLang != "ascii" && codeBlockLang != "raw" {
					b.WriteString(lipgloss.NewStyle().Foreground(ColorDivider).Render("+--"+strings.Repeat("-", 70)) + "\n")
				}
				codeBlockLang = ""
			}
			continue
		}

		if inCodeBlock {
			if codeBlockLang == "ascii" || codeBlockLang == "raw" {
				b.WriteString(line + "\n")
				continue
			}
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				b.WriteString("| " + lipgloss.NewStyle().Foreground(ColorSuccess).Render(line) + "\n")
			} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
				b.WriteString("| " + lipgloss.NewStyle().Foreground(ColorDanger).Render(line) + "\n")
			} else {
				b.WriteString("| " + lipgloss.NewStyle().Foreground(lipgloss.Color("#E2E8F0")).Render(line) + "\n")
			}
			continue
		}

		if trimmed == "" {
			b.WriteString("\n")
			continue
		}

		if strings.HasPrefix(trimmed, "#### ") {
			title := strings.TrimPrefix(trimmed, "#### ")
			b.WriteString("\n" + lipgloss.NewStyle().Bold(true).Foreground(ColorLogo).Render(cleanMarkdownText(title)) + "\n")
		} else if strings.HasPrefix(trimmed, "### ") {
			title := strings.TrimPrefix(trimmed, "### ")
			b.WriteString("\n" + lipgloss.NewStyle().Bold(true).Foreground(ColorLogo).Render(cleanMarkdownText(title)) + "\n")
		} else if strings.HasPrefix(trimmed, "## ") {
			title := strings.TrimPrefix(trimmed, "## ")
			b.WriteString("\n" + lipgloss.NewStyle().Bold(true).Foreground(ColorHighlight).Render(cleanMarkdownText(title)) + "\n")
		} else if strings.HasPrefix(trimmed, "# ") {
			title := strings.TrimPrefix(trimmed, "# ")
			b.WriteString("\n" + lipgloss.NewStyle().Bold(true).Foreground(ColorLogo).Render(cleanMarkdownText(title)) + "\n")
		} else if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "\u2022 ") {
			bulletContent := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(trimmed, "- "), "* "), "\u2022 ")
			b.WriteString("  " + lipgloss.NewStyle().Foreground(ColorLogo).Render("*") + " " + cleanMarkdownText(bulletContent) + "\n")
		} else if len(trimmed) > 2 && trimmed[0] >= '0' && trimmed[0] <= '9' && (trimmed[1] == '.' || (len(trimmed) > 3 && trimmed[1] >= '0' && trimmed[1] <= '9' && trimmed[2] == '.')) {
			dotIdx := strings.Index(trimmed, ".")
			num := trimmed[:dotIdx+1]
			rest := strings.TrimSpace(trimmed[dotIdx+1:])
			b.WriteString("  " + lipgloss.NewStyle().Bold(true).Foreground(ColorLogo).Render(num) + " " + cleanMarkdownText(rest) + "\n")
		} else {
			b.WriteString(cleanMarkdownText(line) + "\n")
		}
	}
	return b.String()
}
