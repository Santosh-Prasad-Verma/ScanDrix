package usecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
)

// ConfigTier represents the inheritance depth in ScanDrix's configuration hierarchy.
type ConfigTier string

const (
	TierSystem       ConfigTier = "SYSTEM"
	TierOrganization ConfigTier = "ORGANIZATION"
	TierTeam         ConfigTier = "TEAM"
	TierRepository   ConfigTier = "REPOSITORY"
	TierDirectory    ConfigTier = "DIRECTORY"
)

// ModelSlotKind identifies specialized LLM slots for targeted review tasks.
type ModelSlotKind string

const (
	SlotPrimaryTriage   ModelSlotKind = "PRIMARY_TRIAGE"
	SlotFastAnalysis    ModelSlotKind = "FAST_ANALYSIS"
	SlotDeepReasoning   ModelSlotKind = "DEEP_REASONING"
	SlotSyntaxValidator ModelSlotKind = "SYNTAX_VALIDATOR"
	SlotSummaryDigest   ModelSlotKind = "SUMMARY_DIGEST"
	SlotEmergencyRescue ModelSlotKind = "EMERGENCY_RESCUE"
)

// ModelSlotSpec defines parameters for a dedicated model slot in the review pipeline.
type ModelSlotSpec struct {
	Kind           ModelSlotKind `json:"kind"`
	Provider       string        `json:"provider"` // anthropic, openai, gemini, novita, byok
	ModelID        string        `json:"model_id"`
	Temperature    float64       `json:"temperature"`
	MaxInputTokens int           `json:"max_input_tokens"`
	MaxOutputTokens int          `json:"max_output_tokens"`
	SupportsTools  bool          `json:"supports_tools"`
	ReasoningEffort string       `json:"reasoning_effort,omitempty"`
	TimeoutSeconds int           `json:"timeout_seconds"`
}

// EnterprisePolicyGuardrail prevents repository or team configs from bypassing org security standards.
type EnterprisePolicyGuardrail struct {
	DisallowDisablingCriticalRules bool     `json:"disallow_disabling_critical_rules"`
	MinStrictness                  string   `json:"min_strictness"` // standard, strict
	MandatoryRules                 []string `json:"mandatory_rules,omitempty"`
	ForbiddenModels                []string `json:"forbidden_models,omitempty"`
	RequireSLAAuditLogs            bool     `json:"require_sla_audit_logs"`
}

// EnterpriseCodeReviewConfig extends domain.CodeReviewConfig with multi-tier inheritance and slot mapping.
type EnterpriseCodeReviewConfig struct {
	ID                     uuid.UUID                  `json:"id"`
	Tier                   ConfigTier                 `json:"tier"`
	OrganizationID         string                     `json:"organization_id"`
	TeamID                 string                     `json:"team_id,omitempty"`
	RepositoryID           string                     `json:"repository_id,omitempty"`
	PathPattern            string                     `json:"path_pattern,omitempty"` // Glob pattern for directory-tier rules
	BranchPattern          string                     `json:"branch_pattern,omitempty"`
	BaselineConfig         domain.CodeReviewConfig    `json:"baseline_config"`
	ModelSlots             map[ModelSlotKind]ModelSlotSpec `json:"model_slots"`
	PolicyGuardrail        EnterprisePolicyGuardrail  `json:"policy_guardrail"`
	AllowedFileExtensions  []string                   `json:"allowed_file_extensions,omitempty"`
	ExcludedFileGlobs      []string                   `json:"excluded_file_globs,omitempty"`
	Cadence                string                     `json:"cadence"` // continuous, nightly, manual
	MaxTokensPerReview     int                        `json:"max_tokens_per_review"`
	ConcurrencyLimit       int                        `json:"concurrency_limit"`
	AutoApproveCleanPRs    bool                       `json:"auto_approve_clean_prs"`
	RequireCommittableOnly bool                       `json:"require_committable_only"`
	Version                int                        `json:"version"`
	UpdatedAt              time.Time                  `json:"updated_at"`
}

// DeepConfigHierarchyService resolves multi-tier enterprise configurations with strict policy inheritance.
type DeepConfigHierarchyService struct {
	mu         sync.RWMutex
	configs    map[string]EnterpriseCodeReviewConfig // key: tier:org:team:repo:path
	ruleRepo   IConfigRepository
	cache      map[string]*EnterpriseCodeReviewConfig
	cacheTTL   time.Duration
}

// NewDeepConfigHierarchyService creates the enterprise configuration hierarchy coordinator.
func NewDeepConfigHierarchyService(repo IConfigRepository) *DeepConfigHierarchyService {
	return &DeepConfigHierarchyService{
		configs:  make(map[string]EnterpriseCodeReviewConfig),
		ruleRepo: repo,
		cache:    make(map[string]*EnterpriseCodeReviewConfig),
		cacheTTL: 5 * time.Minute,
	}
}

// BuildScopeKey generates deterministic hierarchy storage keys.
func BuildScopeKey(tier ConfigTier, orgID, teamID, repoID, path string) string {
	return fmt.Sprintf("%s::%s::%s::%s::%s", tier, orgID, teamID, repoID, path)
}

