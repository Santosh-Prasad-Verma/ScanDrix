// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package status

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/scandrix/backend/internal/cli/configcli"
	"github.com/scandrix/backend/internal/cli/hooks"
	"github.com/scandrix/backend/internal/cli/skills"
)

const CLIVersion = "v1.2.0"

// StatusResult aggregates full developer environment state for ScanDrix.
type StatusResult struct {
	Version        string `json:"version"`
	ServerURL      string `json:"server_url"`
	AuthMode       string `json:"auth_mode"`
	UserEmail      string `json:"user_email,omitempty"`
	WorkspaceID    string `json:"workspace_id,omitempty"`
	TeamKeyStatus  string `json:"team_key_status"`
	Repository     string `json:"repository"`
	CurrentBranch  string `json:"current_branch"`
	PreCommitHook  string `json:"pre_commit_hook"`
	PrePushHook    string `json:"pre_push_hook"`
	AssistantHooks string `json:"assistant_hooks"`
	BundledSkills  int    `json:"bundled_skills"`
}

// GetStatus inspects workspace, auth tokens, git configuration, and hooks.
func GetStatus(workDir string) (*StatusResult, error) {
	if workDir == "" {
		workDir = "."
	}

	cfg := configcli.Load(workDir)

	// Auth mode calculation
	authMode := "Not Authenticated"
	if cfg.AccessToken != "" {
		authMode = "OAuth / Device Session (JWT)"
	} else if cfg.APIKey != "" {
		authMode = "Team API Key"
	}

	// Team key status
	teamKey := "Not Configured (required for: custom rules sync, repo settings)"
	if cfg.APIKey != "" {
		teamKey = "Configured"
	}

	// Git repository and branch inspection
	repoPath := "Not a git repository"
	branch := "unknown"

	if rootBytes, err := exec.Command("git", "rev-parse", "--show-toplevel").Output(); err == nil {
		root := strings.TrimSpace(string(rootBytes))
		home, _ := os.UserHomeDir()
		if home != "" && strings.HasPrefix(root, home) {
			repoPath = "~" + root[len(home):]
		} else {
			repoPath = root
		}

		if branchBytes, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
			branch = strings.TrimSpace(string(branchBytes))
		}
	}

	// Git hooks inspection
	preCommit := "Not Installed"
	prePush := "Not Installed"
	if hookStat, err := hooks.Status(workDir); err == nil && hookStat.GitRepoDetected {
		if hookStat.PreCommitActive {
			preCommit = "Active (ScanDrix Guard)"
		} else if hookStat.PreCommitIsCustom {
			preCommit = "Active (Custom user hook)"
		}

		if hookStat.PrePushActive {
			prePush = "Active (ScanDrix Guard)"
		} else if hookStat.PrePushIsCustom {
			prePush = "Active (Custom user hook)"
		}
	}

	// AI Assistant Hooks detection
	assistantList := make([]string, 0)
	if _, err := os.Stat(filepath.Join(workDir, ".cursor", "rules")); err == nil {
		assistantList = append(assistantList, "Cursor")
	}
	if _, err := os.Stat(filepath.Join(workDir, ".claude", "settings.json")); err == nil {
		assistantList = append(assistantList, "Claude Code")
	} else if _, err := os.Stat(filepath.Join(workDir, ".claude", "commands")); err == nil {
		assistantList = append(assistantList, "Claude Code")
	}
	if home, err := os.UserHomeDir(); err == nil {
		if _, err := os.Stat(filepath.Join(home, ".codex", "config.toml")); err == nil {
			assistantList = append(assistantList, "Codex")
		}
	}

	assistantHooks := "Not Configured"
	if len(assistantList) > 0 {
		assistantHooks = strings.Join(assistantList, ", ")
	}

	bundledSkillsCount := len(skills.BundledSkillsCatalog())

	return &StatusResult{
		Version:        CLIVersion,
		ServerURL:      cfg.ServerURL,
		AuthMode:       authMode,
		UserEmail:      cfg.UserEmail,
		WorkspaceID:    cfg.WorkspaceID,
		TeamKeyStatus:  teamKey,
		Repository:     repoPath,
		CurrentBranch:  branch,
		PreCommitHook:  preCommit,
		PrePushHook:    prePush,
		AssistantHooks: assistantHooks,
		BundledSkills:  bundledSkillsCount,
	}, nil
}

// PrintStatus formats and prints the consolidated developer status to stdout.
func PrintStatus(workDir string) error {
	st, err := GetStatus(workDir)
	if err != nil {
		return err
	}

	fmt.Println("\033[1mScanDrix CLI Status\033[0m")
	fmt.Println()
	fmt.Printf("  \033[2mVersion:\033[0m         %s\n", st.Version)
	fmt.Printf("  \033[2mServer:\033[0m          %s\n", st.ServerURL)
	fmt.Printf("  \033[2mAuth:\033[0m            %s\n", st.AuthMode)
	if st.UserEmail != "" {
		fmt.Printf("  \033[2mUser:\033[0m            %s\n", st.UserEmail)
	}
	fmt.Printf("  \033[2mTeam key:\033[0m        %s\n", st.TeamKeyStatus)
	fmt.Printf("  \033[2mRepository:\033[0m      %s (%s)\n", st.Repository, st.CurrentBranch)
	fmt.Printf("  \033[2mPre-commit:\033[0m      %s\n", st.PreCommitHook)
	fmt.Printf("  \033[2mPre-push:\033[0m        %s\n", st.PrePushHook)
	fmt.Printf("  \033[2mAssistant hooks:\033[0m %s\n", st.AssistantHooks)
	fmt.Printf("  \033[2mBundled skills:\033[0m  %d\n", st.BundledSkills)
	return nil
}
