package sast

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/codehound/codehound/shared/domain"
	"github.com/google/uuid"
)

// SASTRule represents a deterministic security rule for AST/pattern matching.
type SASTRule struct {
	ID          string
	CWEID       string
	OWASPCat    string
	Severity    domain.FindingSeverity
	Title       string
	Description string
	Pattern     *regexp.Regexp
	Languages   []string
}

// SARIFReport represents the standard SARIF v2.1.0 JSON format.
type SARIFReport struct {
	Version string     `json:"version"`
	Schema  string     `json:"$schema"`
	Runs    []SARIFRun `json:"runs"`
}

type SARIFRun struct {
	Tool    SARIFTool     `json:"tool"`
	Results []SARIFResult `json:"results"`
}

type SARIFTool struct {
	Driver SARIFDriver `json:"driver"`
}

type SARIFDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version"`
	InformationURI string      `json:"informationUri"`
	Rules          []SARIFRule `json:"rules"`
}

type SARIFRule struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	ShortDescription struct {
		Text string `json:"text"`
	} `json:"shortDescription"`
}

type SARIFResult struct {
	RuleID    string `json:"ruleId"`
	Level     string `json:"level"`
	Message   struct {
		Text string `json:"text"`
	} `json:"message"`
	Locations []SARIFLocation `json:"locations"`
}

type SARIFLocation struct {
	PhysicalLocation SARIFPhysicalLocation `json:"physicalLocation"`
}

type SARIFPhysicalLocation struct {
	ArtifactLocation struct {
		URI string `json:"uri"`
	} `json:"artifactLocation"`
	Region struct {
		StartLine int `json:"startLine"`
	} `json:"region"`
}

// Engine executes SAST pattern scans across repository files.
type Engine struct {
	rules []SASTRule
}

// NewEngine initializes a SAST Engine with built-in OWASP Top 10 and CWE rules.
func NewEngine() *Engine {
	return &Engine{
		rules: defaultRules(),
	}
}

