package deliberation

// PersonaProfile defines an expert agent's domain authority and review weighting.
type PersonaProfile struct {
	Persona       AgentPersona `json:"persona"`
	Name          string       `json:"name"`
	Weight        float64      `json:"weight"` // Voting multiplier
	CategoryFocus []string     `json:"category_focus"`
	SystemPrompt  string       `json:"system_prompt"`
}

// GetDefaultPersonas returns the enterprise 4-agent deliberation panel.
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
			CategoryFocus: []string{"PERFORMANCE", "CONCURRENCY", "DATABASE", "MEMORY"},
			SystemPrompt:  "You are a Principal Performance Architect. Scrutinize code diffs for N+1 queries, unbounded memory allocations, goroutine leaks, and locking contention.",
		},
		{
			Persona:       PersonaCleanCodeReviewer,
			Name:          "Clean Architecture & Maintainability Reviewer",
			Weight:        1.0,
			CategoryFocus: []string{"MAINTAINABILITY", "API_DESIGN", "ERROR_HANDLING"},
			SystemPrompt:  "You are a Principal Staff Engineer focused on clean architecture, API contracts, idiomatic patterns, and proper error propagation.",
		},
		{
			Persona:       PersonaDevilsAdvocate,
			Name:          "Devil's Advocate & False-Positive Filter",
			Weight:        1.30,
			CategoryFocus: []string{"VALIDATION", "CONTEXT_VERIFICATION"},
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
