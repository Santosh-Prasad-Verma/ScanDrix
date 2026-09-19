// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package tools

import (
	"sync"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

// InMemoryToolRegistry provides a thread-safe registry over a slice of AgentTools.
type InMemoryToolRegistry struct {
	mu     sync.RWMutex
	byName map[string]contracts.AgentTool
	order  []string
}

// NewInMemoryToolRegistry creates a new registry initialized with the provided tools.
func NewInMemoryToolRegistry(tools ...contracts.AgentTool) *InMemoryToolRegistry {
	reg := &InMemoryToolRegistry{
		byName: make(map[string]contracts.AgentTool, len(tools)),
		order:  make([]string, 0, len(tools)),
	}
	for _, t := range tools {
		if t != nil {
			reg.Register(t)
		}
	}
	return reg
}

// Register registers or updates a tool in the registry.
func (r *InMemoryToolRegistry) Register(tool contracts.AgentTool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := tool.Name()
	if _, exists := r.byName[name]; !exists {
		r.order = append(r.order, name)
	}
	r.byName[name] = tool
}

// Get retrieves a tool by name.
func (r *InMemoryToolRegistry) Get(name string) (contracts.AgentTool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.byName[name]
	return t, ok
}

// List returns all registered tools in registration order.
func (r *InMemoryToolRegistry) List() []contracts.AgentTool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]contracts.AgentTool, 0, len(r.order))
	for _, name := range r.order {
		if t, ok := r.byName[name]; ok {
			out = append(out, t)
		}
	}
	return out
}