func defaultRules() []SASTRule {
	return []SASTRule{
		{
			ID:          "CODEHOUND-SAST-SQLI-001",
			CWEID:       "CWE-89",
			OWASPCat:    "A03:2021-Injection",
			Severity:    domain.FindingSeverityCritical,
			Title:       "SQL Injection via String Concatenation or Formatted Query",
			Description: "Detected raw SQL query constructed with string formatting or concatenation rather than parameterized placeholders ($1, ?).",
			Pattern:     regexp.MustCompile(`(?i)(db\.(Query|Exec)|pool\.Exec|cursor\.execute|client\.query)\s*\(|fmt\.Sprintf\s*\(\s*["'].*?(SELECT|INSERT|UPDATE|DELETE).*?%[sv]`),
			Languages:   []string{"go", "python", "typescript", "javascript"},
		},
		{
			ID:          "CODEHOUND-SAST-CMDI-001",
			CWEID:       "CWE-78",
			OWASPCat:    "A03:2021-Injection",
			Severity:    domain.FindingSeverityCritical,
			Title:       "OS Command Injection via Shell Execution",
			Description: "Unvalidated input passed to child_process.exec, os.system, or exec.Command without argument separation.",
			Pattern:     regexp.MustCompile(`(?i)(exec\.Command\s*\(\s*(?:sh|bash|cmd|powershell)\s*,\s*["']-c["']\s*,\s*fmt\.Sprintf|os\.system\s*\(|child_process\.exec\s*\()`),
			Languages:   []string{"go", "python", "typescript", "javascript"},
		},
		{
			ID:          "CODEHOUND-SAST-XSS-001",
			CWEID:       "CWE-79",
			OWASPCat:    "A03:2021-Injection",
			Severity:    domain.FindingSeverityHigh,
			Title:       "Cross-Site Scripting (XSS) via dangerouslySetInnerHTML or Unescaped HTML",
			Description: "Directly injecting raw untrusted HTML into the DOM via dangerouslySetInnerHTML or innerHTML.",
			Pattern:     regexp.MustCompile(`(?i)(dangerouslySetInnerHTML\s*=\s*\{\s*\{\s*__html\s*:|\.innerHTML\s*=\s*)`),
			Languages:   []string{"typescript", "javascript", "jsx", "tsx"},
		},
		{
			ID:          "CODEHOUND-SAST-CRYPTO-001",
			CWEID:       "CWE-327",
			OWASPCat:    "A02:2021-Cryptographic Failures",
			Severity:    domain.FindingSeverityHigh,
			Title:       "Use of Broken or Weak Cryptographic Hash (MD5 / SHA1)",
			Description: "Detected use of MD5 or SHA1 for hashing or signatures. Use SHA-256, SHA-3, or Argon2id instead.",
			Pattern:     regexp.MustCompile(`(?i)(md5\.New\(\)|md5\.Sum\(|crypto\.createHash\s*\(\s*["']md5["']|hashlib\.md5\(|sha1\.New\(\))`),
			Languages:   []string{"go", "python", "typescript", "javascript"},
		},
		{
			ID:          "CODEHOUND-SAST-PATHTRAV-001",
			CWEID:       "CWE-22",
			OWASPCat:    "A01:2021-Broken Access Control",
			Severity:    domain.FindingSeverityHigh,
			Title:       "Path Traversal via Unsanitized File Access",
			Description: "Opening file paths constructed from dynamic input without filepath.Clean or directory path boundary verification.",
			Pattern:     regexp.MustCompile(`(?i)(os\.Open\s*\(\s*(?:fmt\.Sprintf\(|[a-zA-Z0-9_]+\s*\+)|fs\.readFileSync\s*\(\s*(?:path\.join\(req\.|` + "`" + `.*?req\.)|open\s*\(\s*(?:f["']|[a-zA-Z0-9_]+\s*\+))`),
			Languages:   []string{"go", "python", "typescript", "javascript"},
		},
		{
			ID:          "CODEHOUND-SAST-CORS-001",
			CWEID:       "CWE-942",
			OWASPCat:    "A05:2021-Security Misconfiguration",
			Severity:    domain.FindingSeverityMedium,
			Title:       "Overly Permissive Cross-Origin Resource Sharing (CORS Wildcard)",
			Description: "Setting Access-Control-Allow-Origin to '*' with credentials allowed enables cross-domain credential harvesting.",
			Pattern:     regexp.MustCompile(`(?i)(AllowOrigins\s*:\s*\[\s*["']\*["']\s*\]|Access-Control-Allow-Origin["']\s*,\s*["']\*["']|cors\(\s*\{\s*origin\s*:\s*["']\*["'])`),
			Languages:   []string{"go", "typescript", "javascript", "python"},
		},
	}
}

// ScanFile inspects source code content and returns detected SAST vulnerabilities.
func (e *Engine) ScanFile(tenantID, projectID, scanID uuid.UUID, filePath, language string, content []byte) ([]domain.Finding, []domain.Evidence) {
	var findings []domain.Finding
	var evidences []domain.Evidence

	lines := strings.Split(string(content), "\n")

	for _, rule := range e.rules {
		// Check language applicability
		applicable := false
		for _, l := range rule.Languages {
			if strings.EqualFold(l, language) {
				applicable = true
				break
			}
		}
		if !applicable && len(rule.Languages) > 0 {
			continue
		}

		for i, line := range lines {
			lineNum := i + 1
			if rule.Pattern.MatchString(line) {
				findingID := uuid.New()
				cwe := rule.CWEID

				// Compute deterministic canonical key
				keyRaw := fmt.Sprintf("%s:%s:%d:%s", rule.ID, filePath, lineNum, rule.CWEID)
				hash := sha256.Sum256([]byte(keyRaw))
				canonicalKey := fmt.Sprintf("SAST-%s-%s", rule.CWEID, hex.EncodeToString(hash[:8]))

				finding := domain.Finding{
					ID:              findingID,
					TenantID:        tenantID,
					ProjectID:       projectID,
					CanonicalKey:    canonicalKey,
					Category:        domain.FindingCategorySecurityVuln,
					Severity:        rule.Severity,
					State:           domain.FindingStateVerifiedProven,
					Confidence:      0.95,
					Title:           rule.Title,
					Description:     rule.Description,
					PrimaryFile:     filePath,
					PrimaryLine:     lineNum,
					CWEID:           &cwe,
					FirstSeenScanID: scanID,
					LastSeenScanID:  scanID,
				}

				evidence := domain.Evidence{
					ID:             uuid.New(),
					FindingID:      findingID,
					EvidenceType:   "SAST_PATTERN_MATCH",
					SourceAnalyzer: "CODEHOUND_SEMGREP_SAST",
					Strength:       "HIGH",
					Summary:        fmt.Sprintf("Rule %s matched on line %d: %s", rule.ID, lineNum, rule.Title),
					Payload: map[string]any{
						"rule_id":     rule.ID,
						"cwe_id":      rule.CWEID,
						"owasp_cat":   rule.OWASPCat,
						"line_number": lineNum,
						"code_sample": strings.TrimSpace(line),
					},
				}

				findings = append(findings, finding)
				evidences = append(evidences, evidence)
			}
		}
	}

	return findings, evidences
}

