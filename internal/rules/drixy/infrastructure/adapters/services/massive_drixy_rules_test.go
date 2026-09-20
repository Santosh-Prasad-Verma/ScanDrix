// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System Comprehensive Test Suite
// File: massive_drixy_rules_test.go
// ═══════════════════════════════════════════════════════════════

package services_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/rules/drixy/domain/entities"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
	"github.com/scandrix/backend/internal/rules/drixy/dtos"
	"github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/repositories"
	"github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/services"
)

// ─────────────────────────────────────────────────────────────
// 1. All 808 Library Rules Verification Suite (808 test cases)
// ─────────────────────────────────────────────────────────────

func TestMassive_AllLibraryRules_808Cases(t *testing.T) {
	rulesRepo := repositories.NewPostgresDrixyRulesRepository(nil)
	ruleLikeRepo := repositories.NewPostgresRuleLikeRepository(nil)
	ruleLikeSvc := services.NewRuleLikeService(ruleLikeRepo)
	rulesSvc := services.NewDrixyRulesService(rulesRepo, ruleLikeSvc)
	compiler := services.NewDrixyRuleDetectorCompiler(rulesSvc, nil)
	sweeper := services.NewDrixyRuleDetectorSweepService()

	ctx := context.Background()
	rules, err := rulesSvc.GetLibraryDrixyRules(ctx, nil, "")
	if err != nil {
		t.Fatalf("failed to load library rules: %v", err)
	}

	if len(rules) < 800 {
		t.Fatalf("expected at least 800 library rules, found %d", len(rules))
	}

	validSeverities := map[string]bool{
		"CRITICAL": true, "HIGH": true, "MEDIUM": true, "LOW": true, "INFO": true,
	}

	for i, r := range rules {
		rule := r
		idx := i
		t.Run(fmt.Sprintf("Rule_%03d_%s", idx+1, rule.UUID), func(t *testing.T) {
			// 1. Validate UUID format or non-empty identifier
			if strings.TrimSpace(rule.UUID) == "" {
				t.Errorf("rule #%d has empty UUID", idx)
			}

			// 2. Validate Title
			if strings.TrimSpace(rule.Title) == "" {
				t.Errorf("rule #%d (%s) has empty Title", idx, rule.UUID)
			}

			// 3. Validate Rule content
			if strings.TrimSpace(rule.Rule) == "" {
				t.Errorf("rule #%d (%s) has empty Rule text", idx, rule.UUID)
			}

			// 4. Validate Severity
			normSev := strings.ToUpper(strings.TrimSpace(rule.Severity))
			if !validSeverities[normSev] {
				resolved := interfaces.ResolveDrixyRuleSeverityLevelFromString(rule.Severity)
				if !validSeverities[string(resolved)] {
					t.Errorf("rule #%d (%s) has invalid severity: %s", idx, rule.UUID, rule.Severity)
				}
			}

			// 5. Validate Examples if present
			for exIdx, ex := range rule.Examples {
				if strings.TrimSpace(ex.Snippet) == "" {
					t.Errorf("rule #%d example #%d has empty snippet", idx, exIdx)
				}
			}

			// 6. Test compilation with mechanical compiler (must never panic)
			ruleObj := interfaces.DrixyRule{
				UUID:     rule.UUID,
				Title:    rule.Title,
				Rule:     rule.Rule,
				Severity: rule.Severity,
				Status:   interfaces.DrixyRulesStatusActive,
				Examples: rule.Examples,
			}
			res, compileErr := compiler.CompileAndSave(ctx, "org-test", "team-test", rule.UUID, &ruleObj)
			if compileErr != nil {
				t.Errorf("compiler returned error for rule #%d: %v", idx, compileErr)
			}
			if res.Compiled && res.Detector != nil {
				if _, regErr := regexp.Compile(res.Detector.Pattern); regErr != nil {
					t.Errorf("compiled pattern is invalid regex for rule #%d: %v", idx, regErr)
				}
				ruleObj.Detector = res.Detector
			}

			// 7. Test sweeper execution against synthetic diff lines (must never panic)
			sampleLines := []string{
				"// " + rule.Title,
				"func Example() {",
				"    var x = 1",
				"}",
			}
			_ = sweeper.SweepDiffAddedLines(ctx, []interfaces.DrixyRule{ruleObj}, "pkg/sample.go", sampleLines)
		})
	}
}

