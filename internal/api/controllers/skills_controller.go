// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Skills REST API Controller
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	agentSkills "github.com/scandrix/backend/internal/agents/skills"
	cliSkills "github.com/scandrix/backend/internal/cli/skills"
)

// SkillsController provides REST access to agent skill metadata and instructions.
type SkillsController struct {
	baseDir string
}

// NewSkillsController constructs a new skills controller.
func NewSkillsController(baseDir ...string) *SkillsController {
	dir := "skills"
	if len(baseDir) > 0 && strings.TrimSpace(baseDir[0]) != "" {
		dir = baseDir[0]
	} else if envDir := os.Getenv("SCANDRIX_SKILLS_DIR"); envDir != "" {
		dir = envDir
	}
	return &SkillsController{baseDir: dir}
}

// Routes mounts the /skills endpoints.
func (c *SkillsController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/{skillName}/meta", c.handleGetSkillMeta)
	r.Get("/{skillName}/instructions", c.handleGetSkillInstructions)
	r.Get("/", c.handleListSkills)

	return r
}

// SkillMetaResponse matches SkillMetaResponseDto in apps/api/src/dtos/skills-response.dto.ts
type SkillMetaResponse struct {
	Name         string                   `json:"name"`
	Description  string                   `json:"description"`
	Capabilities []string                 `json:"capabilities"`
	AllowedTools []string                 `json:"allowedTools"`
	RequiredMcps []agentSkills.RequiredMcp `json:"requiredMcps"`
}

// SkillInstructionsResponse matches SkillInstructionsResponseDto in apps/api/src/dtos/skills-response.dto.ts
type SkillInstructionsResponse struct {
	Instructions string `json:"instructions"`
}

func (c *SkillsController) resolveSkillManifest(skillName string) (*agentSkills.SkillManifest, error) {
	cleanName := filepath.Clean(skillName)
	if cleanName == "." || cleanName == "/" || strings.Contains(cleanName, "..") {
		return nil, fmt.Errorf("invalid skill name")
	}

	searchPaths := []string{
		filepath.Join(c.baseDir, cleanName),
		filepath.Join(".agents", "skills", cleanName),
		filepath.Join("internal", "agents", "skills", cleanName),
	}

	for _, p := range searchPaths {
		skillFile := filepath.Join(p, "SKILL.md")
		if _, err := os.Stat(skillFile); err == nil {
			return agentSkills.LoadSkillFromFile(skillFile)
		}
	}

	// Check bundled catalog
	for _, b := range cliSkills.BundledSkillsCatalog() {
		if strings.EqualFold(b.Name, cleanName) {
			return &agentSkills.SkillManifest{
				Name:         b.Name,
				Description:  b.Description,
				Instructions: b.Content,
				Capabilities: []string{"code-review", "business-rules"},
				AllowedTools: []string{"read_file", "git_diff", "ast_search"},
			}, nil
		}
	}

	return nil, fmt.Errorf("skill not found: %s", cleanName)
}

func (c *SkillsController) handleGetSkillMeta(w http.ResponseWriter, r *http.Request) {
	skillName := chi.URLParam(r, "skillName")
	if strings.TrimSpace(skillName) == "" {
		http.Error(w, `{"error":"skillName is required"}`, http.StatusBadRequest)
		return
	}

	manifest, err := c.resolveSkillManifest(skillName)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	tools := manifest.AllowedTools
	if tools == nil {
		tools = []string{}
	}
	caps := manifest.Capabilities
	if caps == nil {
		caps = []string{}
	}
	mcps := manifest.RequiredMcps
	if mcps == nil {
		mcps = []agentSkills.RequiredMcp{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(SkillMetaResponse{
		Name:         manifest.Name,
		Description:  manifest.Description,
		Capabilities: caps,
		AllowedTools: tools,
		RequiredMcps: mcps,
	})
}

func (c *SkillsController) handleGetSkillInstructions(w http.ResponseWriter, r *http.Request) {
	skillName := chi.URLParam(r, "skillName")
	if strings.TrimSpace(skillName) == "" {
		http.Error(w, `{"error":"skillName is required"}`, http.StatusBadRequest)
		return
	}

	manifest, err := c.resolveSkillManifest(skillName)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(SkillInstructionsResponse{
		Instructions: manifest.Instructions,
	})
}

func (c *SkillsController) handleListSkills(w http.ResponseWriter, r *http.Request) {
	type skillItem struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	var skillsList []skillItem
	seen := make(map[string]bool)

	// Scan disk
	if entries, err := os.ReadDir(c.baseDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				skillPath := filepath.Join(c.baseDir, e.Name(), "SKILL.md")
				if m, err := agentSkills.LoadSkillFromFile(skillPath); err == nil && m != nil {
					skillsList = append(skillsList, skillItem{
						Name:        m.Name,
						Description: m.Description,
					})
					seen[m.Name] = true
				}
			}
		}
	}

	// Add bundled
	for _, b := range cliSkills.BundledSkillsCatalog() {
		if !seen[b.Name] {
			skillsList = append(skillsList, skillItem{
				Name:        b.Name,
				Description: b.Description,
			})
			seen[b.Name] = true
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(skillsList)
}
