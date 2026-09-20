// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"net/url"
	"regexp"
	"strings"
)

// PlatformType identifies the Git hosting provider.
type PlatformType string

const (
	PlatformGitHub      PlatformType = "GITHUB"
	PlatformGitLab      PlatformType = "GITLAB"
	PlatformBitbucket   PlatformType = "BITBUCKET"
	PlatformAzureRepos  PlatformType = "AZURE_REPOS"
	PlatformForgejo     PlatformType = "FORGEJO"
	PlatformUnknown     PlatformType = "UNKNOWN"
)

var (
	azureSshRegex = regexp.MustCompile(`(?:ssh\.dev\.azure\.com|vs-ssh\.visualstudio\.com)[:/]v3/([^/]+)/[^/]+/([^/.]+)`)
	azureHttpsNewRegex = regexp.MustCompile(`dev\.azure\.com/([^/]+)/[^/]+/_git/([^/.?]+)`)
	azureHttpsOldRegex = regexp.MustCompile(`^https?://([^.]+)\.visualstudio\.com/[^/]+/_git/([^/.?]+)`)
	bitbucketServerRegex = regexp.MustCompile(`/scm/([^/]+)/([^/.]+)`)
	sshScpRegex = regexp.MustCompile(`^[^@/]+@[^:]+:(.+)`)
	sshProtoRegex = regexp.MustCompile(`^ssh://[^/]+/(.+)`)
	httpsProtoRegex = regexp.MustCompile(`^https?://[^/]+/(.+)`)
)

// ExtractOrgRepoFromRemote parses git remote URL and extracts org/workspace and repo name.
func ExtractOrgRepoFromRemote(remoteURL string) (org, repo string, ok bool) {
	u := strings.TrimSpace(remoteURL)
	if u == "" {
		return "", "", false
	}

	// Azure DevOps SSH
	if m := azureSshRegex.FindStringSubmatch(u); len(m) == 3 {
		return m[1], m[2], true
	}

	// Azure DevOps HTTPS (new)
	if m := azureHttpsNewRegex.FindStringSubmatch(u); len(m) == 3 {
		return m[1], m[2], true
	}

	// Azure DevOps HTTPS (old)
	if m := azureHttpsOldRegex.FindStringSubmatch(u); len(m) == 3 {
		return m[1], m[2], true
	}

	// Bitbucket Server (self-hosted)
	if m := bitbucketServerRegex.FindStringSubmatch(u); len(m) == 3 {
		return m[1], m[2], true
	}

	// SSH SCP-like: git@github.com:org/repo.git
	if m := sshScpRegex.FindStringSubmatch(u); len(m) == 2 {
		parts := splitCleanPath(m[1])
		if len(parts) >= 2 {
			return parts[0], parts[len(parts)-1], true
		}
	}

	// SSH protocol: ssh://git@host/org/repo.git
	if m := sshProtoRegex.FindStringSubmatch(u); len(m) == 2 {
		parts := splitCleanPath(m[1])
		if len(parts) >= 2 {
			return parts[0], parts[len(parts)-1], true
		}
	}

	// HTTPS: https://github.com/org/repo.git
	if m := httpsProtoRegex.FindStringSubmatch(u); len(m) == 2 {
		parts := splitCleanPath(m[1])
		if len(parts) >= 2 {
			return parts[0], parts[len(parts)-1], true
		}
	}

	return "", "", false
}

func splitCleanPath(p string) []string {
	clean := strings.TrimSuffix(p, ".git")
	clean = strings.Split(clean, "?")[0]
	var res []string
	for _, s := range strings.Split(clean, "/") {
		s = strings.TrimSpace(s)
		if s != "" {
			res = append(res, s)
		}
	}
	return res
}

// ExtractRemoteHost extracts the lower-cased hostname from a remote URL.
func ExtractRemoteHost(remote string) string {
	val := strings.TrimSpace(strings.ToLower(remote))
	if val == "" {
		return ""
	}

	if parsed, err := url.Parse(val); err == nil && parsed.Hostname() != "" {
		return strings.ToLower(parsed.Hostname())
	}

	// Check SCP-like syntax git@github.com:owner/repo
	scpRegex := regexp.MustCompile(`^(?:[^@/]+@)?([^:/]+):.+`)
	if m := scpRegex.FindStringSubmatch(val); len(m) == 2 {
		return strings.ToLower(m[1])
	}

	return ""
}

// InferPlatformFromRemote determines the VCS platform from a remote URL.
func InferPlatformFromRemote(remote string) PlatformType {
	host := ExtractRemoteHost(remote)
	if host == "" {
		return PlatformUnknown
	}

	switch {
	case host == "github.com":
		return PlatformGitHub
	case host == "gitlab.com":
		return PlatformGitLab
	case host == "bitbucket.org":
		return PlatformBitbucket
	case host == "dev.azure.com" || host == "ssh.dev.azure.com" || host == "visualstudio.com" || strings.HasSuffix(host, ".visualstudio.com"):
		return PlatformAzureRepos
	case strings.Contains(host, "forgejo") || strings.Contains(host, "gitea"):
		return PlatformForgejo
	default:
		return PlatformUnknown
	}
}

// BuildWebRepoURL constructs the browser URL for a given remote repository.
func BuildWebRepoURL(remote string) string {
	org, repo, ok := ExtractOrgRepoFromRemote(remote)
	if !ok {
		return ""
	}

	platform := InferPlatformFromRemote(remote)
	switch platform {
	case PlatformGitHub:
		return "https://github.com/" + org + "/" + repo
	case PlatformGitLab:
		return "https://gitlab.com/" + org + "/" + repo
	case PlatformBitbucket:
		return "https://bitbucket.org/" + org + "/" + repo
	case PlatformAzureRepos:
		return "https://dev.azure.com/" + org + "/_git/" + repo
	default:
		host := ExtractRemoteHost(remote)
		if host != "" {
			return "https://" + host + "/" + org + "/" + repo
		}
		return ""
	}
}
