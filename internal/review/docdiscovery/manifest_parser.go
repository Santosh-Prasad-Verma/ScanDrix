// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package docdiscovery

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/scandrix/backend/internal/review/diff"
)

var (
	goModRequireRegex = regexp.MustCompile(`^\s*([a-zA-Z0-9.\-_/]+)\s+v?([a-zA-Z0-9.\-_+]+)(?:\s+//\s*(indirect))?`)
	pythonReqRegex    = regexp.MustCompile(`^\s*([a-zA-Z0-9.\-_]+)\s*(?:[=><~]=?|@)\s*([a-zA-Z0-9.\-_+]+)`)
	cargoDepRegex     = regexp.MustCompile(`^\s*([a-zA-Z0-9.\-_]+)\s*=\s*(?:\{.*version\s*=\s*["']([^"']+)["'].*\}|["']([^"']+)["'])`)
	gradleDepRegex    = regexp.MustCompile(`(?:implementation|api|testImplementation)\s*\(?['"]([^:'"]+):([^:'"]+):([^'"]+)['"]\)?`)
	gemfileRegex      = regexp.MustCompile(`^\s*gem\s+['"]([^'"]+)['"](?:\s*,\s*['"]([^'"]+)['"])?`)
)

// ManifestParser parses dependency declarations and analyzes diffs across diverse package ecosystems.
type ManifestParser struct{}

// NewManifestParser constructs a new manifest parser.
func NewManifestParser() *ManifestParser {
	return &ManifestParser{}
}

// DetectManifestType determines the ecosystem kind from a relative file path.
func (p *ManifestParser) DetectManifestType(filePath string) ManifestKind {
	base := strings.ToLower(filepath.Base(filePath))
	clean := strings.ToLower(filepath.ToSlash(filePath))

	switch {
	case base == "go.mod":
		return ManifestGoMod
	case base == "package.json":
		return ManifestPackageJSON
	case base == "requirements.txt" || strings.HasSuffix(base, "requirements.txt"):
		return ManifestRequirementsTxt
	case base == "pyproject.toml":
		return ManifestPyprojectToml
	case base == "cargo.toml":
		return ManifestCargoToml
	case base == "pom.xml":
		return ManifestPomXML
	case base == "build.gradle" || base == "build.gradle.kts":
		return ManifestGradle
	case base == "gemfile":
		return ManifestGemfile
	default:
		if strings.Contains(clean, "requirements") && strings.HasSuffix(clean, ".txt") {
			return ManifestRequirementsTxt
		}
		return ManifestUnknown
	}
}

// IsSupportedManifest checks if a file is a recognized dependency manifest.
func (p *ManifestParser) IsSupportedManifest(filePath string) bool {
	return p.DetectManifestType(filePath) != ManifestUnknown
}

// ParseManifestContent extracts all dependencies from full file content.
func (p *ManifestParser) ParseManifestContent(path string, content []byte) (*DiscoveredManifest, error) {
	kind := p.DetectManifestType(path)
	text := string(content)

	var deps []PackageDependency
	switch kind {
	case ManifestGoMod:
		deps = p.parseGoMod(path, text)
	case ManifestPackageJSON:
		deps = p.parsePackageJSON(path, content)
	case ManifestRequirementsTxt:
		deps = p.parseRequirementsTxt(path, text)
	case ManifestCargoToml:
		deps = p.parseCargoToml(path, text)
	case ManifestGradle:
		deps = p.parseGradle(path, text)
	case ManifestGemfile:
		deps = p.parseGemfile(path, text)
	default:
		// Unknown or unsupported
	}

	return &DiscoveredManifest{
		Path:         path,
		Kind:         kind,
		Dependencies: deps,
	}, nil
}

