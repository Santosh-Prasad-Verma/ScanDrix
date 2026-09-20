// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package priority

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// SecurityRiskDomain classifies sensitive operational and security boundaries.
type SecurityRiskDomain string

const (
	DomainAuthCredentials   SecurityRiskDomain = "auth_credentials"
	DomainDatabaseStorage   SecurityRiskDomain = "database_storage"
	DomainAccessControlRBAC SecurityRiskDomain = "access_control_rbac"
	DomainPaymentBilling    SecurityRiskDomain = "payment_billing"
	DomainCryptography      SecurityRiskDomain = "cryptography"
	DomainNetworkGateway    SecurityRiskDomain = "network_gateway"
	DomainCoreSharedLogic   SecurityRiskDomain = "core_shared_logic"
	DomainStandardBusiness  SecurityRiskDomain = "standard_business"
	DomainTestingFixture    SecurityRiskDomain = "testing_fixture"
	DomainDocumentation     SecurityRiskDomain = "documentation"
)

// SecurityDomainConfig details risk weights and detection patterns for a domain.
type SecurityDomainConfig struct {
	Domain          SecurityRiskDomain
	WeightMultiplier float64
	PathPatterns    []string
	ContentKeywords []string
}

// DefaultDomainConfigurations provides calibrated blast-radius weights for enterprise repositories.
func DefaultDomainConfigurations() []SecurityDomainConfig {
	return []SecurityDomainConfig{
		{
			Domain:           DomainAuthCredentials,
			WeightMultiplier: 4.0,
			PathPatterns: []string{
				"**/auth/**", "**/oauth/**", "**/saml/**", "**/jwt/**", "**/token/**",
				"**/session/**", "**/credentials/**", "**/secret/**", "**/password/**",
				"**/cert/**", "**/login/**", "**/sso/**",
			},
			ContentKeywords: []string{
				"token", "jwt", "password", "secret", "bearer", "private_key", "claim",
				"authentication", "hashpassword", "signjwt", "verifytoken",
			},
		},
		{
			Domain:           DomainAccessControlRBAC,
			WeightMultiplier: 3.5,
			PathPatterns: []string{
				"**/permission/**", "**/policy/**", "**/rbac/**", "**/role/**",
				"**/guard/**", "**/authorizer/**", "**/acl/**",
			},
			ContentKeywords: []string{
				"haspermission", "checkpolicy", "canaccess", "userrole", "authorize",
				"forbidden", "unauthorized", "enforcepolicy",
			},
		},
		{
			Domain:           DomainPaymentBilling,
			WeightMultiplier: 3.5,
			PathPatterns: []string{
				"**/billing/**", "**/payment/**", "**/stripe/**", "**/checkout/**",
				"**/subscription/**", "**/invoice/**", "**/wallet/**",
			},
			ContentKeywords: []string{
				"stripe", "charge", "refund", "invoice", "creditcard", "subscription",
				"paymentintent", "customerid", "billingportal",
			},
		},
		{
			Domain:           DomainDatabaseStorage,
			WeightMultiplier: 3.0,
			PathPatterns: []string{
				"**/migrations/**", "**/database/**", "**/db/**", "**/schema/**",
				"**/*.sql", "**/repository/**", "**/models/**", "**/entities/**",
			},
			ContentKeywords: []string{
				"select", "insert", "update", "delete", "create table", "drop table",
				"alter table", "transaction", "rollback", "commit", "execquery",
			},
		},
		{
			Domain:           DomainCryptography,
			WeightMultiplier: 3.0,
			PathPatterns: []string{
				"**/crypto/**", "**/cipher/**", "**/kms/**", "**/encrypt/**", "**/tls/**",
			},
			ContentKeywords: []string{
				"aes", "rsa", "sha256", "bcrypt", "argon2", "hmac", "cipher",
				"encrypt", "decrypt", "nonce", "gcm",
			},
		},
		{
			Domain:           DomainNetworkGateway,
			WeightMultiplier: 2.2,
			PathPatterns: []string{
				"**/gateway/**", "**/proxy/**", "**/middleware/**", "**/router/**",
				"**/ingress/**", "**/egress/**", "**/cors/**",
			},
			ContentKeywords: []string{
				"cors", "reverseproxy", "forwardheader", "ratelimit", "whitelist",
				"tlsconfig", "mutualtls", "trustedproxy",
			},
		},
		{
			Domain:           DomainCoreSharedLogic,
			WeightMultiplier: 1.8,
			PathPatterns: []string{
				"**/core/**", "**/pkg/**", "**/shared/**", "**/common/**", "**/libs/**",
			},
			ContentKeywords: []string{
				"interface", "exported", "contract", "adapter", "registry",
			},
		},
		{
			Domain:           DomainTestingFixture,
			WeightMultiplier: 0.4,
			PathPatterns: []string{
				"**/*_test.go", "**/*.spec.ts", "**/*.test.ts", "**/tests/**",
				"**/mock/**", "**/fixtures/**", "**/testdata/**",
			},
		},
		{
			Domain:           DomainDocumentation,
			WeightMultiplier: 0.2,
			PathPatterns: []string{
				"**/*.md", "**/*.txt", "**/docs/**", "**/changelog/**", "LICENSE",
			},
		},
	}
}

