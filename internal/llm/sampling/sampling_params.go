// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package sampling

import (
	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

// ResolveByokTemperature resolves the temperature value to actually send to the model provider.
// Queries the provider module's intrinsic TemperaturePolicy:
//   - unsupported -> returns nil (omits field so provider doesn't fail with 400)
//   - fixed       -> returns pinned policy value over stored configuration
//   - adjustable  -> returns the configured slot temperature
func ResolveByokTemperature(slot *byok.NormalizedModel) *float64 {
	if slot == nil {
		return nil
	}

	provider := string(slot.Provider)
	if provider == "" || !kernel.DefaultRegistry.Has(provider) {
		return slot.Temperature
	}

	module, ok := kernel.DefaultRegistry.Get(provider)
	if !ok || module == nil {
		return slot.Temperature
	}

	policy := module.TemperaturePolicy(*slot)
	if policy == nil {
		return slot.Temperature
	}

	pKind := policy.Kind
	if pKind == "" {
		pKind = policy.Mode
	}
	pValue := policy.Value
	if pValue == nil {
		pValue = policy.FixedValue
	}

	switch pKind {
	case kernel.TemperatureUnsupported:
		return nil
	case kernel.TemperatureFixed:
		return pValue
	default: // adjustable
		return slot.Temperature
	}
}
