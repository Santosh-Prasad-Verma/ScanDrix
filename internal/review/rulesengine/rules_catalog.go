// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package rulesengine

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// RulesCatalog maintains an in-memory thread-safe catalog of system, organization, team, and repo rules.
type RulesCatalog struct {
	libraryRules []*DrixyRule
	orgRules     map[uuid.UUID][]*DrixyRule
	teamRules    map[string][]*DrixyRule // key: orgID:teamID
	repoRules    map[string][]*DrixyRule // key: orgID:repoID
	rulesByID    map[uuid.UUID]*DrixyRule
	mu           sync.RWMutex
}

// NewRulesCatalog constructs a catalog pre-populated with standard enterprise library rules.
func NewRulesCatalog() *RulesCatalog {
	catalog := &RulesCatalog{
		orgRules:  make(map[uuid.UUID][]*DrixyRule),
		teamRules: make(map[string][]*DrixyRule),
		repoRules: make(map[string][]*DrixyRule),
		rulesByID: make(map[uuid.UUID]*DrixyRule),
	}

	catalog.initBuiltinLibraryRules()
	return catalog
}

func (c *RulesCatalog) initBuiltinLibraryRules() {
	builtinRules := []*DrixyRule{
		{
			ID:          uuid.MustParse("00000000-0000-0000-0001-000000000001"),
			Slug:        "sec-no-hardcoded-credentials",
			Title:       "No Hardcoded Credentials or API Keys",
			Description: "API keys, passwords, and sensitive tokens must never be committed to source files.",
			Severity:    models.SeverityCritical,
			Scope:       ScopeFile,
			Origin:      OriginLibrary,
			Status:      StatusActive,
			Inheritable: true,
			Detector: &CompiledRuleDetector{
				Type:            DetectorRegex,
				Pattern:         `(?i)(?:sk_live_|ghp_|AKIA[0-9A-Z]{16}|xoxb-[0-9]{11}|bearer\s+[a-zA-Z0-9_\-\.]{30,})`,
				NegativePattern: `(?i)(?:placeholder|example|mock|test_secret)`,
				Reason:          "Matches known high-entropy token prefix for Stripe, GitHub, AWS, or Slack",
			},
			RemediationHint: "Read credentials at runtime via environment variables or secret manager",
			CreatedAt:       time.Now().UTC(),
		},
		{
			ID:          uuid.MustParse("00000000-0000-0000-0001-000000000002"),
			Slug:        "sec-no-raw-sql-concat",
			Title:       "Prevent SQL Injection via String Concatenation",
			Description: "SQL queries must be parameterized using query placeholders instead of fmt.Sprintf or string concatenation.",
			Severity:    models.SeverityCritical,
			Scope:       ScopeFile,
			PathGlobs:   []string{"**/*.go", "**/*.ts", "**/*.js", "**/*.py"},
			Origin:      OriginLibrary,
			Status:      StatusActive,
			Inheritable: true,
			Detector: &CompiledRuleDetector{
				Type:            DetectorRegex,
				Pattern:         `(?i)(?:db\.Query|db\.Exec|db\.Raw)\s*\(\s*(?:fmt\.Sprintf|["'].*\+.*["'])`,
				NegativePattern: `(?i)(?:WHERE\s+1=1|\$1|\?)`,
				Reason:          "Dynamic string concatenation inside database query execution",
			},
			RemediationHint: "Use parameterized query parameters: db.Query(\"SELECT ... WHERE id = $1\", id)",
			CreatedAt:       time.Now().UTC(),
		},
		{
			ID:          uuid.MustParse("00000000-0000-0000-0001-000000000003"),
			Slug:        "perf-no-debug-prints-in-prod",
			Title:       "Disallow Debug Prints in Production Code",
			Description: "Avoid leaving fmt.Println, console.log, or print() statements in production services.",
			Severity:    models.SeverityLow,
			Scope:       ScopeFile,
			PathGlobs:   []string{"**/*.go", "**/*.ts", "**/*.js", "**/*.py"},
			Origin:      OriginLibrary,
			Status:      StatusActive,
			Inheritable: true,
			Detector: &CompiledRuleDetector{
				Type:            DetectorRegex,
				Pattern:         `(?m)^\s*(?:fmt\.Print(?:ln|f)?|console\.log|console\.debug)\s*\(`,
				NegativePattern: `(?i)(?:_test\.go|\.test\.ts|\.spec\.ts|test_)`,
				Reason:          "Direct unbuffered standard out debug print statement",
			},
			RemediationHint: "Replace debug prints with structured logger calls (e.g. logger.Info, logger.Debug)",
			CreatedAt:       time.Now().UTC(),
		},
		{
			ID:          uuid.MustParse("00000000-0000-0000-0001-000000000004"),
			Slug:        "arch-enforce-constant-time-token-compare",
			Title:       "Enforce Constant-Time Comparison for Secrets",
			Description: "Cryptographic secrets, hashes, and session tokens must use subtle.ConstantTimeCompare to avoid timing attacks.",
			Severity:    models.SeverityHigh,
			Scope:       ScopeFile,
			PathGlobs:   []string{"**/auth/**", "**/security/**", "**/*token*.go"},
			Origin:      OriginLibrary,
			Status:      StatusActive,
			Inheritable: true,
			RemediationHint: "Use subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1",
			CreatedAt:       time.Now().UTC(),
		},
	}

	for _, r := range builtinRules {
		if r.Detector != nil {
			_ = CompileRuleDetector(r.Detector)
		}
		c.libraryRules = append(c.libraryRules, r)
		c.rulesByID[r.ID] = r
	}
}