// AnalyzeManifestDiff extracts added, upgraded, or removed dependencies from a diff patch.
func (p *ManifestParser) AnalyzeManifestDiff(patch *diff.FilePatch) ([]PackageDependency, error) {
	if patch == nil {
		return nil, nil
	}

	targetPath := patch.NewPath
	if targetPath == "" {
		targetPath = patch.OldPath
	}

	kind := p.DetectManifestType(targetPath)
	if kind == ManifestUnknown {
		return nil, nil
	}

	var deletedLines []string
	var addedLines []string

	for _, hunk := range patch.Hunks {
		for _, line := range hunk.Lines {
			switch line.Type {
			case diff.LineDeletion:
				deletedLines = append(deletedLines, line.Content)
			case diff.LineAddition:
				addedLines = append(addedLines, line.Content)
			}
		}
	}

	oldContent := strings.Join(deletedLines, "\n")
	newContent := strings.Join(addedLines, "\n")

	oldManifest, _ := p.ParseManifestContent(targetPath, []byte(oldContent))
	newManifest, _ := p.ParseManifestContent(targetPath, []byte(newContent))

	oldMap := make(map[string]PackageDependency)
	if oldManifest != nil {
		for _, d := range oldManifest.Dependencies {
			oldMap[d.Name] = d
		}
	}

	var results []PackageDependency
	newMap := make(map[string]PackageDependency)

	if newManifest != nil {
		for _, d := range newManifest.Dependencies {
			newMap[d.Name] = d
			if old, exists := oldMap[d.Name]; exists {
				if old.Version != d.Version {
					d.PreviousVersion = old.Version
					d.ChangeKind = ChangeUpgraded
					results = append(results, d)
				}
			} else {
				d.ChangeKind = ChangeAdded
				results = append(results, d)
			}
		}
	}

	for name, old := range oldMap {
		if _, exists := newMap[name]; !exists {
			old.PreviousVersion = old.Version
			old.ChangeKind = ChangeRemoved
			results = append(results, old)
		}
	}

	return results, nil
}

// ─────────────────────────────────────────────────────────────
// Specific Manifest Parsers
// ─────────────────────────────────────────────────────────────

func (p *ManifestParser) parseGoMod(path, text string) []PackageDependency {
	lines := strings.Split(text, "\n")
	var deps []PackageDependency
	inRequireBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "require (") {
			inRequireBlock = true
			continue
		}
		if inRequireBlock && trimmed == ")" {
			inRequireBlock = false
			continue
		}

		if inRequireBlock || strings.HasPrefix(trimmed, "require ") {
			clean := strings.TrimPrefix(trimmed, "require ")
			m := goModRequireRegex.FindStringSubmatch(clean)
			if len(m) > 2 {
				name := m[1]
				version := m[2]
				isIndirect := len(m) > 3 && m[3] == "indirect"

				depType := DepDirect
				if isIndirect {
					depType = DepIndirect
				}

				deps = append(deps, PackageDependency{
					Name:         name,
					Version:      version,
					Type:         depType,
					ChangeKind:   ChangeUnchanged,
					ManifestPath: path,
				})
			}
		}
	}
	return deps
}

type packageJSONStruct struct {
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	PeerDependencies map[string]string `json:"peerDependencies"`
}

func (p *ManifestParser) parsePackageJSON(path string, content []byte) []PackageDependency {
	var pkg packageJSONStruct
	if err := json.Unmarshal(content, &pkg); err != nil {
		return p.parsePackageJSONRegex(path, string(content))
	}

	var deps []PackageDependency
	for name, ver := range pkg.Dependencies {
		deps = append(deps, PackageDependency{
			Name:         name,
			Version:      cleanVersion(ver),
			Type:         DepDirect,
			ChangeKind:   ChangeUnchanged,
			ManifestPath: path,
		})
	}
	for name, ver := range pkg.DevDependencies {
		deps = append(deps, PackageDependency{
			Name:         name,
			Version:      cleanVersion(ver),
			Type:         DepDev,
			ChangeKind:   ChangeUnchanged,
			ManifestPath: path,
		})
	}
	for name, ver := range pkg.PeerDependencies {
		deps = append(deps, PackageDependency{
			Name:         name,
			Version:      cleanVersion(ver),
			Type:         DepPeer,
			ChangeKind:   ChangeUnchanged,
			ManifestPath: path,
		})
	}
	return deps
}

