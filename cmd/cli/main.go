package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/scandrix/backend/internal/auth/cliauth"
	"github.com/scandrix/backend/internal/cli/tui"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/pkg/models"
)

// CLIConfig holds persistent credentials stored in ~/.scandrix/config.json
type CLIConfig struct {
	ServerURL    string `json:"server_url"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	APIKey       string `json:"api_key,omitempty"`
	UserEmail    string `json:"user_email,omitempty"`
	WorkspaceID  string `json:"workspace_id,omitempty"`
}

func configPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".scandrix", "config.json")
}

func loadConfig() *CLIConfig {
	cfg := &CLIConfig{ServerURL: "http://localhost:8080"}
	data, err := os.ReadFile(configPath())
	if err == nil {
		_ = json.Unmarshal(data, cfg)
	}
	if envURL := os.Getenv("SCANDRIX_SERVER_URL"); envURL != "" {
		cfg.ServerURL = envURL
	}
	if envKey := os.Getenv("SCANDRIX_API_KEY"); envKey != "" {
		cfg.APIKey = envKey
	}
	return cfg
}

func saveConfig(cfg *CLIConfig) error {
	dir := filepath.Dir(configPath())
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), data, 0600)
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "login":
		handleLogin(args)
	case "logout":
		handleLogout()
	case "whoami":
		handleWhoami()
	case "review":
		handleReview(args)
	case "tui", "dashboard", "ui":
		handleTUI(args)
	case "dry-run":
		handleDryRun(args)
	case "version", "--version", "-v":
		fmt.Println("ScanDrix CLI v1.0.0 (darwin/linux/windows)")
	case "help", "--help", "-h":
		printUsage()
	default:
		// If first arg is a flag, default to review command
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
  tui         Launch interactive Bubbletea developer terminal cockpit dashboard
  review      Perform comprehensive code review on git diff or patch files
  dry-run     Locally evaluate deterministic security and quality rules without server
  login       Authenticate terminal via RFC 8628 browser device authorization flow
  logout      Clear stored authentication tokens from ~/.scandrix/config.json
  whoami      Display current authenticated user, active workspace, and token status
  version     Display ScanDrix CLI version

Options for 'review' & 'tui':
  --git       Extract diff automatically from 'git diff HEAD~1' (default if in git repo)
  --staged    Extract diff from staged changes ('git diff --cached')
  --file      Path to a unified .diff or .patch file
  --server    Override server URL (default: http://localhost:8080)
  --key       Provide an API key (scandrix_*) or Bearer token directly
  -i, --tui   Launch interactive Bubbletea TUI dashboard directly

Examples:
  scandrix tui
  scandrix review --staged --tui
  git diff main | scandrix tui
  scandrix dry-run --file patch.diff`)
}

// ----------------------------------------------------------------------------
// Command: login (RFC 8628 Device Authorization Flow)
// ----------------------------------------------------------------------------

