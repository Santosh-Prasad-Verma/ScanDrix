package sync

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/pkg/models"
)

// RuleSyncer validates, reconciles, and compiles declarative rule repositories into the active engine.
type RuleSyncer struct {
	mu           sync.RWMutex
	rulesByWs    map[uuid.UUID][]rules.RuleSpec
	ruleHashes   map[uuid.UUID]map[string]string // wsID -> ruleID/name -> hash
}

// NewRuleSyncer initializes the rule synchronization service.
func NewRuleSyncer() *RuleSyncer {
	return &RuleSyncer{
		rulesByWs:  make(map[uuid.UUID][]rules.RuleSpec),
		ruleHashes: make(map[uuid.UUID]map[string]string),
	}
}

// SyncFromJSON decodes, compiles, and registers custom rules from a raw JSON payload.
func (s *RuleSyncer) SyncFromJSON(ctx context.Context, wsID uuid.UUID, rawJSON []byte) (*SyncResult, error) {
	var configFile RuleConfigFile
	if err := json.Unmarshal(rawJSON, &configFile); err != nil {
		return nil, fmt.Errorf("failed to parse rule JSON configuration: %w", err)
	}
	return s.syncDefinitions(ctx, wsID, configFile.Rules)
}

// GetWorkspaceRules returns compiled rules for a specific workspace.
func (s *RuleSyncer) GetWorkspaceRules(wsID uuid.UUID) []rules.RuleSpec {
	s.mu.RLock()
	defer s.mu.RUnlock()

	active, ok := s.rulesByWs[wsID]
	if !ok {
		return rules.DefaultCatalog()
	}

	// Merge with DefaultCatalog
	combined := make([]rules.RuleSpec, 0, len(rules.DefaultCatalog())+len(active))
	combined = append(combined, rules.DefaultCatalog()...)
	combined = append(combined, active...)
	return combined
}

func (s *RuleSyncer) syncDefinitions(ctx context.Context, wsID uuid.UUID, defs []RuleDefinition) (*SyncResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := &SyncResult{
		WorkspaceID: wsID,
		TotalParsed: len(defs),
		SyncedAt:    time.Now().UTC(),
	}

	if s.ruleHashes[wsID] == nil {
		s.ruleHashes[wsID] = make(map[string]string)
	}

	validRules := make([]rules.RuleSpec, 0, len(defs))

	for _, d := range defs {
		// 1. Validate regex compilability
		if _, err := regexp.Compile(d.RegexRule); err != nil {
			result.InvalidCount++
			result.Errors = append(result.Errors, fmt.Sprintf("invalid regex in rule '%s': %s", d.Name, err.Error()))
			continue
		}

		// 2. Validate severity
		severity := d.Severity
		if severity == "" {
			severity = models.SeverityMedium
		}

		// 3. Compute deterministic hash to detect updates
		ruleKey := d.Name
		if d.ID != "" {
			ruleKey = d.ID
		}
		hash := computeRuleHash(d)

		existingHash, exists := s.ruleHashes[wsID][ruleKey]
		if !exists {
			result.AddedCount++
		} else if existingHash != hash {
			result.UpdatedCount++
		}
		s.ruleHashes[wsID][ruleKey] = hash

		parsedID := uuid.New()
		if d.ID != "" {
			if u, err := uuid.Parse(d.ID); err == nil {
				parsedID = u
			}
		}

		pathPattern := d.PathPattern
		if pathPattern == "" {
			pathPattern = "*"
		}

		validRules = append(validRules, rules.RuleSpec{
			ID:          parsedID,
			Name:        d.Name,
			PathPattern: pathPattern,
			RegexRule:   d.RegexRule,
			Severity:    severity,
			Category:    d.Category,
			Description: d.Description,
			Remediation: d.Remediation,
		})
	}

	s.rulesByWs[wsID] = validRules
	return result, nil
}

func computeRuleHash(d RuleDefinition) string {
	h := sha256.New()
	h.Write([]byte(d.Name))
	h.Write([]byte(d.PathPattern))
	h.Write([]byte(d.RegexRule))
	h.Write([]byte(d.Severity))
	h.Write([]byte(d.Description))
	h.Write([]byte(d.Remediation))
	return fmt.Sprintf("%x", h.Sum(nil))
}
