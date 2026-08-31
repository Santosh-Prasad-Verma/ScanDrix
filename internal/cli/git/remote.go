package git

import (
	"bytes"
	"context"
	"net/url"
	"os/exec"
	"strings"

	"github.com/scandrix/backend/pkg/models"
)

// RemoteInfo holds the parsed Git remote details.
type RemoteInfo struct {
	Provider      models.SCMProvider `json:"provider"`
	NamespacePath string             `json:"namespace_path"` // e.g. "kodustech/kodus-ai"
	DefaultBranch string             `json:"default_branch"` // e.g. "main"
	RemoteURL     string             `json:"remote_url"`
}

// DetectRemote detects git remote configuration from the working directory.
func DetectRemote(ctx context.Context, workDir string) (*RemoteInfo, error) {
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", "origin")
	if workDir != "" {
		cmd.Dir = workDir
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	remoteURL := strings.TrimSpace(out.String())
	return ParseRemoteURL(remoteURL), nil
}

// ParseRemoteURL extracts provider and namespace path from any standard Git remote URL.
func ParseRemoteURL(raw string) *RemoteInfo {
	info := &RemoteInfo{
		Provider:      models.ProviderGitHub,
		DefaultBranch: "main",
		RemoteURL:     raw,
	}

	clean := strings.TrimSpace(raw)
	clean = strings.TrimSuffix(clean, ".git")

	// Detect provider & clean SSH vs HTTPS
	if strings.Contains(clean, "gitlab.com") {
		info.Provider = models.ProviderGitLab
	} else if strings.Contains(clean, "bitbucket.org") {
		info.Provider = models.ProviderBitbucket
	} else if strings.Contains(clean, "dev.azure.com") || strings.Contains(clean, "visualstudio.com") {
		info.Provider = models.ProviderAzure
	} else if strings.Contains(clean, "codeberg.org") {
		info.Provider = models.ProviderForgejo
	} else {
		info.Provider = models.ProviderGitHub
	}

	// Case 1: SSH format: git@github.com:owner/repo
	if strings.HasPrefix(clean, "git@") {
		parts := strings.Split(clean, ":")
		if len(parts) == 2 {
			info.NamespacePath = parts[1]
			return info
		}
	}

	// Case 2: HTTPS format: https://github.com/owner/repo
	if strings.HasPrefix(clean, "http://") || strings.HasPrefix(clean, "https://") {
		if parsed, err := url.Parse(clean); err == nil {
			info.NamespacePath = strings.TrimPrefix(parsed.Path, "/")
			return info
		}
	}

	info.NamespacePath = clean
	return info
}