// SaveConfig registers or updates a tiered configuration record.
func (s *DeepConfigHierarchyService) SaveConfig(ctx context.Context, cfg EnterpriseCodeReviewConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cfg.OrganizationID == "" {
		return fmt.Errorf("organization_id is mandatory for enterprise configuration")
	}
	if cfg.ID == uuid.Nil {
		cfg.ID = uuid.New()
	}
	cfg.UpdatedAt = time.Now().UTC()
	cfg.Version++

	key := BuildScopeKey(cfg.Tier, cfg.OrganizationID, cfg.TeamID, cfg.RepositoryID, cfg.PathPattern)
	s.configs[key] = cfg

	// Invalidate cache
	s.cache = make(map[string]*EnterpriseCodeReviewConfig)
	return nil
}

// ResolveEffectiveConfig computes the finalized configuration for a review execution context.
func (s *DeepConfigHierarchyService) ResolveEffectiveConfig(
	ctx context.Context,
	orgID, teamID, repoID, targetFile, targetBranch string,
) (*EnterpriseCodeReviewConfig, error) {
	s.mu.RLock()
	cacheKey := fmt.Sprintf("%s::%s::%s::%s::%s", orgID, teamID, repoID, targetFile, targetBranch)
	if cached, ok := s.cache[cacheKey]; ok {
		s.mu.RUnlock()
		return cached, nil
	}
	s.mu.RUnlock()

	// 1. Initialize with System Default baseline
	effective := s.getSystemDefaultConfig()

	// 2. Apply Organization level override
	s.mu.RLock()
	orgKey := BuildScopeKey(TierOrganization, orgID, "", "", "")
	if orgCfg, ok := s.configs[orgKey]; ok {
		s.mergeConfig(&effective, orgCfg)
	}

	// 3. Apply Team level override
	if teamID != "" {
		teamKey := BuildScopeKey(TierTeam, orgID, teamID, "", "")
		if teamCfg, ok := s.configs[teamKey]; ok {
			s.mergeConfig(&effective, teamCfg)
		}
	}

	// 4. Apply Repository level override
	if repoID != "" {
		repoKey := BuildScopeKey(TierRepository, orgID, teamID, repoID, "")
		if repoCfg, ok := s.configs[repoKey]; ok {
			s.mergeConfig(&effective, repoCfg)
		}
	}

	// 5. Apply Directory / File path pattern override
	if targetFile != "" {
		for key, dirCfg := range s.configs {
			if dirCfg.Tier == TierDirectory && dirCfg.OrganizationID == orgID {
				if dirCfg.PathPattern != "" {
					matched, err := filepath.Match(dirCfg.PathPattern, targetFile)
					if err == nil && matched {
						s.mergeConfig(&effective, dirCfg)
					}
				}
			}
			_ = key
		}
	}
	s.mu.RUnlock()

	// 6. Enforce Enterprise Policy Guardrails (fail-closed security)
	s.enforceGuardrails(&effective)

	s.mu.Lock()
	s.cache[cacheKey] = &effective
	s.mu.Unlock()

	return &effective, nil
}

func (s *DeepConfigHierarchyService) getSystemDefaultConfig() EnterpriseCodeReviewConfig {
	slots := make(map[ModelSlotKind]ModelSlotSpec)
	slots[SlotPrimaryTriage] = ModelSlotSpec{
		Kind:           SlotPrimaryTriage,
		Provider:       "anthropic",
		ModelID:        "claude-3-7-sonnet",
		Temperature:    0.2,
		MaxInputTokens: 120_000,
		MaxOutputTokens: 4096,
		SupportsTools:  true,
		TimeoutSeconds: 60,
	}
	slots[SlotFastAnalysis] = ModelSlotSpec{
		Kind:           SlotFastAnalysis,
		Provider:       "google",
		ModelID:        "gemini-2.5-flash",
		Temperature:    0.1,
		MaxInputTokens: 64_000,
		MaxOutputTokens: 2048,
		SupportsTools:  true,
		TimeoutSeconds: 30,
	}
	slots[SlotDeepReasoning] = ModelSlotSpec{
		Kind:           SlotDeepReasoning,
		Provider:       "openai",
		ModelID:        "o3-mini",
		Temperature:    0.0,
		MaxInputTokens: 100_000,
		MaxOutputTokens: 8192,
		SupportsTools:  false,
		ReasoningEffort: "high",
		TimeoutSeconds: 90,
	}

	return EnterpriseCodeReviewConfig{
		ID:             uuid.New(),
		Tier:           TierSystem,
		OrganizationID: "system",
		BaselineConfig: domain.DefaultCodeReviewConfig(),
		ModelSlots:     slots,
		PolicyGuardrail: EnterprisePolicyGuardrail{
			DisallowDisablingCriticalRules: true,
			MinStrictness:                  "standard",
		},
		Cadence:            "continuous",
		MaxTokensPerReview: 250_000,
		ConcurrencyLimit:   5,
		Version:            1,
		UpdatedAt:          time.Now().UTC(),
	}
}