// SecurityRiskClassifier evaluates repository file changes for security blast-radius and review criticality.
type SecurityRiskClassifier struct {
	domainConfigs []SecurityDomainConfig
	branchKeywordRegex *regexp.Regexp
}

// NewSecurityRiskClassifier constructs a risk classifier.
func NewSecurityRiskClassifier() *SecurityRiskClassifier {
	return &SecurityRiskClassifier{
		domainConfigs: DefaultDomainConfigurations(),
		branchKeywordRegex: regexp.MustCompile(`\b(if|else|for|switch|case|select|while|catch|&&|\|\|)\b`),
	}
}

// ClassifiedSecurityRisk details the security posture and blast-radius evaluation for a file.
type ClassifiedSecurityRisk struct {
	FilePath            string             `json:"file_path"`
	PrimaryDomain       SecurityRiskDomain `json:"primary_domain"`
	MatchedDomains      []SecurityRiskDomain `json:"matched_domains"`
	DomainMultiplier    float64            `json:"domain_multiplier"`
	ComplexityMultiplier float64           `json:"complexity_multiplier"`
	CompositeRiskScore  float64            `json:"composite_risk_score"`
	RecommendedTier     CoverageTier       `json:"recommended_tier"`
	SecurityFlags       []string           `json:"security_flags"`
}

// ClassifyFile evaluates path heuristics, keywords, and diff structure.
func (c *SecurityRiskClassifier) ClassifyFile(filePath string, content string, patch string) ClassifiedSecurityRisk {
	normPath := filepath.ToSlash(filepath.Clean(strings.TrimSpace(filePath)))
	lowerPath := strings.ToLower(normPath)
	lowerContent := strings.ToLower(content)
	lowerPatch := strings.ToLower(patch)

	matchedDomains := make([]SecurityRiskDomain, 0)
	highestMultiplier := 1.0
	primaryDomain := DomainStandardBusiness
	var securityFlags []string

	// Pre-filter tests and documentation: tests/docs must never be elevated to critical production tiers
	if IsTestFile(lowerPath) {
		matchedDomains = append(matchedDomains, DomainTestingFixture)
		return ClassifiedSecurityRisk{
			FilePath:             normPath,
			PrimaryDomain:        DomainTestingFixture,
			MatchedDomains:       matchedDomains,
			DomainMultiplier:     0.4,
			ComplexityMultiplier: 1.0,
			CompositeRiskScore:   0.4,
			RecommendedTier:      TierOptional,
		}
	}

	if IsDocFile(lowerPath) {
		matchedDomains = append(matchedDomains, DomainDocumentation)
		return ClassifiedSecurityRisk{
			FilePath:             normPath,
			PrimaryDomain:        DomainDocumentation,
			MatchedDomains:       matchedDomains,
			DomainMultiplier:     0.2,
			ComplexityMultiplier: 1.0,
			CompositeRiskScore:   0.2,
			RecommendedTier:      TierOptional,
		}
	}

	// Check domain matches
	for _, cfg := range c.domainConfigs {
		if cfg.Domain == DomainTestingFixture || cfg.Domain == DomainDocumentation {
			continue
		}

		matched := false

		// 1. Path glob pattern matching
		for _, pat := range cfg.PathPatterns {
			if matchPathGlobPattern(pat, lowerPath) {
				matched = true
				break
			}
		}

		// 2. Sensitive keyword matches in patch or content
		if !matched && len(cfg.ContentKeywords) > 0 {
			keywordHits := 0
			for _, kw := range cfg.ContentKeywords {
				if strings.Contains(lowerPatch, kw) || strings.Contains(lowerContent, kw) {
					keywordHits++
				}
			}
			if keywordHits >= 2 {
				matched = true
				securityFlags = append(securityFlags, fmt.Sprintf("%s: keyword density (%d keywords)", cfg.Domain, keywordHits))
			}
		}

		if matched {
			matchedDomains = append(matchedDomains, cfg.Domain)
			if cfg.WeightMultiplier > highestMultiplier {
				highestMultiplier = cfg.WeightMultiplier
				primaryDomain = cfg.Domain
			}
		}
	}

	if len(matchedDomains) == 0 {
		matchedDomains = append(matchedDomains, DomainStandardBusiness)
	}

	// 3. Compute AST cyclomatic branching complexity multiplier (1.0 to 2.5)
	complexityMultiplier := 1.0
	if patch != "" {
		branchMatches := c.branchKeywordRegex.FindAllString(patch, -1)
		branchCount := len(branchMatches)
		if branchCount > 20 {
			complexityMultiplier = 2.0
		} else if branchCount > 10 {
			complexityMultiplier = 1.5
		} else if branchCount > 4 {
			complexityMultiplier = 1.2
		}
	}

	compositeScore := highestMultiplier * complexityMultiplier

	// 4. Assign recommended coverage tier
	tier := TierWarm
	if compositeScore >= 3.0 || primaryDomain == DomainAuthCredentials || primaryDomain == DomainAccessControlRBAC || primaryDomain == DomainPaymentBilling {
		tier = TierCritical
	}

	return ClassifiedSecurityRisk{
		FilePath:             normPath,
		PrimaryDomain:        primaryDomain,
		MatchedDomains:       matchedDomains,
		DomainMultiplier:     highestMultiplier,
		ComplexityMultiplier: complexityMultiplier,
		CompositeRiskScore:   compositeScore,
		RecommendedTier:      tier,
		SecurityFlags:        securityFlags,
	}
}

