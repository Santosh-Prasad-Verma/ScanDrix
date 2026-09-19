// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package docdiscovery

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/scandrix/backend/internal/review/diff"
)

var (
	goImportRegex = regexp.MustCompile(`"([^"]+)"`)
	tsImportRegex = regexp.MustCompile(`(?:import|from)\s+['"]([^'"]+)['"]`)
	pyImportRegex = regexp.MustCompile(`(?:import|from)\s+([a-zA-Z0-9_.]+)`)
)

// DocQueryTask represents an individual documentation inquiry for a package.
type DocQueryTask struct {
	PackageName     string `json:"package_name"`
	Version         string `json:"version"`
	PreviousVersion string `json:"previous_version,omitempty"`
	SourceFile      string `json:"source_file"`
	Query           string `json:"query"`
	Priority        int    `json:"priority"` // Higher = more urgent
}

// DocQueryPlanner correlates changed source code with manifest dependencies to plan documentation retrieval.
type DocQueryPlanner struct{}

// NewDocQueryPlanner constructs a documentation query planner.
func NewDocQueryPlanner() *DocQueryPlanner {
	return &DocQueryPlanner{}
}

// PlanDocumentationQueries analyzes modified patches and active packages to formulate targeted documentation tasks.
func (p *DocQueryPlanner) PlanDocumentationQueries(
	patches []*diff.FilePatch,
	packages []PackageDependency,
) []DocQueryTask {
	if len(packages) == 0 || len(patches) == 0 {
		return nil
	}

	pkgMap := make(map[string]PackageDependency)
	for _, pkg := range packages {
		pkgMap[pkg.Name] = pkg
	}

	var tasks []DocQueryTask
	seenTasks := make(map[string]bool)

	for _, patch := range patches {
		targetPath := patch.NewPath
		if targetPath == "" {
			targetPath = patch.OldPath
		}
		if targetPath == "" || isManifestPath(targetPath) {
			continue
		}

		imports := p.extractImportsFromPatch(patch)
		for _, imp := range imports {
			matchedPkg, found := p.matchImportToPackage(imp, pkgMap)
			if !found {
				continue
			}

			taskKey := fmt.Sprintf("%s:%s", matchedPkg.Name, targetPath)
			if seenTasks[taskKey] {
				continue
			}
			seenTasks[taskKey] = true

			priority := 1
			query := fmt.Sprintf("%s %s API documentation and usage", matchedPkg.Name, matchedPkg.Version)

			if matchedPkg.ChangeKind == ChangeUpgraded {
				priority = 3
				query = fmt.Sprintf("%s breaking changes migration from %s to %s", matchedPkg.Name, matchedPkg.PreviousVersion, matchedPkg.Version)
			} else if matchedPkg.ChangeKind == ChangeAdded {
				priority = 2
				query = fmt.Sprintf("%s %s getting started best practices and security caveats", matchedPkg.Name, matchedPkg.Version)
			}

			tasks = append(tasks, DocQueryTask{
				PackageName:     matchedPkg.Name,
				Version:         matchedPkg.Version,
				PreviousVersion: matchedPkg.PreviousVersion,
				SourceFile:      targetPath,
				Query:           query,
				Priority:        priority,
			})
		}
	}

	return tasks
}

func isManifestPath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return base == "go.mod" || base == "package.json" || base == "cargo.toml" || strings.HasSuffix(base, "requirements.txt")
}

func (p *DocQueryPlanner) extractImportsFromPatch(patch *diff.FilePatch) []string {
	var imports []string
	ext := strings.ToLower(filepath.Ext(patch.NewPath))

	for _, hunk := range patch.Hunks {
		for _, line := range hunk.Lines {
			if line.Type != diff.LineAddition {
				continue
			}
			content := strings.TrimSpace(line.Content)

			switch ext {
			case ".go":
				if strings.HasPrefix(content, "import") || strings.Contains(content, "\"") {
					for _, m := range goImportRegex.FindAllStringSubmatch(content, -1) {
						if len(m) > 1 {
							imports = append(imports, m[1])
						}
					}
				}
			case ".ts", ".js", ".tsx", ".jsx":
				if strings.Contains(content, "import") || strings.Contains(content, "require") {
					for _, m := range tsImportRegex.FindAllStringSubmatch(content, -1) {
						if len(m) > 1 {
							imports = append(imports, m[1])
						}
					}
				}
			case ".py":
				if strings.HasPrefix(content, "import ") || strings.HasPrefix(content, "from ") {
					for _, m := range pyImportRegex.FindAllStringSubmatch(content, -1) {
						if len(m) > 1 {
							imports = append(imports, m[1])
						}
					}
				}
			}
		}
	}

	return imports
}

func (p *DocQueryPlanner) matchImportToPackage(imp string, pkgs map[string]PackageDependency) (PackageDependency, bool) {
	// Exact match
	if d, ok := pkgs[imp]; ok {
		return d, true
	}

	// Prefix match (e.g. import "github.com/gin-gonic/gin/binding" matches "github.com/gin-gonic/gin")
	for name, d := range pkgs {
		if strings.HasPrefix(imp, name+"/") || strings.HasPrefix(name, imp+"/") {
			return d, true
		}
	}

	return PackageDependency{}, false
}
