// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: global_rules_source.go
// ═══════════════════════════════════════════════════════════════

package interfaces

// GlobalRulesSourceRepository defines a repository selected as a source of org-wide global rules.
type GlobalRulesSourceRepository struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"fullName,omitempty"`
}

// GlobalRulesSourceConfig captures source repository selections for global synchronization.
type GlobalRulesSourceConfig struct {
	Repositories []GlobalRulesSourceRepository `json:"repositories"`
}

// GlobalRulesImportTier defines access tiers for global rules importing.
type GlobalRulesImportTier string

const (
	GlobalRulesImportTierFree  GlobalRulesImportTier = "free"
	GlobalRulesImportTierTrial GlobalRulesImportTier = "trial"
	GlobalRulesImportTierPaid  GlobalRulesImportTier = "paid"
)

// GlobalRulesTrialImportLimit is the maximum global rules a trial tier organization can import.
const GlobalRulesTrialImportLimit = 5

// GlobalRulesImportStatus models import quotas and usage for UI rendering and server gates.
type GlobalRulesImportStatus struct {
	Tier      GlobalRulesImportTier `json:"tier"`
	Limit     *int                  `json:"limit"`     // nil = unlimited (paid), integer = cap, 0 = blocked (free)
	Used      int                   `json:"used"`      // Current active global-synced rules
	Remaining *int                  `json:"remaining"` // nil = unlimited, otherwise max(0, limit - used)
}