// IsTestFile returns true if the file is a unit/integration test, mock, or fixture.
func IsTestFile(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(path))
	return strings.HasSuffix(lower, "_test.go") ||
		strings.HasSuffix(lower, ".spec.ts") ||
		strings.HasSuffix(lower, ".test.ts") ||
		strings.HasSuffix(lower, ".spec.js") ||
		strings.HasSuffix(lower, ".test.js") ||
		strings.Contains(lower, "/test/") ||
		strings.Contains(lower, "/tests/") ||
		strings.Contains(lower, "/mock/") ||
		strings.Contains(lower, "/mocks/") ||
		strings.Contains(lower, "/fixtures/") ||
		strings.Contains(lower, "/testdata/")
}

// IsDocFile returns true if the file is documentation or metadata.
func IsDocFile(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(path))
	return strings.HasSuffix(lower, ".md") ||
		strings.HasSuffix(lower, ".txt") ||
		strings.HasSuffix(lower, ".rst") ||
		strings.HasSuffix(lower, ".adoc") ||
		strings.HasPrefix(lower, "docs/") ||
		strings.Contains(lower, "/docs/") ||
		strings.Contains(lower, "license") ||
		strings.Contains(lower, "changelog")
}

// matchPathGlobPattern evaluates whether a path matches a glob supporting '**'.
func matchPathGlobPattern(pattern, path string) bool {
	pattern = filepath.ToSlash(strings.ToLower(pattern))
	path = filepath.ToSlash(strings.ToLower(path))

	if pattern == path {
		return true
	}

	// Suffix wildcard: "**/*.ext" or "*.ext"
	if strings.HasPrefix(pattern, "**/*.") {
		ext := strings.TrimPrefix(pattern, "**/*")
		return strings.HasSuffix(path, ext)
	}
	if strings.HasPrefix(pattern, "*.") {
		ext := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(path, ext)
	}

	// Subdirectory wildcard: "**/dir/**"
	if strings.HasPrefix(pattern, "**/") && strings.HasSuffix(pattern, "/**") {
		dir := strings.TrimSuffix(strings.TrimPrefix(pattern, "**/"), "/**")
		return strings.Contains(path, "/"+dir+"/") || strings.HasPrefix(path, dir+"/")
	}

	// Leading wildcard: "**/dir/*"
	if strings.HasPrefix(pattern, "**/") {
		sub := strings.TrimPrefix(pattern, "**/")
		parts := strings.Split(path, "/")
		for i := range parts {
			subPath := strings.Join(parts[i:], "/")
			if ok, _ := filepath.Match(sub, subPath); ok {
				return true
			}
		}
	}

	if ok, _ := filepath.Match(pattern, path); ok {
		return true
	}

	return false
}