func handleLogin(args []string) {
	cfg := loadConfig()

	for i := 0; i < len(args); i++ {
		if args[i] == "--server" && i+1 < len(args) {
			cfg.ServerURL = args[i+1]
			i++
		}
	}

	fmt.Println("🔐 Initiating ScanDrix Device Authorization Flow...")

	resp, err := http.Post(cfg.ServerURL+"/api/v1/auth/cli/device/initiate", "application/json", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed contacting server at %s: %v\n", cfg.ServerURL, err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Login initiation failed (HTTP %d): %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	var initResult cliauth.DeviceLoginInitiateResult
	if err := json.NewDecoder(resp.Body).Decode(&initResult); err != nil {
		fmt.Fprintf(os.Stderr, "Failed decoding initiation response: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n========================================================")
	fmt.Printf("  Verification Code : \033[1;32m%s\033[0m\n", initResult.UserCode)
	fmt.Printf("  Browser URL       : %s\n", initResult.VerificationURIComplete)
	fmt.Println("========================================================")
	fmt.Println("\nPlease confirm the code in your browser. Waiting for approval...")

	// Try to open browser automatically
	openBrowser(initResult.VerificationURIComplete)

	// Poll for authorization completion
	interval := time.Duration(initResult.Interval) * time.Second
	if interval < 2*time.Second {
		interval = 3 * time.Second
	}

	deadline := time.Now().Add(time.Duration(initResult.ExpiresIn) * time.Second)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		<-ticker.C

		pollResp, err := http.Get(fmt.Sprintf("%s/api/v1/auth/cli/device/poll?device_code=%s", cfg.ServerURL, initResult.DeviceCode))
		if err != nil {
			continue
		}

		if pollResp.StatusCode == http.StatusOK {
			var pollResult cliauth.DeviceLoginPollResult
			_ = json.NewDecoder(pollResp.Body).Decode(&pollResult)
			pollResp.Body.Close()

			if pollResult.Status == cliauth.StatusCompleted {
				cfg.AccessToken = pollResult.AccessToken
				cfg.RefreshToken = pollResult.RefreshToken
				cfg.UserEmail = pollResult.UserEmail

				if err := saveConfig(cfg); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed saving config: %v\n", err)
				}

				fmt.Printf("\n✨ \033[1;32mSuccessfully authenticated!\033[0m Logged in as: %s\n", cfg.UserEmail)
				fmt.Printf("Credentials saved to %s\n", configPath())
				return
			}
		} else if pollResp.StatusCode == http.StatusForbidden {
			pollResp.Body.Close()
			fmt.Fprintln(os.Stderr, "\n❌ Authorization was denied by the user.")
			os.Exit(1)
		} else if pollResp.StatusCode == http.StatusGone {
			pollResp.Body.Close()
			fmt.Fprintln(os.Stderr, "\n⌛ Authorization code expired. Please run 'scandrix login' again.")
			os.Exit(1)
		}
		pollResp.Body.Close()
		fmt.Print(".")
	}

	fmt.Fprintln(os.Stderr, "\n⌛ Device authorization timed out.")
	os.Exit(1)
}

func handleLogout() {
	path := configPath()
	_ = os.Remove(path)
	fmt.Printf("👋 Logged out. Stored credentials removed from %s\n", path)
}

func handleWhoami() {
	cfg := loadConfig()
	if cfg.AccessToken == "" && cfg.APIKey == "" {
		fmt.Println("Not logged in. Run 'scandrix login' or set SCANDRIX_API_KEY.")
		return
	}

	fmt.Println("👤 ScanDrix Identity Profile:")
	fmt.Printf("  Server    : %s\n", cfg.ServerURL)
	if cfg.UserEmail != "" {
		fmt.Printf("  User      : %s\n", cfg.UserEmail)
	}
	if cfg.AccessToken != "" {
		fmt.Println("  Auth Mode : OAuth / Device Session (JWT)")
	} else if cfg.APIKey != "" {
		fmt.Println("  Auth Mode : Team API Key")
	}
}

func extractDiff(diffPath string, staged bool, useGit bool) string {
	if staged {
		out, err := exec.Command("git", "diff", "--cached").Output()
		if err == nil && len(out) > 0 {
			return string(out)
		}
	} else if useGit {
		out, err := exec.Command("git", "diff", "HEAD~1").Output()
		if err == nil && len(out) > 0 {
			return string(out)
		}
		out, _ = exec.Command("git", "diff").Output()
		return string(out)
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
		// Default to git diff
		out, err := exec.Command("git", "diff", "HEAD~1").Output()
		if err == nil && len(out) > 0 {
			return string(out)
		}
		out, _ = exec.Command("git", "diff").Output()
		return string(out)
	}
	return ""
}

// ----------------------------------------------------------------------------
// Command: tui (Interactive Bubbletea Developer Cockpit)
// ----------------------------------------------------------------------------

func handleTUI(args []string) {
	var (
		diffPath string
		staged   bool
		useGit   bool
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--file":
			if i+1 < len(args) {
				diffPath = args[i+1]
				i++
			}
		case "--staged":
			staged = true
		case "--git":
			useGit = true
		}
	}

	rawDiff := extractDiff(diffPath, staged, useGit)
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
// Command: review
// ----------------------------------------------------------------------------

