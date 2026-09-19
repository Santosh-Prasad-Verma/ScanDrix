// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Dynamic Skills Subsystem
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package skills

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/scandrix/backend/internal/agents/businessrules"
)

// SkillCapability represents an abstract capability provided by the host environment or integrations.
type SkillCapability interface {
	Name() string
	Execute(ctx context.Context, input map[string]any) (map[string]any, error)
}

// CapabilityRegistry provides thread-safe registration and resolution of capabilities.
type CapabilityRegistry struct {
	mu           sync.RWMutex
	capabilities map[string]SkillCapability
}

// NewCapabilityRegistry constructs a registry with default built-in capabilities.
func NewCapabilityRegistry() *CapabilityRegistry {
	reg := &CapabilityRegistry{
		capabilities: make(map[string]SkillCapability),
	}

	reg.Register(&PRDiffReadCapability{})
	reg.Register(&PRMetadataReadCapability{})
	reg.Register(&TaskContextReadCapability{})

	return reg
}

// Register adds or replaces a capability.
func (r *CapabilityRegistry) Register(cap SkillCapability) {
	if cap == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.capabilities[cap.Name()] = cap
}

// Get retrieves a capability by name.
func (r *CapabilityRegistry) Get(name string) (SkillCapability, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.capabilities[name]
	return c, ok
}

// List returns the names of all registered capabilities.
func (r *CapabilityRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.capabilities))
	for k := range r.capabilities {
		names = append(names, k)
	}
	return names
}

// ═══════════════════════════════════════════════════════════════
// Standard Built-In Capabilities
// ═══════════════════════════════════════════════════════════════

// PRDiffReadCapability reads pull request diff patches from the execution context.
type PRDiffReadCapability struct{}

func (c *PRDiffReadCapability) Name() string {
	return "pr.diff.read"
}

func (c *PRDiffReadCapability) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	if input == nil {
		return nil, fmt.Errorf("missing execution input for %s", c.Name())
	}

	diff, ok := input["pr_diff"].(string)
	if !ok || diff == "" {
		// Check nested prepareContext
		if prep, ok := input["prepare_context"].(map[string]any); ok {
			if d, ok := prep["pr_diff"].(string); ok {
				diff = d
			}
		}
	}

	return map[string]any{
		"pr_diff":  diff,
		"has_diff": diff != "",
	}, nil
}

// PRMetadataReadCapability reads metadata like PR title, body, author, and branch.
type PRMetadataReadCapability struct{}

func (c *PRMetadataReadCapability) Name() string {
	return "pr.metadata.read"
}

func (c *PRMetadataReadCapability) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	meta := make(map[string]any)

	if prep, ok := input["prepare_context"].(map[string]any); ok {
		if pr, ok := prep["pull_request"].(map[string]any); ok {
			for k, v := range pr {
				meta[k] = v
			}
		}
	}

	for _, k := range []string{"title", "pr_title", "pr_body", "author", "head_sha", "base_sha"} {
		if v, ok := input[k]; ok {
			meta[k] = v
		}
	}

	return meta, nil
}

// TaskContextReadCapability reads external task requirements (Jira, Linear, GitHub issues).
type TaskContextReadCapability struct{}

func (c *TaskContextReadCapability) Name() string {
	return "task.context.read"
}

func (c *TaskContextReadCapability) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	taskCtx := ""
	if tc, ok := input["task_context"].(string); ok {
		taskCtx = tc
	} else if prep, ok := input["prepare_context"].(map[string]any); ok {
		if tc, ok := prep["task_context"].(string); ok {
			taskCtx = tc
		}
	}

	diff := ""
	if d, ok := input["pr_diff"].(string); ok {
		diff = d
	} else if prep, ok := input["prepare_context"].(map[string]any); ok {
		if d, ok := prep["pr_diff"].(string); ok {
			diff = d
		}
	}

	body := ""
	if b, ok := input["pr_body"].(string); ok {
		body = b
	} else if prep, ok := input["prepare_context"].(map[string]any); ok {
		if b, ok := prep["pr_body"].(string); ok {
			body = b
		}
	}

	taskKeys := businessrules.ExtractTaskIdentifiers(diff, body)

	// If task context is empty but taskKeys were identified, attempt dynamic resolution
	if strings.TrimSpace(taskCtx) == "" && len(taskKeys) > 0 {
		if fetcher, ok := input["task_fetcher"].(businessrules.TaskFetcher); ok && fetcher != nil {
			if fetched, err := fetcher.FetchTaskContext(ctx, taskKeys); err == nil && strings.TrimSpace(fetched) != "" {
				taskCtx = fetched
			}
		} else if fetcherFn, ok := input["task_fetcher"].(func(context.Context, []string) (string, error)); ok && fetcherFn != nil {
			if fetched, err := fetcherFn(ctx, taskKeys); err == nil && strings.TrimSpace(fetched) != "" {
				taskCtx = fetched
			}
		}
	}

	return map[string]any{
		"task_context": strings.TrimSpace(taskCtx),
		"has_task":     strings.TrimSpace(taskCtx) != "",
		"task_keys":    taskKeys,
	}, nil
}