// ─────────────────────────────────────────────────────────────
// 2. Mechanical Detector Compiler Stress Suite (1,000 test cases)
// ─────────────────────────────────────────────────────────────

func TestMassive_DetectorCompiler_1000Cases(t *testing.T) {
	rulesRepo := repositories.NewPostgresDrixyRulesRepository(nil)
	rulesSvc := services.NewDrixyRulesService(rulesRepo, nil)
	compiler := services.NewDrixyRuleDetectorCompiler(rulesSvc, nil)
	ctx := context.Background()

	// 250 mechanical syntax patterns
	mechanicalTokens := []string{
		"console.log", "ioutil.ReadFile", "md5.New", "fmt.Println", "os.Exit",
		"eval(", "SELECT * FROM", "chmod 777", "exec.Command", "password =",
		"http.Get(", "crypto/des", "math/rand", "panic(", "recover()",
		"reflect.ValueOf", "unsafe.Pointer", "TODO:", "FIXME:", "any",
		"interface{}", "var _ = ", "log.Fatal", "system(", "child_process",
	}

	// 250 architectural phrases
	architecturalPhrases := []string{
		"prefer domain driven design", "maintain modular architecture",
		"write readable functions", "keep classes small", "use dependency injection",
		"document public interfaces", "ensure high test coverage",
		"follow idiomatic style guidelines", "avoid god objects", "use clean error handling",
	}

	totalCases := 1000
	for i := 0; i < totalCases; i++ {
		caseNum := i
		var ruleText string
		var isMechanical bool

		switch {
		case caseNum < 250:
			// Mechanical syntax rule
			token := mechanicalTokens[caseNum%len(mechanicalTokens)]
			ruleText = fmt.Sprintf("Do not use `%s` in production code #%d", token, caseNum)
			isMechanical = true
		case caseNum < 500:
			// Architectural rule
			phrase := architecturalPhrases[caseNum%len(architecturalPhrases)]
			ruleText = fmt.Sprintf("Always %s across services #%d", phrase, caseNum)
			isMechanical = false
		case caseNum < 750:
			// Special characters and escaped regex patterns
			chars := []string{`\d+`, `[a-z]+`, `\s*=\s*`, `\bconst\b`, `\bvar\b`, `\bfunc\b`}
			c := chars[caseNum%len(chars)]
			ruleText = fmt.Sprintf("Pattern requirement `%s` with flags #%d", c, caseNum)
			isMechanical = true
		default:
			// Complex edge cases
			edgeTypes := []string{
				"Empty rule",
				"Rule with trailing whitespace   ",
				"Rule with unicode: 规则验证 🚀",
				"Multiline\nRule\nDefinition\nHere",
				"Rule with quotes: \"strict_mode\" and 'no-eval'",
			}
			ruleText = fmt.Sprintf("%s (id: %d)", edgeTypes[caseNum%len(edgeTypes)], caseNum)
			isMechanical = false
		}

		t.Run(fmt.Sprintf("Compiler_Case_%04d", caseNum+1), func(t *testing.T) {
			rule := &interfaces.DrixyRule{
				UUID:     fmt.Sprintf("stress-comp-%04d", caseNum),
				Title:    fmt.Sprintf("Test Rule %04d", caseNum),
				Rule:     ruleText,
				Status:   interfaces.DrixyRulesStatusActive,
				Severity: "MEDIUM",
			}

			if isMechanical {
				rule.Detector = &interfaces.DrixyRuleDetector{
					Type:    "regex",
					Pattern: regexp.QuoteMeta(ruleText),
				}
			}

			res, err := compiler.CompileAndSave(ctx, "org-stress", "team-stress", rule.UUID, rule)
			if err != nil {
				t.Fatalf("unexpected error during compilation: %v", err)
			}

			if res.Compiled {
				if res.Detector == nil || res.Detector.Pattern == "" {
					t.Fatalf("expected non-empty detector pattern when compiled is true")
				}
				if _, rErr := regexp.Compile(res.Detector.Pattern); rErr != nil {
					t.Fatalf("compiled pattern failed regexp.Compile: %v", rErr)
				}
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────
// 3. Path Matching and Scoping Suite (1,000 test cases)
// ─────────────────────────────────────────────────────────────

func TestMassive_PathMatchingAndScoping_1000Cases(t *testing.T) {
	extensions := []string{
		".go", ".ts", ".js", ".py", ".java", ".cpp", ".h", ".rs",
		".rb", ".php", ".cs", ".json", ".yaml", ".sql", ".sh", ".dockerfile",
	}

	dirs := []string{
		"src", "internal", "pkg", "cmd", "api", "controllers",
		"services", "domain", "infrastructure", "tests",
	}

	matchGlob := func(pattern, path string) bool {
		// Normalise Windows separators
		cleanPattern := strings.ReplaceAll(pattern, "\\", "/")
		cleanPath := strings.ReplaceAll(path, "\\", "/")

		if cleanPattern == "**/*" || cleanPattern == "*" {
			return true
		}
		if strings.HasPrefix(cleanPattern, "**/*.") {
			ext := cleanPattern[4:]
			return strings.HasSuffix(cleanPath, ext)
		}
		matched, err := filepath.Match(cleanPattern, filepath.Base(cleanPath))
		if err == nil && matched {
			return true
		}
		return strings.Contains(cleanPath, strings.Trim(cleanPattern, "*"))
	}

	totalCases := 1000
	for i := 0; i < totalCases; i++ {
		caseNum := i
		ext := extensions[caseNum%len(extensions)]
		dir := dirs[(caseNum/len(extensions))%len(dirs)]

		var pattern string
		var testPath string
		var expectMatch bool

		switch {
		case caseNum < 250:
			// Standard extension globs: "**/*" + ext
			pattern = "**/*" + ext
			testPath = fmt.Sprintf("%s/module_%d/file_%d%s", dir, caseNum, caseNum, ext)
			expectMatch = true
		case caseNum < 500:
			// Mismatched extension globs
			altExt := extensions[(caseNum+1)%len(extensions)]
			pattern = "**/*" + ext
			testPath = fmt.Sprintf("%s/module_%d/file_%d%s", dir, caseNum, caseNum, altExt)
			expectMatch = (ext == altExt)
		case caseNum < 750:
			// Deeply nested subdirectories (depth 5 to 15)
			depth := 5 + (caseNum % 10)
			parts := make([]string, depth)
			for d := 0; d < depth; d++ {
				parts[d] = fmt.Sprintf("sub_%d", d)
			}
			parts = append(parts, fmt.Sprintf("deep_file_%d%s", caseNum, ext))
			testPath = strings.Join(parts, "/")
			pattern = "**/*" + ext
			expectMatch = true
		default:
			// Hidden files, config files, dotfiles, root files
			specialFiles := []string{
				".github/workflows/ci.yml", ".env.example", ".gitignore",
				"Dockerfile", "Makefile", "README.md", "go.mod", "package.json",
			}
			spec := specialFiles[caseNum%len(specialFiles)]
			testPath = spec
			pattern = "**/*"
			expectMatch = true
		}

		t.Run(fmt.Sprintf("Path_Case_%04d", caseNum+1), func(t *testing.T) {
			res := matchGlob(pattern, testPath)
			if res != expectMatch {
				t.Errorf("pattern %q against path %q: expected match=%v, got %v", pattern, testPath, expectMatch, res)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────
// 4. Real-Time Diff Sweeper Stress Suite (1,000 test cases)
// ─────────────────────────────────────────────────────────────

func TestMassive_SweeperStress_1000Cases(t *testing.T) {
	sweeper := services.NewDrixyRuleDetectorSweepService()
	ctx := context.Background()

	// Base set of compiled active rules
	rules := []interfaces.DrixyRule{
		{
			UUID:   "rule-sec-01",
			Title:  "Disallow eval()",
			Status: interfaces.DrixyRulesStatusActive,
			Detector: &interfaces.DrixyRuleDetector{
				Type:    "regex",
				Pattern: `\beval\s*\(`,
			},
		},
		{
			UUID:   "rule-sec-02",
			Title:  "Disallow hardcoded credentials",
			Status: interfaces.DrixyRulesStatusActive,
			Detector: &interfaces.DrixyRuleDetector{
				Type:    "regex",
				Pattern: `(?i)(password|secret|api_key)\s*[:=]\s*["'][A-Za-z0-9_\-]{8,}["']`,
			},
		},
		{
			UUID:   "rule-sec-03",
			Title:  "Disallow ioutil.ReadFile",
			Status: interfaces.DrixyRulesStatusActive,
			Detector: &interfaces.DrixyRuleDetector{
				Type:    "regex",
				Pattern: `ioutil\.ReadFile\(`,
			},
		},
		{
			UUID:   "rule-sec-04",
			Title:  "Disallow fmt.Println in production",
			Status: interfaces.DrixyRulesStatusActive,
			Detector: &interfaces.DrixyRuleDetector{
				Type:    "regex",
				Pattern: `fmt\.Println\(`,
			},
		},
	}

	totalCases := 1000
	for i := 0; i < totalCases; i++ {
		caseNum := i
		var diffLines []string
		var expectedMatches int

		switch caseNum % 5 {
		case 0:
			// Triggers rule-sec-01 (eval)
			diffLines = []string{
				"const result = eval(userStringInput);",
				"console.log('done');",
			}
			expectedMatches = 1
		case 1:
			// Triggers rule-sec-02 (hardcoded credentials)
			diffLines = []string{
				`const api_key = "super_secret_token_12345";`,
				`return connect(api_key);`,
			}
			expectedMatches = 1
		case 2:
			// Triggers rule-sec-03 (ioutil.ReadFile)
			diffLines = []string{
				`data, err := ioutil.ReadFile("/etc/config.json")`,
				`if err != nil { return err }`,
			}
			expectedMatches = 1
		case 3:
			// Triggers rule-sec-04 (fmt.Println)
			diffLines = []string{
				`fmt.Println("debug line:", value)`,
				`return nil`,
			}
			expectedMatches = 1
		case 4:
			// Clean lines - triggers 0 matches
			diffLines = []string{
				`func ProcessOrder(ctx context.Context, id string) error {`,
				`    return service.Execute(ctx, id)`,
				`}`,
			}
			expectedMatches = 0
		}

		t.Run(fmt.Sprintf("Sweeper_Case_%04d", caseNum+1), func(t *testing.T) {
			filePath := fmt.Sprintf("src/pkg_%d/service_%d.go", caseNum/10, caseNum)
			matches := sweeper.SweepDiffAddedLines(ctx, rules, filePath, diffLines)

			if len(matches) != expectedMatches {
				t.Errorf("case #%d: expected %d matches, got %d", caseNum, expectedMatches, len(matches))
			}

			// Verify match payload metadata
			for _, m := range matches {
				if m.RuleUUID == "" || m.RuleTitle == "" || m.LineNumber <= 0 || m.LineContent == "" {
					t.Errorf("invalid match metadata: %+v", m)
				}
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────
// 5. Entity Transformations, Buckets & DTOs Suite (500 test cases)
// ─────────────────────────────────────────────────────────────

func TestMassive_EntityTransformationsAndBuckets_500Cases(t *testing.T) {
	rulesRepo := repositories.NewPostgresDrixyRulesRepository(nil)
	rulesSvc := services.NewDrixyRulesService(rulesRepo, nil)
	ctx := context.Background()

	// 1. Verify all 46 buckets from catalog
	buckets, err := rulesSvc.GetLibraryDrixyRulesBuckets(ctx)
	if err != nil {
		t.Fatalf("failed to load catalog buckets: %v", err)
	}

	for bIdx, b := range buckets {
		bucket := b
		idx := bIdx
		t.Run(fmt.Sprintf("Bucket_%02d_%s", idx+1, bucket.Slug), func(t *testing.T) {
			if strings.TrimSpace(bucket.Slug) == "" {
				t.Errorf("bucket #%d has empty slug", idx)
			}
			if strings.TrimSpace(bucket.Title) == "" {
				t.Errorf("bucket #%d (%s) has empty title", idx, bucket.Slug)
			}
			if strings.TrimSpace(bucket.Description) == "" {
				t.Errorf("bucket #%d (%s) has empty description", idx, bucket.Slug)
			}
		})
	}

	// 2. 454 DTO & Entity Serialization, Immutability and Round-Trip Cases
	severities := []string{"CRITICAL", "HIGH", "MEDIUM", "LOW", "INFO"}
	statuses := []interfaces.DrixyRulesStatus{
		interfaces.DrixyRulesStatusActive,
		interfaces.DrixyRulesStatusPending,
		interfaces.DrixyRulesStatusRejected,
		interfaces.DrixyRulesStatusApplied,
		interfaces.DrixyRulesStatusDeleted,
		interfaces.DrixyRulesStatusPaused,
	}
	scopes := []interfaces.DrixyRulesScope{
		interfaces.DrixyRulesScopeFile,
		interfaces.DrixyRulesScopePullRequest,
	}

	remainingCases := 500 - len(buckets)
	for i := 0; i < remainingCases; i++ {
		caseNum := i
		sev := severities[caseNum%len(severities)]
		stat := statuses[caseNum%len(statuses)]
		sc := scopes[caseNum%len(scopes)]

		t.Run(fmt.Sprintf("Entity_Case_%03d", caseNum+1), func(t *testing.T) {
			ruleID := uuid.New().String()
			orgID := uuid.New().String()
			now := time.Now().UTC()

			dto := dtos.CreateDrixyRuleDto{
				UUID:         ruleID,
				Title:        fmt.Sprintf("Entity Test Rule %d", caseNum),
				Rule:         fmt.Sprintf("Ensure correct encapsulation and invariants for entity #%d", caseNum),
				Severity:     sev,
				Status:       stat,
				Scope:        sc,
				Path:         "**/*.go",
				RepositoryID: fmt.Sprintf("repo-%d", caseNum%10),
			}

			// Convert DTO to Domain Rule
			rule := dto.ToRule()
			rule.CreatedAt = &now
			rule.UpdatedAt = &now

			// Construct Domain Entity
			entity := entities.NewDrixyRulesEntity(interfaces.DrixyRules{
				UUID:           uuid.New().String(),
				OrganizationID: orgID,
				Rules:          []interfaces.DrixyRule{rule},
				CreatedAt:      &now,
				UpdatedAt:      &now,
			})

			// Validate Entity Getters
			if entity.OrganizationID() != orgID {
				t.Errorf("entity OrganizationID mismatch: expected %s, got %s", orgID, entity.OrganizationID())
			}
			if len(entity.Rules()) != 1 {
				t.Fatalf("expected 1 rule in entity, got %d", len(entity.Rules()))
			}
			if entity.Rules()[0].UUID != ruleID {
				t.Errorf("rule UUID mismatch: expected %s, got %s", ruleID, entity.Rules()[0].UUID)
			}
			if entity.Rules()[0].Severity != sev {
				t.Errorf("severity mismatch: expected %s, got %s", sev, entity.Rules()[0].Severity)
			}

			// JSON Serialization Round-Trip
			data, jErr := json.Marshal(rule)
			if jErr != nil {
				t.Fatalf("failed to marshal rule to JSON: %v", jErr)
			}

			var unmarshaled interfaces.DrixyRule
			if uErr := json.Unmarshal(data, &unmarshaled); uErr != nil {
				t.Fatalf("failed to unmarshal rule from JSON: %v", uErr)
			}

			if unmarshaled.UUID != ruleID || unmarshaled.Title != rule.Title {
				t.Errorf("JSON round-trip mismatch: %+v vs %+v", unmarshaled, rule)
			}
		})
	}
}