// AddRule registers or updates a custom rule in the catalog.
func (c *RulesCatalog) AddRule(rule *DrixyRule) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if rule.ID == uuid.Nil {
		rule.ID = uuid.New()
	}
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = time.Now().UTC()
	}
	rule.UpdatedAt = time.Now().UTC()

	if rule.Detector != nil {
		_ = CompileRuleDetector(rule.Detector)
	}

	c.rulesByID[rule.ID] = rule

	if rule.RepoID != "" && rule.RepoID != "global" {
		k := rule.OrgID.String() + ":" + rule.RepoID
		c.repoRules[k] = append(c.repoRules[k], rule)
	} else if rule.TeamID != nil && *rule.TeamID != uuid.Nil {
		k := rule.OrgID.String() + ":" + rule.TeamID.String()
		c.teamRules[k] = append(c.teamRules[k], rule)
	} else if rule.OrgID != uuid.Nil {
		c.orgRules[rule.OrgID] = append(c.orgRules[rule.OrgID], rule)
	} else {
		c.libraryRules = append(c.libraryRules, rule)
	}
}

// GetRuleByID retrieves a rule by its UUID.
func (c *RulesCatalog) GetRuleByID(id uuid.UUID) (*DrixyRule, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	r, found := c.rulesByID[id]
	return r, found
}

// GetLibraryRules returns system-wide library rules.
func (c *RulesCatalog) GetLibraryRules() []*DrixyRule {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]*DrixyRule, len(c.libraryRules))
	copy(out, c.libraryRules)
	return out
}

// GetRulesForOrg retrieves all rules created at the organization level.
func (c *RulesCatalog) GetRulesForOrg(orgID uuid.UUID) []*DrixyRule {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]*DrixyRule(nil), c.orgRules[orgID]...)
}

// GetRulesForTeam retrieves all rules created at the team level.
func (c *RulesCatalog) GetRulesForTeam(orgID, teamID uuid.UUID) []*DrixyRule {
	c.mu.RLock()
	defer c.mu.RUnlock()
	k := orgID.String() + ":" + teamID.String()
	return append([]*DrixyRule(nil), c.teamRules[k]...)
}

// GetRulesForRepo retrieves all rules scoped specifically to a repository.
func (c *RulesCatalog) GetRulesForRepo(orgID uuid.UUID, repoID string) []*DrixyRule {
	c.mu.RLock()
	defer c.mu.RUnlock()
	k := orgID.String() + ":" + repoID
	return append([]*DrixyRule(nil), c.repoRules[k]...)
}
