// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"testing"
)

func TestExtractOrgRepoFromRemote(t *testing.T) {
	tests := []struct {
		url     string
		wantOrg string
		wantRepo string
		wantOk  bool
	}{
		{"git@github.com:scandrix/engine.git", "scandrix", "engine", true},
		{"https://github.com/scandrix/engine.git", "scandrix", "engine", true},
		{"https://github.com/scandrix/engine", "scandrix", "engine", true},
		{"git@gitlab.com:my-org/sub-group/my-repo.git", "my-org", "my-repo", true},
		{"git@ssh.dev.azure.com:v3/myorg/myproject/myrepo", "myorg", "myrepo", true},
		{"https://dev.azure.com/myorg/myproject/_git/myrepo", "myorg", "myrepo", true},
		{"https://myorg.visualstudio.com/myproject/_git/myrepo", "myorg", "myrepo", true},
		{"https://git.corp.internal/scm/PRJ/myrepo.git", "PRJ", "myrepo", true},
		{"", "", "", false},
		{"invalid-url-string", "", "", false},
	}

	for _, tt := range tests {
		org, repo, ok := ExtractOrgRepoFromRemote(tt.url)
		if ok != tt.wantOk {
			t.Errorf("ExtractOrgRepoFromRemote(%q) ok = %v, want %v", tt.url, ok, tt.wantOk)
		}
		if ok {
			if org != tt.wantOrg {
				t.Errorf("ExtractOrgRepoFromRemote(%q) org = %q, want %q", tt.url, org, tt.wantOrg)
			}
			if repo != tt.wantRepo {
				t.Errorf("ExtractOrgRepoFromRemote(%q) repo = %q, want %q", tt.url, repo, tt.wantRepo)
			}
		}
	}
}

func TestInferPlatformFromRemote(t *testing.T) {
	tests := []struct {
		url          string
		wantPlatform PlatformType
	}{
		{"git@github.com:scandrix/engine.git", PlatformGitHub},
		{"https://gitlab.com/group/repo.git", PlatformGitLab},
		{"git@bitbucket.org:team/repo.git", PlatformBitbucket},
		{"https://dev.azure.com/myorg/proj/_git/repo", PlatformAzureRepos},
		{"https://forgejo.community.org/user/repo", PlatformForgejo},
		{"https://unknown-host.example.com/repo.git", PlatformUnknown},
		{"", PlatformUnknown},
	}

	for _, tt := range tests {
		got := InferPlatformFromRemote(tt.url)
		if got != tt.wantPlatform {
			t.Errorf("InferPlatformFromRemote(%q) = %v, want %v", tt.url, got, tt.wantPlatform)
		}
	}
}

func TestBuildWebRepoURL(t *testing.T) {
	tests := []struct {
		remote  string
		wantURL string
	}{
		{"git@github.com:scandrix/engine.git", "https://github.com/scandrix/engine"},
		{"https://gitlab.com/group/repo.git", "https://gitlab.com/group/repo"},
		{"https://dev.azure.com/myorg/proj/_git/repo", "https://dev.azure.com/myorg/_git/repo"},
	}

	for _, tt := range tests {
		got := BuildWebRepoURL(tt.remote)
		if got != tt.wantURL {
			t.Errorf("BuildWebRepoURL(%q) = %q, want %q", tt.remote, got, tt.wantURL)
		}
	}
}
