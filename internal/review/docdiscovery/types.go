// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package docdiscovery

import (
	"fmt"
	"strings"
	"time"
)

// ManifestKind identifies the package management ecosystem of a dependency manifest.
type ManifestKind string

const (
	ManifestGoMod           ManifestKind = "go_mod"
	ManifestPackageJSON     ManifestKind = "package_json"
	ManifestRequirementsTxt ManifestKind = "requirements_txt"
	ManifestPyprojectToml   ManifestKind = "pyproject_toml"
	ManifestCargoToml       ManifestKind = "cargo_toml"
	ManifestPomXML          ManifestKind = "pom_xml"
	ManifestGradle          ManifestKind = "gradle"
	ManifestGemfile         ManifestKind = "gemfile"
	ManifestUnknown         ManifestKind = "unknown"
)

// DependencyType categorizes direct vs transitive or development dependencies.
type DependencyType string

const (
	DepDirect   DependencyType = "direct"
	DepDev      DependencyType = "dev"
	DepIndirect DependencyType = "indirect"
	DepPeer     DependencyType = "peer"
)

// DependencyChangeKind indicates how a dependency changed in a pull request diff.
type DependencyChangeKind string

const (
	ChangeAdded       DependencyChangeKind = "added"
	ChangeUpgraded    DependencyChangeKind = "upgraded"
	ChangeDowngraded  DependencyChangeKind = "downgraded"
	ChangeRemoved     DependencyChangeKind = "removed"
	ChangeUnchanged   DependencyChangeKind = "unchanged"
)

// PackageDependency describes a single library or framework package requirement.
type PackageDependency struct {
	Name            string               `json:"name"`
	Version         string               `json:"version"`
	PreviousVersion string               `json:"previous_version,omitempty"`
	Type            DependencyType       `json:"type"`
	ChangeKind      DependencyChangeKind `json:"change_kind"`
	ManifestPath    string               `json:"manifest_path"`
}

// DiscoveredManifest holds all dependencies extracted from a manifest file.
type DiscoveredManifest struct {
	Path         string              `json:"path"`
	Kind         ManifestKind        `json:"kind"`
	Dependencies []PackageDependency `json:"dependencies"`
}

// DocumentationSnippet represents cached or fetched API documentation for a package.
type DocumentationSnippet struct {
	PackageName    string    `json:"package_name"`
	Version        string    `json:"version,omitempty"`
	Title          string    `json:"title"`
	URL            string    `json:"url,omitempty"`
	Content        string    `json:"content"`
	RelevanceScore float64   `json:"relevance_score"`
	FetchedAt      time.Time `json:"fetched_at"`
}

// EstimateTokens provides a lightweight token count approximation.
func (s *DocumentationSnippet) EstimateTokens() int {
	textLen := len(s.PackageName) + len(s.Title) + len(s.Content)
	return (textLen + 3) / 4
}

// DocumentationPack bundles package changes and relevant API snippets for the review prompt.
type DocumentationPack struct {
	ModifiedPackages     []PackageDependency    `json:"modified_packages"`
	Snippets             []DocumentationSnippet `json:"snippets"`
	TotalEstimatedTokens int                    `json:"total_estimated_tokens"`
}

// FormatPromptSlice formats documentation context into an LLM reviewer markdown slice.
func (p *DocumentationPack) FormatPromptSlice() string {
	if len(p.ModifiedPackages) == 0 && len(p.Snippets) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("### Package Dependencies & Official API Reference Context\n")
	sb.WriteString("The pull request modifies or relies upon the following third-party dependencies:\n\n")

	if len(p.ModifiedPackages) > 0 {
		sb.WriteString("#### Dependency Modifications:\n")
		for _, pkg := range p.ModifiedPackages {
			switch pkg.ChangeKind {
			case ChangeAdded:
				sb.WriteString(fmt.Sprintf("- **%s** `v%s` (*Added* in `%s`)\n", pkg.Name, pkg.Version, pkg.ManifestPath))
			case ChangeUpgraded:
				sb.WriteString(fmt.Sprintf("- **%s** `v%s` -> `v%s` (*Upgraded* in `%s`)\n", pkg.Name, pkg.PreviousVersion, pkg.Version, pkg.ManifestPath))
			case ChangeDowngraded:
				sb.WriteString(fmt.Sprintf("- **%s** `v%s` -> `v%s` (*Downgraded* in `%s`)\n", pkg.Name, pkg.PreviousVersion, pkg.Version, pkg.ManifestPath))
			case ChangeRemoved:
				sb.WriteString(fmt.Sprintf("- **%s** `v%s` (*Removed* from `%s`)\n", pkg.Name, pkg.PreviousVersion, pkg.ManifestPath))
			}
		}
		sb.WriteString("\n")
	}

	if len(p.Snippets) > 0 {
		sb.WriteString("#### Official Documentation & API Signatures:\n")
		for i, s := range p.Snippets {
			sb.WriteString(fmt.Sprintf("%d. **[%s] %s**\n", i+1, s.PackageName, s.Title))
			if s.URL != "" {
				sb.WriteString(fmt.Sprintf("   *Source*: %s\n", s.URL))
			}
			sb.WriteString(fmt.Sprintf("```\n%s\n```\n\n", strings.TrimSpace(s.Content)))
		}
	}

	return sb.String()
}