func handleReview(args []string) {
	cfg := loadConfig()

	var (
		diffPath   string
		useGit     bool
		staged     bool
		customKey  string
		launchTUI  bool
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--file":
			if i+1 < len(args) {
				diffPath = args[i+1]
				i++
			}
		case "--git":
			useGit = true
		case "--staged":
			staged = true
			useGit = true
		case "--server":
			if i+1 < len(args) {
				cfg.ServerURL = args[i+1]
				i++
			}
		case "--key":
			if i+1 < len(args) {
				customKey = args[i+1]
				i++
			}
		case "-i", "--tui", "--interactive":
			launchTUI = true
		}
	}

	if launchTUI {
		handleTUI(args)
		return
	}

	authToken := cfg.AccessToken
	if customKey != "" {
		authToken = customKey
	} else if cfg.APIKey != "" {
		authToken = cfg.APIKey
	}

	rawDiff := extractDiff(diffPath, staged, useGit)
	if strings.TrimSpace(rawDiff) == "" {
		fmt.Println("✨ No changes detected in diff. Nothing to review.")
		return
	}

	// If no auth token is available, perform local rule evaluation directly
	if authToken == "" {
		fmt.Println("ℹ️  No authentication token found. Running local deterministic rule evaluation...")
		runLocalRules(rawDiff)
		return
	}

	fmt.Println("🔍 Submitting diff to ScanDrix Autonomous Multi-Agent Engine...")

	reqPayload, _ := json.Marshal(map[string]any{
		"title":    "CLI Local Review",
		"raw_diff": rawDiff,
	})

	req, err := http.NewRequest(http.MethodPost, cfg.ServerURL+"/api/v1/reviews", bytes.NewReader(reqPayload))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed creating request: %v\n", err)
		os.Exit(1)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("⚠️  Backend unavailable at %s: %v\nFalling back to local rule evaluation...\n", cfg.ServerURL, err)
		runLocalRules(rawDiff)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusOK {
		var result map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&result)
		fmt.Printf("\n🚀 Review enqueued successfully! Review ID: %v\n", result["review_id"])
		if streamURL, ok := result["stream"].(string); ok {
			fmt.Printf("   Live Stream: %s%s\n", cfg.ServerURL, streamURL)
		}
	} else {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Review submission failed (HTTP %d): %s\n", resp.StatusCode, string(body))
	}
}

// ----------------------------------------------------------------------------
// Command: dry-run (Local Deterministic Rule Engine)
// ----------------------------------------------------------------------------

func handleDryRun(args []string) {
	var diffPath string
	for i := 0; i < len(args); i++ {
		if args[i] == "--file" && i+1 < len(args) {
			diffPath = args[i+1]
			i++
		}
	}

	var rawDiff string
	if diffPath != "" {
		bytes, err := os.ReadFile(diffPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed reading diff file: %v\n", err)
			os.Exit(1)
		}
		rawDiff = string(bytes)
	} else {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			bytes, _ := io.ReadAll(os.Stdin)
			rawDiff = string(bytes)
		} else {
			out, _ := exec.Command("git", "diff", "HEAD~1").Output()
			rawDiff = string(out)
		}
	}

	if strings.TrimSpace(rawDiff) == "" {
		fmt.Println("No diff content found to dry-run.")
		return
	}

	runLocalRules(rawDiff)
}

func runLocalRules(rawDiff string) {
	evaluator, err := rules.NewEvaluator(rules.DefaultCatalog())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed initializing rule evaluator: %v\n", err)
		return
	}

	patches, err := diff.ParseUnifiedDiff(strings.NewReader(rawDiff))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed parsing diff: %v\n", err)
		return
	}

	findings := evaluator.EvaluatePatches(uuid.New(), uuid.Nil, patches)

	fmt.Printf("\n📋 ScanDrix Rule Evaluation Report: %d findings\n", len(findings))
	if len(findings) == 0 {
		fmt.Println("✅ All deterministic security and quality rules passed cleanly!")
		return
	}

	for i, f := range findings {
		color := "\033[33m" // Yellow
		if f.Severity == models.SeverityCritical || f.Severity == models.SeverityHigh {
			color = "\033[31m" // Red
		} else if f.Severity == models.SeverityInfo {
			color = "\033[36m" // Cyan
		}
		reset := "\033[0m"

		fmt.Printf("\n[%d] %s[%s]%s %s (Line %d:%d)\n", i+1, color, f.Severity, reset, f.FilePath, f.StartLine, f.EndLine)
		fmt.Printf("    Title: %s\n", f.Title)
		fmt.Printf("    Description: %s\n", f.Description)
		if f.Remediation != "" {
			fmt.Printf("    Remediation: %s\n", f.Remediation)
		}
		if f.SuggestedDiff != "" {
			fmt.Printf("    Suggested Fix:\n%s\n", indent(f.SuggestedDiff, "      "))
		}
	}
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default: // linux, freebsd
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
