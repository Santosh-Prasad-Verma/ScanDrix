// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package docdiscovery_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/review/docdiscovery"
)

func TestManifestParser_ParseGoMod(t *testing.T) {
	parser := docdiscovery.NewManifestParser()

	content := `
module github.com/scandrix/backend

go 1.22.0

require (
	github.com/google/uuid v1.6.0
	github.com/gin-gonic/gin v1.9.1
	github.com/stretchr/testify v1.8.4 // indirect
)
`

	manifest, err := parser.ParseManifestContent("go.mod", []byte(content))
	require.NoError(t, err)
	require.NotNil(t, manifest)

	assert.Equal(t, docdiscovery.ManifestGoMod, manifest.Kind)
	assert.Len(t, manifest.Dependencies, 3)

	dep1 := manifest.Dependencies[0]
	assert.Equal(t, "github.com/google/uuid", dep1.Name)
	assert.Equal(t, "1.6.0", dep1.Version)
	assert.Equal(t, docdiscovery.DepDirect, dep1.Type)

	dep3 := manifest.Dependencies[2]
	assert.Equal(t, "github.com/stretchr/testify", dep3.Name)
	assert.Equal(t, docdiscovery.DepIndirect, dep3.Type)
}

func TestManifestParser_ParsePackageJSON(t *testing.T) {
	parser := docdiscovery.NewManifestParser()

	content := `{
		"name": "web-dashboard",
		"version": "1.0.0",
		"dependencies": {
			"next": "^15.0.1",
			"react": "^19.0.0",
			"zod": "~3.23.8"
		},
		"devDependencies": {
			"typescript": "^5.4.0"
		}
	}`

	manifest, err := parser.ParseManifestContent("package.json", []byte(content))
	require.NoError(t, err)
	require.NotNil(t, manifest)

	assert.Equal(t, docdiscovery.ManifestPackageJSON, manifest.Kind)
	assert.Len(t, manifest.Dependencies, 4)

	var nextDep, tsDep *docdiscovery.PackageDependency
	for i := range manifest.Dependencies {
		if manifest.Dependencies[i].Name == "next" {
			nextDep = &manifest.Dependencies[i]
		}
		if manifest.Dependencies[i].Name == "typescript" {
			tsDep = &manifest.Dependencies[i]
		}
	}

	require.NotNil(t, nextDep)
	assert.Equal(t, "15.0.1", nextDep.Version)
	assert.Equal(t, docdiscovery.DepDirect, nextDep.Type)

	require.NotNil(t, tsDep)
	assert.Equal(t, "5.4.0", tsDep.Version)
	assert.Equal(t, docdiscovery.DepDev, tsDep.Type)
}

func TestManifestParser_ParseRequirementsTxt(t *testing.T) {
	parser := docdiscovery.NewManifestParser()

	content := `
# Production requirements
fastapi>=0.110.0
pydantic==2.6.4
uvicorn~=0.28.0
requests
`

	manifest, err := parser.ParseManifestContent("requirements.txt", []byte(content))
	require.NoError(t, err)
	require.NotNil(t, manifest)

	assert.Equal(t, docdiscovery.ManifestRequirementsTxt, manifest.Kind)
	assert.Len(t, manifest.Dependencies, 4)

	assert.Equal(t, "fastapi", manifest.Dependencies[0].Name)
	assert.Equal(t, "0.110.0", manifest.Dependencies[0].Version)

	assert.Equal(t, "requests", manifest.Dependencies[3].Name)
	assert.Equal(t, "latest", manifest.Dependencies[3].Version)
}

func TestManifestParser_ParseCargoToml(t *testing.T) {
	parser := docdiscovery.NewManifestParser()

	content := `
[package]
name = "engine"
version = "0.1.0"

[dependencies]
tokio = { version = "1.37.0", features = ["full"] }
serde = "1.0.200"

[dev-dependencies]
criterion = "0.5.1"
`

	manifest, err := parser.ParseManifestContent("Cargo.toml", []byte(content))
	require.NoError(t, err)
	require.NotNil(t, manifest)

	assert.Equal(t, docdiscovery.ManifestCargoToml, manifest.Kind)
	assert.Len(t, manifest.Dependencies, 3)

	assert.Equal(t, "tokio", manifest.Dependencies[0].Name)
	assert.Equal(t, "1.37.0", manifest.Dependencies[0].Version)
	assert.Equal(t, docdiscovery.DepDirect, manifest.Dependencies[0].Type)

	assert.Equal(t, "criterion", manifest.Dependencies[2].Name)
	assert.Equal(t, docdiscovery.DepDev, manifest.Dependencies[2].Type)
}

