package deliberation

// PersonaProfile defines an expert agent's domain authority and review weighting.
type PersonaProfile struct {
	Persona       AgentPersona `json:"persona"`
	Name          string       `json:"name"`
	Weight        float64      `json:"weight"` // Voting multiplier
	CategoryFocus []string     `json:"category_focus"`
	SystemPrompt  string       `json:"system_prompt"`
}

// GetDefaultPersonas returns the enterprise 7-agent specialized deliberation panel.
func GetDefaultPersonas() []PersonaProfile {
	return []PersonaProfile{
		{
			Persona:       PersonaSecurityAuditor,
			Name:          "Senior Application Security Auditor",
			Weight:        1.25,
			CategoryFocus: []string{"SECURITY", "OWASP", "CRYPTO", "AUTH", "SECRETS"},
			SystemPrompt:  "You are a rigorous Application Security Engineer. Scrutinize code diffs for CWEs, SQL/command injection, hardcoded credentials, and authorization bypasses.",
		},
		{
			Persona:       PersonaPerformanceArchitect,
			Name:          "Distributed Systems & Performance Architect",
			Weight:        1.15,
			CategoryFocus: []string{"PERFORMANCE", "SYSTEMS", "THROUGHPUT", "LATENCY"},
			SystemPrompt:  "You are a Principal Performance Architect. Scrutinize code diffs for inefficient algorithmic complexity, hot loops, serialization overhead, and latency bottlenecks.",
		},
		{
			Persona:       PersonaConcurrencyAuditor,
			Name:          "Concurrency & Race Condition Auditor",
			Weight:        1.25,
			CategoryFocus: []string{"CONCURRENCY", "RACE_CONDITION", "DEADLOCK", "THREAD_SAFETY", "MUTEX", "CHANNELS"},
			SystemPrompt:  "You are an elite Concurrency & Multithreading Specialist. Scrutinize code diffs for race conditions, unsynchronized map/slice access, lock ordering deadlocks, unbuffered channel blocking, and missing mutex/atomic synchronizations.",
		},
		{
			Persona:       PersonaMemoryLeakSpecialist,
			Name:          "Memory Management & Resource Leak Specialist",
			Weight:        1.20,
			CategoryFocus: []string{"MEMORY", "RESOURCE_LEAK", "GOROUTINE_LEAK", "GC_PRESSURE", "FILE_DESCRIPTORS"},
			SystemPrompt:  "You are a Memory Management & Reliability Engineer. Scrutinize code diffs for unclosed response bodies/database rows, orphan goroutines/threads, unbounded memory allocations, circular references, and memory leaks.",
		},
		{
			Persona:       PersonaSQLOptimizer,
			Name:          "Database & SQL Optimization Engineer",
			Weight:        1.25,
			CategoryFocus: []string{"SQL", "DATABASE", "N_PLUS_ONE", "INDEXING", "TRANSACTIONS", "ORM"},
			SystemPrompt:  "You are a Principal Database Administrator & SQL Performance Engineer. Scrutinize code diffs for N+1 queries, unindexed table scans, missing foreign keys, transaction lock contention, ORM Cartesian explosions, and inefficient JOINs.",
		},
		{
			Persona:       PersonaCleanCodeReviewer,
			Name:          "Clean Architecture & Maintainability Reviewer",
			Weight:        1.0,
			CategoryFocus: []string{"MAINTAINABILITY", "API_DESIGN", "ERROR_HANDLING", "CODE_SMELLS"},
			SystemPrompt:  "You are a Principal Staff Engineer focused on clean architecture, API contracts, idiomatic patterns, and proper error propagation.",
		},
		{
			Persona:       PersonaDevilsAdvocate,
			Name:          "Devil's Advocate & False-Positive Filter",
			Weight:        1.30,
			CategoryFocus: []string{"VALIDATION", "CONTEXT_VERIFICATION", "MITIGATION_CHECK"},
			SystemPrompt:  "You are a skeptical peer reviewer. Challenge proposed findings aggressively. Check if the issue is already mitigated upstream, handled by framework defaults, or represents an acceptable low-risk pattern.",
		},
	}
}

// PersonaWeight returns the voting weight for a given persona.
func PersonaWeight(persona AgentPersona) float64 {
	for _, p := range GetDefaultPersonas() {
		if p.Persona == persona {
			return p.Weight
		}
	}
	return 1.0
}