// ParseSARIF parses a SARIF v2.1.0 JSON report (from Semgrep/Trivy) into CodeHound Findings.
func ParseSARIF(tenantID, projectID, scanID uuid.UUID, sarifJSON []byte) ([]domain.Finding, []domain.Evidence, error) {
	var report SARIFReport
	if err := json.Unmarshal(sarifJSON, &report); err != nil {
		return nil, nil, fmt.Errorf("failed to parse SARIF JSON: %w", err)
	}

	var findings []domain.Finding
	var evidences []domain.Evidence

	for _, run := range report.Runs {
		toolName := run.Tool.Driver.Name
		if toolName == "" {
			toolName = "SEMGREP"
		}

		for _, result := range run.Results {
			findingID := uuid.New()
			filePath := "unknown"
			lineNum := 1

			if len(result.Locations) > 0 {
				filePath = result.Locations[0].PhysicalLocation.ArtifactLocation.URI
				lineNum = result.Locations[0].PhysicalLocation.Region.StartLine
				if lineNum <= 0 {
					lineNum = 1
				}
			}

			sev := domain.FindingSeverityMedium
			switch strings.ToLower(result.Level) {
			case "error":
				sev = domain.FindingSeverityHigh
			case "warning":
				sev = domain.FindingSeverityMedium
			case "note", "none":
				sev = domain.FindingSeverityLow
			}

			keyRaw := fmt.Sprintf("%s:%s:%d", result.RuleID, filePath, lineNum)
			hash := sha256.Sum256([]byte(keyRaw))
			canonicalKey := fmt.Sprintf("SARIF-%s-%s", toolName, hex.EncodeToString(hash[:8]))

			ruleID := result.RuleID
			finding := domain.Finding{
				ID:              findingID,
				TenantID:        tenantID,
				ProjectID:       projectID,
				CanonicalKey:    canonicalKey,
				Category:        domain.FindingCategorySecurityVuln,
				Severity:        sev,
				State:           domain.FindingStateSuspected,
				Confidence:      0.90,
				Title:           fmt.Sprintf("[%s] %s", toolName, result.RuleID),
				Description:     result.Message.Text,
				PrimaryFile:     filePath,
				PrimaryLine:     lineNum,
				CWEID:           &ruleID,
				FirstSeenScanID: scanID,
				LastSeenScanID:  scanID,
			}

			evidence := domain.Evidence{
				ID:             uuid.New(),
				FindingID:      findingID,
				EvidenceType:   "SARIF_IMPORT",
				SourceAnalyzer: toolName,
				Strength:       "MEDIUM",
				Summary:        result.Message.Text,
				Payload: map[string]any{
					"sarif_rule_id": result.RuleID,
					"sarif_level":   result.Level,
					"file_path":     filePath,
					"line_number":   lineNum,
				},
			}

			findings = append(findings, finding)
			evidences = append(evidences, evidence)
		}
	}

	return findings, evidences, nil
}
