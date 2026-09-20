package spendlimit

import (
	"sort"
	"strings"
)

// CollectByokModels collects distinct non-blank model identifiers from configured BYOK models and overrides.
func CollectByokModels(byokConfig *BYOKConfigRef, extraModels []string) []string {
	var candidates []string
	if byokConfig != nil {
		for _, m := range byokConfig.Models {
			candidates = append(candidates, m.Model)
		}
	}
	candidates = append(candidates, extraModels...)

	seen := make(map[string]bool)
	var unique []string
	for _, c := range candidates {
		clean := strings.TrimSpace(c)
		if clean != "" && !seen[clean] {
			seen[clean] = true
			unique = append(unique, clean)
		}
	}
	return unique
}

// ExtractByokModelsFromConfig recursively walks untyped configuration structures to locate all "byokModel" overrides.
func ExtractByokModelsFromConfig(configValue any) []string {
	var found []string

	var walk func(node any)
	walk = func(node any) {
		if node == nil {
			return
		}

		switch v := node.(type) {
		case []any:
			for _, item := range v {
				walk(item)
			}
		case map[string]any:
			if val, ok := v["byokModel"]; ok {
				if str, ok := val.(string); ok && str != "" {
					found = append(found, str)
				}
			}
			var keys []string
			for k := range v {
				if k != "byokModel" {
					keys = append(keys, k)
				}
			}
			sort.Strings(keys)
			for _, k := range keys {
				walk(v[k])
			}
		}
	}

	walk(configValue)
	return found
}
