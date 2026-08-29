package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/scandrix/backend/pkg/models"
)

func main() {
	var (
		serverURL string
		apiKey    string
		diffPath  string
		useGit    bool
	)

	flag.StringVar(&serverURL, "server", "http://localhost:8080", "Scandrix backend API URL")
	flag.StringVar(&apiKey, "key", os.Getenv("SCANDRIX_API_KEY"), "Workspace API key (scandrix_*)")
	flag.StringVar(&diffPath, "file", "", "Path to local unified diff file to review")
	flag.BoolVar(&useGit, "git", false, "Extract diff automatically via 'git diff HEAD~1'")
	flag.Parse()

	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "Error: missing required API key. Set SCANDRIX_API_KEY or use --key flag.")
		os.Exit(1)
	}

	var rawDiff string
	if useGit {
		cmd := exec.Command("git", "diff", "HEAD~1")
		out, err := cmd.Output()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to extract git diff: %v\n", err)
			os.Exit(1)
		}
		rawDiff = string(out)
	} else if diffPath != "" {
		bytes, err := os.ReadFile(diffPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed reading diff file: %v\n", err)
			os.Exit(1)
		}
		rawDiff = string(bytes)
	} else {
		// Read from stdin if piped
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			bytes, err := io.ReadAll(os.Stdin)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed reading stdin: %v\n", err)
				os.Exit(1)
			}
			rawDiff = string(bytes)
		} else {
			fmt.Println("Usage: scandrix-cli --git OR scandrix-cli --file <path> OR cat patch.diff | scandrix-cli")
			os.Exit(1)
		}
	}

	if strings.TrimSpace(rawDiff) == "" {
		fmt.Println("No changes detected in diff. Nothing to review.")
		return
	}

	fmt.Println("🔍 Submitting diff to Scandrix Autonomous Review Engine...")

	reqPayload := map[string]any{
		"title":    "CLI Local Review",
		"raw_diff": rawDiff,
	}
	payloadBytes, _ := json.Marshal(reqPayload)

	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/v1/reviews/cli", bytes.NewReader(payloadBytes))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed creating request: %v\n", err)
		os.Exit(1)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed communicating with backend at %s: %v\n", serverURL, err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Review failed (HTTP %d): %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	var results struct {
		ReviewID uuidString          `json:"review_id"`
		Findings []models.CodeFinding `json:"findings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		fmt.Fprintf(os.Stderr, "Failed decoding response: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n✅ Review Complete! Found %d issues:\n", len(results.Findings))
	for i, f := range results.Findings {
		color := "\033[33m" // Yellow for medium
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
			fmt.Printf("    Fix: %s\n", f.Remediation)
		}
		if f.SuggestedDiff != "" {
			fmt.Printf("    Suggested Diff:\n%s\n", indent(f.SuggestedDiff, "      "))
		}
	}
}

type uuidString string

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
