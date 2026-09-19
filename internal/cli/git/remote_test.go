package git_test

import (
	"testing"

	"github.com/scandrix/backend/internal/cli/git"
	"github.com/scandrix/backend/pkg/models"
)

func TestParseRemoteURL(t *testing.T) {
	tests := []struct {
		url          string
		wantProvider models.SCMProvider
		wantPath     string
	}{
		{
			url:          "git@github.com:scandrix/scandrix-core.git",
			wantProvider: models.ProviderGitHub,
			wantPath:     "scandrix/scandrix-core",
		},
		{
			url:          "https://github.com/scandrix/backend.git",
			wantProvider: models.ProviderGitHub,
			wantPath:     "scandrix/backend",
		},
		{
			url:          "git@gitlab.com:enterprise-org/core-service.git",
			wantProvider: models.ProviderGitLab,
			wantPath:     "enterprise-org/core-service",
		},
		{
			url:          "https://codeberg.org/forgejo/forgejo.git",
			wantProvider: models.ProviderForgejo,
			wantPath:     "forgejo/forgejo",
		},
	}

	for _, tt := range tests {
		info := git.ParseRemoteURL(tt.url)
		if info.Provider != tt.wantProvider {
			t.Errorf("url %s: got provider %s, want %s", tt.url, info.Provider, tt.wantProvider)
		}
		if info.NamespacePath != tt.wantPath {
			t.Errorf("url %s: got path %s, want %s", tt.url, info.NamespacePath, tt.wantPath)
		}
	}
}
