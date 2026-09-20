// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise MCP Rules & Memories Tools
// File: drixy_rules.go
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// DrixyRuleRecord represents a custom review rule in ScanDrix.
type DrixyRuleRecord struct {
	UUID           string         `json:"uuid"`
	ID             string         `json:"id"`
	OrganizationID string         `json:"organizationId"`
	TeamID         string         `json:"teamId,omitempty"`
	RepositoryID   string         `json:"repositoryId,omitempty"`
	Title          string         `json:"title"`
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	Prompt         string         `json:"prompt"`
	Severity       string         `json:"severity"` // "CRITICAL", "HIGH", "MEDIUM", "LOW"
	Category       string         `json:"category"`
	Scope          string         `json:"scope"`  // "ORGANIZATION", "REPOSITORY"
	Status         string         `json:"status"` // "ACTIVE", "INACTIVE"
	Active         bool           `json:"active"`
	Examples       []map[string]any `json:"examples,omitempty"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
}

// MemoryRecord represents an institutional memory item in ScanDrix.
type MemoryRecord struct {
	UUID           string    `json:"uuid"`
	OrganizationID string    `json:"organizationId"`
	RepositoryID   string    `json:"repositoryId,omitempty"`
	Content        string    `json:"content"`
	Category       string    `json:"category"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

var (
	rulesStoreMu  sync.RWMutex
	rulesStore    = make(map[string]*DrixyRuleRecord)
	memoryStoreMu sync.RWMutex
	memoryStore   = make(map[string]*MemoryRecord)
)

// GetDrixyRulesTools returns the MCP custom review rules and institutional memory tools.
func GetDrixyRulesTools() []MCPTool {
	var toolList []MCPTool

	// 1. DRIXY_GET_RULES / SCANDRIX_GET_RULES
	getRulesHandler := func(ctx context.Context, args map[string]any) (any, error) {
		orgID, _ := args["organizationId"].(string)
		if orgID == "" {
			return nil, fmt.Errorf("organizationId is required")
		}

		rulesStoreMu.RLock()
		defer rulesStoreMu.RUnlock()

		var matched []*DrixyRuleRecord
		for _, r := range rulesStore {
			if r.OrganizationID == orgID || orgID == "*" {
				matched = append(matched, r)
			}
		}

		if matched == nil {
			matched = []*DrixyRuleRecord{}
		}

		return map[string]any{
			"success": true,
			"count":   len(matched),
			"data":    matched,
		}, nil
	}

	toolList = append(toolList,
		MCPTool{
			Name:        "DRIXY_GET_RULES",
			Description: "Get all custom review rules configured for an organization.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string", "description": "Organization UUID"},
					"teamId":         map[string]string{"type": "string", "description": "Optional team UUID"},
				},
				"required": []string{"organizationId"},
			},
			Handler: getRulesHandler,
		},
		MCPTool{
			Name:        "SCANDRIX_GET_RULES",
			Description: "Get all custom review rules configured for an organization.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string", "description": "Organization UUID"},
					"teamId":         map[string]string{"type": "string", "description": "Optional team UUID"},
				},
				"required": []string{"organizationId"},
			},
			Handler: getRulesHandler,
		},
		MCPTool{
			Name:        "get_drixy_rules",
			Description: "Get all custom review rules configured for an organization.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
				},
				"required": []string{"organizationId"},
			},
			Handler: getRulesHandler,
		},
		MCPTool{
			Name:        "list_rules",
			Description: "Get all custom review rules configured for an organization.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
				},
				"required": []string{"organizationId"},
			},
			Handler: getRulesHandler,
		},
	)

	// 2. DRIXY_GET_RULES_REPOSITORY / SCANDRIX_GET_RULES_REPOSITORY
	getRepoRulesHandler := func(ctx context.Context, args map[string]any) (any, error) {
		orgID, _ := args["organizationId"].(string)
		repoID, _ := args["repositoryId"].(string)
		if orgID == "" {
			return nil, fmt.Errorf("organizationId is required")
		}

		rulesStoreMu.RLock()
		defer rulesStoreMu.RUnlock()

		var matched []*DrixyRuleRecord
		for _, r := range rulesStore {
			if (r.OrganizationID == orgID || orgID == "*") && (repoID == "" || r.RepositoryID == repoID || r.Scope == "ORGANIZATION") {
				matched = append(matched, r)
			}
		}

		if matched == nil {
			matched = []*DrixyRuleRecord{}
		}

		return map[string]any{
			"success": true,
			"count":   len(matched),
			"data":    matched,
		}, nil
	}

	toolList = append(toolList,
		MCPTool{
			Name:        "DRIXY_GET_RULES_REPOSITORY",
			Description: "Get custom review rules that apply to a specific repository.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"teamId":         map[string]string{"type": "string"},
					"repositoryId":   map[string]string{"type": "string"},
				},
				"required": []string{"organizationId"},
			},
			Handler: getRepoRulesHandler,
		},
		MCPTool{
			Name:        "SCANDRIX_GET_RULES_REPOSITORY",
			Description: "Get custom review rules that apply to a specific repository.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"teamId":         map[string]string{"type": "string"},
					"repositoryId":   map[string]string{"type": "string"},
				},
				"required": []string{"organizationId"},
			},
			Handler: getRepoRulesHandler,
		},
		MCPTool{
			Name:        "get_drixy_rules_repository",
			Description: "Get custom review rules that apply to a specific repository.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"repositoryId":   map[string]string{"type": "string"},
				},
				"required": []string{"organizationId"},
			},
			Handler: getRepoRulesHandler,
		},
		MCPTool{
			Name:        "get_rules_repository",
			Description: "Get custom review rules that apply to a specific repository.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"repositoryId":   map[string]string{"type": "string"},
				},
				"required": []string{"organizationId"},
			},
			Handler: getRepoRulesHandler,
		},
	)

	// 3. DRIXY_CREATE_RULE / SCANDRIX_CREATE_RULE
	createRuleHandler := func(ctx context.Context, args map[string]any) (any, error) {
		orgID, _ := args["organizationId"].(string)
		title, _ := args["title"].(string)
		if title == "" {
			title, _ = args["name"].(string)
		}
		prompt, _ := args["prompt"].(string)
		if prompt == "" {
			prompt, _ = args["description"].(string)
		}
		if orgID == "" || title == "" || prompt == "" {
			return nil, fmt.Errorf("organizationId, title, and prompt are required")
		}

		desc, _ := args["description"].(string)
		sev, _ := args["severity"].(string)
		if sev == "" {
			sev = "HIGH"
		}
		cat, _ := args["category"].(string)
		if cat == "" {
			cat = "SECURITY"
		}
		teamID, _ := args["teamId"].(string)
		repoID, _ := args["repositoryId"].(string)
		scope, _ := args["scope"].(string)
		if scope == "" {
			if repoID != "" {
				scope = "REPOSITORY"
			} else {
				scope = "ORGANIZATION"
			}
		}

		ruleUUID := uuid.New().String()
		now := time.Now().UTC()

		rule := &DrixyRuleRecord{
			UUID:           ruleUUID,
			ID:             ruleUUID,
			OrganizationID: orgID,
			TeamID:         teamID,
			RepositoryID:   repoID,
			Title:          title,
			Name:           title,
			Description:    desc,
			Prompt:         prompt,
			Severity:       sev,
			Category:       cat,
			Scope:          scope,
			Status:         "ACTIVE",
			Active:         true,
			CreatedAt:      now,
			UpdatedAt:      now,
		}

		rulesStoreMu.Lock()
		rulesStore[ruleUUID] = rule
		rulesStoreMu.Unlock()

		return map[string]any{
			"success": true,
			"data":    rule,
		}, nil
	}

	toolList = append(toolList,
		MCPTool{
			Name:        "DRIXY_CREATE_RULE",
			Description: "Create a new custom review rule for an organization or repository.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"title":          map[string]string{"type": "string"},
					"prompt":         map[string]string{"type": "string"},
					"severity":       map[string]string{"type": "string"},
					"category":       map[string]string{"type": "string"},
				},
				"required": []string{"organizationId", "title", "prompt"},
			},
			Handler: createRuleHandler,
		},
		MCPTool{
			Name:        "SCANDRIX_CREATE_RULE",
			Description: "Create a new custom review rule for an organization or repository.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"name":           map[string]string{"type": "string"},
					"prompt":         map[string]string{"type": "string"},
				},
				"required": []string{"organizationId", "name", "prompt"},
			},
			Handler: createRuleHandler,
		},
		MCPTool{
			Name:        "create_drixy_rule",
			Description: "Create a new custom review rule for an organization or repository.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"title":          map[string]string{"type": "string"},
					"prompt":         map[string]string{"type": "string"},
				},
				"required": []string{"organizationId", "title", "prompt"},
			},
			Handler: createRuleHandler,
		},
		MCPTool{
			Name:        "create_rule",
			Description: "Create a new custom review rule for an organization or repository.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"title":          map[string]string{"type": "string"},
					"prompt":         map[string]string{"type": "string"},
				},
				"required": []string{"organizationId", "title", "prompt"},
			},
			Handler: createRuleHandler,
		},
	)

	// 4. DRIXY_UPDATE_RULE / SCANDRIX_UPDATE_RULE
	updateRuleHandler := func(ctx context.Context, args map[string]any) (any, error) {
		ruleID, _ := args["ruleUuid"].(string)
		if ruleID == "" {
			ruleID, _ = args["ruleId"].(string)
		}
		if ruleID == "" {
			return nil, fmt.Errorf("ruleUuid or ruleId is required")
		}

		rulesStoreMu.Lock()
		defer rulesStoreMu.Unlock()

		rule, ok := rulesStore[ruleID]
		if !ok {
			return map[string]any{"success": false, "message": "Rule not found"}, nil
		}

		if title, ok := args["title"].(string); ok && title != "" {
			rule.Title = title
			rule.Name = title
		}
		if prompt, ok := args["prompt"].(string); ok && prompt != "" {
			rule.Prompt = prompt
		}
		if sev, ok := args["severity"].(string); ok && sev != "" {
			rule.Severity = sev
		}
		if active, ok := args["active"].(bool); ok {
			rule.Active = active
			if active {
				rule.Status = "ACTIVE"
			} else {
				rule.Status = "INACTIVE"
			}
		}
		rule.UpdatedAt = time.Now().UTC()

		return map[string]any{
			"success": true,
			"data":    rule,
		}, nil
	}

	toolList = append(toolList,
		MCPTool{
			Name:        "DRIXY_UPDATE_RULE",
			Description: "Update an existing custom review rule.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ruleUuid": map[string]string{"type": "string"},
					"title":    map[string]string{"type": "string"},
					"prompt":   map[string]string{"type": "string"},
					"severity": map[string]string{"type": "string"},
				},
				"required": []string{"ruleUuid"},
			},
			Handler: updateRuleHandler,
		},
		MCPTool{
			Name:        "SCANDRIX_UPDATE_RULE",
			Description: "Update an existing custom review rule.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ruleId": map[string]string{"type": "string"},
				},
				"required": []string{"ruleId"},
			},
			Handler: updateRuleHandler,
		},
		MCPTool{
			Name:        "update_drixy_rule",
			Description: "Update an existing custom review rule.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ruleUuid": map[string]string{"type": "string"},
				},
				"required": []string{"ruleUuid"},
			},
			Handler: updateRuleHandler,
		},
		MCPTool{
			Name:        "update_rule",
			Description: "Update an existing custom review rule.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ruleId": map[string]string{"type": "string"},
				},
				"required": []string{"ruleId"},
			},
			Handler: updateRuleHandler,
		},
	)

	// 5. DRIXY_DELETE_RULE / SCANDRIX_DELETE_RULE
	deleteRuleHandler := func(ctx context.Context, args map[string]any) (any, error) {
		ruleID, _ := args["ruleUuid"].(string)
		if ruleID == "" {
			ruleID, _ = args["ruleId"].(string)
		}
		if ruleID == "" {
			return nil, fmt.Errorf("ruleUuid or ruleId is required")
		}

		rulesStoreMu.Lock()
		defer rulesStoreMu.Unlock()

		if _, ok := rulesStore[ruleID]; !ok {
			return map[string]any{"success": false, "message": "Rule not found"}, nil
		}

		delete(rulesStore, ruleID)
		return map[string]any{
			"success": true,
			"deleted": ruleID,
		}, nil
	}

	toolList = append(toolList,
		MCPTool{
			Name:        "DRIXY_DELETE_RULE",
			Description: "Delete a custom review rule.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ruleUuid": map[string]string{"type": "string"},
				},
				"required": []string{"ruleUuid"},
			},
			Handler: deleteRuleHandler,
		},
		MCPTool{
			Name:        "SCANDRIX_DELETE_RULE",
			Description: "Delete a custom review rule.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ruleId": map[string]string{"type": "string"},
				},
				"required": []string{"ruleId"},
			},
			Handler: deleteRuleHandler,
		},
		MCPTool{
			Name:        "delete_drixy_rule",
			Description: "Delete a custom review rule.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ruleUuid": map[string]string{"type": "string"},
				},
				"required": []string{"ruleUuid"},
			},
			Handler: deleteRuleHandler,
		},
		MCPTool{
			Name:        "delete_rule",
			Description: "Delete a custom review rule.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ruleId": map[string]string{"type": "string"},
				},
				"required": []string{"ruleId"},
			},
			Handler: deleteRuleHandler,
		},
	)

	// 6. DRIXY_CREATE_MEMORY / SCANDRIX_CREATE_MEMORY
	createMemHandler := func(ctx context.Context, args map[string]any) (any, error) {
		orgID, _ := args["organizationId"].(string)
		content, _ := args["content"].(string)
		if orgID == "" || content == "" {
			return nil, fmt.Errorf("organizationId and content are required")
		}

		memUUID := uuid.New().String()
		cat, _ := args["category"].(string)
		if cat == "" {
			cat = "CONVENTION"
		}
		repoID, _ := args["repositoryId"].(string)
		now := time.Now().UTC()

		mem := &MemoryRecord{
			UUID:           memUUID,
			OrganizationID: orgID,
			RepositoryID:   repoID,
			Content:        content,
			Category:       cat,
			CreatedAt:      now,
			UpdatedAt:      now,
		}

		memoryStoreMu.Lock()
		memoryStore[memUUID] = mem
		memoryStoreMu.Unlock()

		return map[string]any{
			"success": true,
			"data":    mem,
		}, nil
	}

	toolList = append(toolList,
		MCPTool{
			Name:        "DRIXY_CREATE_MEMORY",
			Description: "Create a persistent institutional memory or architectural pattern for review agents.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"content":        map[string]string{"type": "string"},
					"category":       map[string]string{"type": "string"},
					"repositoryId":   map[string]string{"type": "string"},
				},
				"required": []string{"organizationId", "content"},
			},
			Handler: createMemHandler,
		},
		MCPTool{
			Name:        "SCANDRIX_CREATE_MEMORY",
			Description: "Create a persistent institutional memory or architectural pattern for review agents.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"content":        map[string]string{"type": "string"},
					"category":       map[string]string{"type": "string"},
					"repositoryId":   map[string]string{"type": "string"},
				},
				"required": []string{"organizationId", "content"},
			},
			Handler: createMemHandler,
		},
		MCPTool{
			Name:        "create_drixy_memory",
			Description: "Create a persistent institutional memory or architectural pattern for review agents.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"content":        map[string]string{"type": "string"},
				},
				"required": []string{"organizationId", "content"},
			},
			Handler: createMemHandler,
		},
		MCPTool{
			Name:        "create_memory",
			Description: "Create a persistent institutional memory or architectural pattern for review agents.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"content":        map[string]string{"type": "string"},
				},
				"required": []string{"organizationId", "content"},
			},
			Handler: createMemHandler,
		},
	)

	// 7. DRIXY_FIND_MEMORIES / SCANDRIX_FIND_MEMORIES
	findMemHandler := func(ctx context.Context, args map[string]any) (any, error) {
		orgID, _ := args["organizationId"].(string)
		query, _ := args["query"].(string)
		if orgID == "" {
			return nil, fmt.Errorf("organizationId is required")
		}

		memoryStoreMu.RLock()
		defer memoryStoreMu.RUnlock()

		var matched []*MemoryRecord
		lowerQ := strings.ToLower(query)
		for _, m := range memoryStore {
			if m.OrganizationID == orgID || orgID == "*" {
				if query == "" || strings.Contains(strings.ToLower(m.Content), lowerQ) {
					matched = append(matched, m)
				}
			}
		}

		if matched == nil {
			matched = []*MemoryRecord{}
		}

		return map[string]any{
			"success": true,
			"count":   len(matched),
			"data":    matched,
		}, nil
	}

	toolList = append(toolList,
		MCPTool{
			Name:        "DRIXY_FIND_MEMORIES",
			Description: "Search persistent institutional memories and code review findings.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"query":          map[string]string{"type": "string"},
				},
				"required": []string{"organizationId"},
			},
			Handler: findMemHandler,
		},
		MCPTool{
			Name:        "SCANDRIX_FIND_MEMORIES",
			Description: "Search persistent institutional memories and code review findings.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"query":          map[string]string{"type": "string"},
				},
				"required": []string{"organizationId"},
			},
			Handler: findMemHandler,
		},
		MCPTool{
			Name:        "find_drixy_memories",
			Description: "Search persistent institutional memories and code review findings.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"query":          map[string]string{"type": "string"},
				},
				"required": []string{"organizationId"},
			},
			Handler: findMemHandler,
		},
		MCPTool{
			Name:        "find_memories",
			Description: "Search persistent institutional memories and code review findings.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"query":          map[string]string{"type": "string"},
				},
				"required": []string{"organizationId"},
			},
			Handler: findMemHandler,
		},
	)

	return toolList
}