func (p *ManifestParser) parsePackageJSONRegex(path, text string) []PackageDependency {
	lineRegex := regexp.MustCompile(`"([^"]+)"\s*:\s*"([^"]+)"`)
	lines := strings.Split(text, "\n")
	var deps []PackageDependency

	for _, line := range lines {
		m := lineRegex.FindStringSubmatch(line)
		if len(m) == 3 {
			name := m[1]
			ver := m[2]
			if name != "version" && name != "name" && name != "description" {
				deps = append(deps, PackageDependency{
					Name:         name,
					Version:      cleanVersion(ver),
					Type:         DepDirect,
					ChangeKind:   ChangeUnchanged,
					ManifestPath: path,
				})
			}
		}
	}
	return deps
}

func (p *ManifestParser) parseRequirementsTxt(path, text string) []PackageDependency {
	lines := strings.Split(text, "\n")
	var deps []PackageDependency

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "-r ") {
			continue
		}

		m := pythonReqRegex.FindStringSubmatch(trimmed)
		if len(m) >= 3 {
			deps = append(deps, PackageDependency{
				Name:         m[1],
				Version:      m[2],
				Type:         DepDirect,
				ChangeKind:   ChangeUnchanged,
				ManifestPath: path,
			})
		} else {
			// Single name without pinned version
			parts := strings.Fields(trimmed)
			if len(parts) > 0 {
				deps = append(deps, PackageDependency{
					Name:         parts[0],
					Version:      "latest",
					Type:         DepDirect,
					ChangeKind:   ChangeUnchanged,
					ManifestPath: path,
				})
			}
		}
	}
	return deps
}

func (p *ManifestParser) parseCargoToml(path, text string) []PackageDependency {
	lines := strings.Split(text, "\n")
	var deps []PackageDependency
	inDepBlock := false
	isDev := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[dependencies]") {
			inDepBlock = true
			isDev = false
			continue
		}
		if strings.HasPrefix(trimmed, "[dev-dependencies]") {
			inDepBlock = true
			isDev = true
			continue
		}
		if strings.HasPrefix(trimmed, "[") && inDepBlock {
			inDepBlock = false
			continue
		}

		if inDepBlock {
			m := cargoDepRegex.FindStringSubmatch(trimmed)
			if len(m) >= 3 {
				name := m[1]
				ver := m[2]
				if ver == "" && len(m) > 3 {
					ver = m[3]
				}
				depType := DepDirect
				if isDev {
					depType = DepDev
				}

				deps = append(deps, PackageDependency{
					Name:         name,
					Version:      cleanVersion(ver),
					Type:         depType,
					ChangeKind:   ChangeUnchanged,
					ManifestPath: path,
				})
			}
		}
	}
	return deps
}

func (p *ManifestParser) parseGradle(path, text string) []PackageDependency {
	lines := strings.Split(text, "\n")
	var deps []PackageDependency

	for _, line := range lines {
		m := gradleDepRegex.FindStringSubmatch(line)
		if len(m) >= 4 {
			name := m[1] + ":" + m[2]
			ver := m[3]
			deps = append(deps, PackageDependency{
				Name:         name,
				Version:      cleanVersion(ver),
				Type:         DepDirect,
				ChangeKind:   ChangeUnchanged,
				ManifestPath: path,
			})
		}
	}
	return deps
}

func (p *ManifestParser) parseGemfile(path, text string) []PackageDependency {
	lines := strings.Split(text, "\n")
	var deps []PackageDependency

	for _, line := range lines {
		m := gemfileRegex.FindStringSubmatch(line)
		if len(m) >= 2 {
			name := m[1]
			ver := "latest"
			if len(m) >= 3 && m[2] != "" {
				ver = cleanVersion(m[2])
			}
			deps = append(deps, PackageDependency{
				Name:         name,
				Version:      ver,
				Type:         DepDirect,
				ChangeKind:   ChangeUnchanged,
				ManifestPath: path,
			})
		}
	}
	return deps
}

func cleanVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "^")
	v = strings.TrimPrefix(v, "~")
	v = strings.TrimPrefix(v, ">=")
	v = strings.TrimPrefix(v, "==")
	v = strings.TrimPrefix(v, "=")
	v = strings.TrimPrefix(v, "v")
	return v
}