func TestManifestParser_AnalyzeManifestDiff(t *testing.T) {
	parser := docdiscovery.NewManifestParser()

	patch := &diff.FilePatch{
		NewPath: "go.mod",
		Hunks: []diff.Hunk{
			{
				Lines: []diff.DiffLine{
					{Type: diff.LineDeletion, Content: "require github.com/google/uuid v1.5.0"},
					{Type: diff.LineAddition, Content: "require github.com/google/uuid v1.6.0"},
					{Type: diff.LineAddition, Content: "require github.com/gin-gonic/gin v1.9.1"},
					{Type: diff.LineDeletion, Content: "require github.com/old/lib v0.1.0"},
				},
			},
		},
	}

	diffDeps, err := parser.AnalyzeManifestDiff(patch)
	require.NoError(t, err)

	var upgraded, added, removed *docdiscovery.PackageDependency
	for i := range diffDeps {
		if diffDeps[i].Name == "github.com/google/uuid" {
			upgraded = &diffDeps[i]
		}
		if diffDeps[i].Name == "github.com/gin-gonic/gin" {
			added = &diffDeps[i]
		}
		if diffDeps[i].Name == "github.com/old/lib" {
			removed = &diffDeps[i]
		}
	}

	require.NotNil(t, upgraded)
	assert.Equal(t, docdiscovery.ChangeUpgraded, upgraded.ChangeKind)
	assert.Equal(t, "1.5.0", upgraded.PreviousVersion)
	assert.Equal(t, "1.6.0", upgraded.Version)

	require.NotNil(t, added)
	assert.Equal(t, docdiscovery.ChangeAdded, added.ChangeKind)
	assert.Equal(t, "1.9.1", added.Version)

	require.NotNil(t, removed)
	assert.Equal(t, docdiscovery.ChangeRemoved, removed.ChangeKind)
}

func TestDocQueryPlanner_PlanDocumentationQueries(t *testing.T) {
	planner := docdiscovery.NewDocQueryPlanner()

	packages := []docdiscovery.PackageDependency{
		{
			Name:            "github.com/gin-gonic/gin",
			Version:         "1.9.1",
			PreviousVersion: "1.8.0",
			ChangeKind:      docdiscovery.ChangeUpgraded,
		},
	}

	codePatch := &diff.FilePatch{
		NewPath: "server/router.go",
		Hunks: []diff.Hunk{
			{
				Lines: []diff.DiffLine{
					{Type: diff.LineAddition, Content: `import "github.com/gin-gonic/gin"`},
					{Type: diff.LineAddition, Content: `r := gin.Default()`},
				},
			},
		},
	}

	tasks := planner.PlanDocumentationQueries([]*diff.FilePatch{codePatch}, packages)
	require.Len(t, tasks, 1)

	task := tasks[0]
	assert.Equal(t, "github.com/gin-gonic/gin", task.PackageName)
	assert.Equal(t, 3, task.Priority)
	assert.Contains(t, task.Query, "breaking changes migration from 1.8.0 to 1.9.1")
}

func TestDocSearchCache_BuildDocumentationPack(t *testing.T) {
	cache := docdiscovery.NewDocSearchCache(1 * time.Hour)
	cache.PreloadStandardLibraryDocs()

	modified := []docdiscovery.PackageDependency{
		{
			Name:       "github.com/google/uuid",
			Version:    "v1.6.0",
			ChangeKind: docdiscovery.ChangeAdded,
			ManifestPath: "go.mod",
		},
	}

	pack := cache.BuildDocumentationPack(modified, 2000)
	require.NotNil(t, pack)

	assert.Len(t, pack.ModifiedPackages, 1)
	assert.Len(t, pack.Snippets, 1)
	assert.Equal(t, "github.com/google/uuid", pack.Snippets[0].PackageName)

	promptSlice := pack.FormatPromptSlice()
	assert.Contains(t, promptSlice, "### Package Dependencies & Official API Reference Context")
	assert.Contains(t, promptSlice, "github.com/google/uuid")
	assert.Contains(t, promptSlice, "uuid.NewV7")
}