func (s *DeepConfigHierarchyService) mergeConfig(target *EnterpriseCodeReviewConfig, src EnterpriseCodeReviewConfig) {
	// Merge baseline flags
	target.BaselineConfig.Enabled = src.BaselineConfig.Enabled
	if src.BaselineConfig.Strictness != "" {
		target.BaselineConfig.Strictness = src.BaselineConfig.Strictness
	}
	if src.BaselineConfig.MaxSuggestions > 0 {
		target.BaselineConfig.MaxSuggestions = src.BaselineConfig.MaxSuggestions
	}
	if src.BaselineConfig.ByokModelID != "" {
		target.BaselineConfig.ByokModelID = src.BaselineConfig.ByokModelID
	}

	// Merge Policy Guardrails
	if src.PolicyGuardrail.DisallowDisablingCriticalRules {
		target.PolicyGuardrail.DisallowDisablingCriticalRules = true
	}
	if src.PolicyGuardrail.MinStrictness != "" {
		target.PolicyGuardrail.MinStrictness = src.PolicyGuardrail.MinStrictness
	}
	if len(src.PolicyGuardrail.ForbiddenModels) > 0 {
		target.PolicyGuardrail.ForbiddenModels = append(target.PolicyGuardrail.ForbiddenModels, src.PolicyGuardrail.ForbiddenModels...)
	}
	if src.PolicyGuardrail.RequireSLAAuditLogs {
		target.PolicyGuardrail.RequireSLAAuditLogs = true
	}

	// Merge Model Slots
	if target.ModelSlots == nil {
		target.ModelSlots = make(map[ModelSlotKind]ModelSlotSpec)
	}
	for k, v := range src.ModelSlots {
		target.ModelSlots[k] = v
	}

	// Merge Globs & Extensions
	if len(src.AllowedFileExtensions) > 0 {
		target.AllowedFileExtensions = src.AllowedFileExtensions
	}
	if len(src.ExcludedFileGlobs) > 0 {
		target.ExcludedFileGlobs = append(target.ExcludedFileGlobs, src.ExcludedFileGlobs...)
	}

	if src.MaxTokensPerReview > 0 {
		target.MaxTokensPerReview = src.MaxTokensPerReview
	}
	if src.ConcurrencyLimit > 0 {
		target.ConcurrencyLimit = src.ConcurrencyLimit
	}
	if src.Cadence != "" {
		target.Cadence = src.Cadence
	}
	if src.AutoApproveCleanPRs {
		target.AutoApproveCleanPRs = true
	}
	if src.RequireCommittableOnly {
		target.RequireCommittableOnly = true
	}
}

func (s *DeepConfigHierarchyService) enforceGuardrails(cfg *EnterpriseCodeReviewConfig) {
	// Guardrail 1: Organization requires critical rule protection
	if cfg.PolicyGuardrail.DisallowDisablingCriticalRules {
		cfg.BaselineConfig.Enabled = true // Cannot turn off platform entirely if guardrail active
	}

	// Guardrail 2: Enforce minimum strictness
	if strings.EqualFold(cfg.PolicyGuardrail.MinStrictness, "strict") {
		cfg.BaselineConfig.Strictness = domain.StrictnessStrict
	}

	// Guardrail 3: Forbidden models check
	if len(cfg.PolicyGuardrail.ForbiddenModels) > 0 && cfg.BaselineConfig.ByokModelID != "" {
		for _, forbidden := range cfg.PolicyGuardrail.ForbiddenModels {
			if strings.EqualFold(cfg.BaselineConfig.ByokModelID, forbidden) {
				// Reset to safe system default slot
				cfg.BaselineConfig.ByokModelID = cfg.ModelSlots[SlotPrimaryTriage].ModelID
				break
			}
		}
	}
}

// ReconcileInRepoConfig checks proposed in-repo .scandrix.yml changes against organization policy.
func (s *DeepConfigHierarchyService) ReconcileInRepoConfig(
	ctx context.Context,
	orgID, repoID string,
	inRepoYAML string,
) (bool, string, error) {
	orgCfg, err := s.ResolveEffectiveConfig(ctx, orgID, "", repoID, "", "")
	if err != nil {
		return false, "", err
	}

	// Simple YAML scan for prohibited bypasses
	lower := strings.ToLower(inRepoYAML)
	if orgCfg.PolicyGuardrail.DisallowDisablingCriticalRules {
		if strings.Contains(lower, "enabled: false") || strings.Contains(lower, "enabled: 0") {
			return false, "organization policy prohibits disabling ScanDrix review via in-repo configuration", nil
		}
	}

	if orgCfg.PolicyGuardrail.MinStrictness == "strict" {
		if strings.Contains(lower, "strictness: low") || strings.Contains(lower, "strictness: lax") {
			return false, "organization policy requires minimum strictness of 'strict'", nil
		}
	}

	hash := sha256.Sum256([]byte(inRepoYAML))
	return true, hex.EncodeToString(hash[:8]), nil
}
