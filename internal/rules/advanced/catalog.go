package advanced

import (
	"regexp"

	"github.com/google/uuid"
)

// EnterpriseRuleDefinition encapsulates an OWASP / CWE security rule.
type EnterpriseRuleDefinition struct {
	ID          uuid.UUID      `json:"id"`
	RuleKey     string         `json:"rule_key"`
	Title       string         `json:"title"`
	CWE         string         `json:"cwe"`
	OWASPCat    string         `json:"owasp_cat"`
	Severity    string         `json:"severity"` // critical, high, medium, low
	Description string         `json:"description"`
	Remediation string         `json:"remediation"`
	RegexMatch  *regexp.Regexp `json:"-"`
	PatternRaw  string         `json:"pattern_raw"`
}

// BuiltinEnterpriseCatalog returns standard enterprise security detectors.
func BuiltinEnterpriseCatalog() []EnterpriseRuleDefinition {
	return []EnterpriseRuleDefinition{
		{
			ID:          uuid.MustParse("10000000-0000-0000-0000-000000000001"),
			RuleKey:     "SEC-CWE-89-SQLI",
			Title:       "SQL Injection via Unsanitized Concatenation",
			CWE:         "CWE-89",
			OWASPCat:    "A03:2021-Injection",
			Severity:    "critical",
			Description: "Detects direct string concatenation or formatting into SQL execution functions.",
			Remediation: "Use parameterized queries or prepared statements instead of string concatenation.",
			PatternRaw:  `(?i)(SELECT|INSERT|UPDATE|DELETE).*\+.*(?:req|input|param|query)`,
			RegexMatch:  regexp.MustCompile(`(?i)(SELECT|INSERT|UPDATE|DELETE).*\+.*(?:req|input|param|query)`),
		},
		{
			ID:          uuid.MustParse("10000000-0000-0000-0000-000000000002"),
			RuleKey:     "SEC-CWE-78-OS-INJECT",
			Title:       "OS Command Injection via Shell Execution",
			CWE:         "CWE-78",
			OWASPCat:    "A03:2021-Injection",
			Severity:    "critical",
			Description: "Detects untrusted input passed directly into shell commands (/bin/sh, bash -c).",
			Remediation: "Pass arguments as separate array elements to exec.Command without invoking a shell.",
			PatternRaw:  `exec\.Command\s*\(\s*["'](?:bash|sh)["']\s*,\s*["']-c["']\s*,\s*.*\+`,
			RegexMatch:  regexp.MustCompile(`exec\.Command\s*\(\s*["'](?:bash|sh)["']\s*,\s*["']-c["']\s*,\s*.*\+`),
		},
		{
			ID:          uuid.MustParse("10000000-0000-0000-0000-000000000003"),
			RuleKey:     "SEC-CWE-798-HARDCODED-SECRET",
			Title:       "Hardcoded Cloud Credentials or API Tokens",
			CWE:         "CWE-798",
			OWASPCat:    "A07:2021-Identification and Authentication Failures",
			Severity:    "critical",
			Description: "Detects hardcoded cloud credentials (AWS keys, private keys, GitHub tokens).",
			Remediation: "Store secrets in environment variables (.env) or a cloud secret manager.",
			PatternRaw:  `(AKIA[0-9A-Z]{16}|-----BEGIN (?:RSA )?PRIVATE KEY-----|ghp_[0-9a-zA-Z]{36})`,
			RegexMatch:  regexp.MustCompile(`(AKIA[0-9A-Z]{16}|-----BEGIN (?:RSA )?PRIVATE KEY-----|ghp_[0-9a-zA-Z]{36})`),
		},
		{
			ID:          uuid.MustParse("10000000-0000-0000-0000-000000000004"),
			RuleKey:     "SEC-CWE-22-PATH-TRAVERSAL",
			Title:       "Path Traversal via Unvalidated Filepath",
			CWE:         "CWE-22",
			OWASPCat:    "A01:2021-Broken Access Control",
			Severity:    "high",
			Description: "Detects direct joining of user input into file paths without filepath.Clean or boundary check.",
			Remediation: "Use filepath.Clean and verify strings.HasPrefix(targetPath, baseDir).",
			PatternRaw:  `os\.Open(?:File)?\s*\(\s*filepath\.Join\s*\(.*,\s*(?:req|input|filename)\s*\)`,
			RegexMatch:  regexp.MustCompile(`os\.Open(?:File)?\s*\(\s*filepath\.Join\s*\(.*,\s*(?:req|input|filename)\s*\)`),
		},
		{
			ID:          uuid.MustParse("10000000-0000-0000-0000-000000000005"),
			RuleKey:     "SEC-CWE-327-BROKEN-CRYPTO",
			Title:       "Use of Broken Cryptographic Algorithm",
			CWE:         "CWE-327",
			OWASPCat:    "A02:2021-Cryptographic Failures",
			Severity:    "high",
			Description: "Detects insecure hash algorithms (MD5, SHA1, DES) used in security-sensitive contexts.",
			Remediation: "Use SHA-256, SHA-512, or AES-GCM.",
			PatternRaw:  `(?i)(md5\.New|sha1\.New|des\.NewCipher)`,
			RegexMatch:  regexp.MustCompile(`(?i)(md5\.New|sha1\.New|des\.NewCipher)`),
		},
		{
			ID:          uuid.MustParse("10000000-0000-0000-0000-000000000006"),
			RuleKey:     "SEC-CWE-295-DISABLED-TLS",
			Title:       "Disabled TLS Certificate Verification",
			CWE:         "CWE-295",
			OWASPCat:    "A07:2021-Identification and Authentication Failures",
			Severity:    "critical",
			Description: "Detects InsecureSkipVerify: true disabling TLS certificate validation.",
			Remediation: "Always verify TLS certificates using valid system CA trust pools.",
			PatternRaw:  `InsecureSkipVerify\s*:\s*true`,
			RegexMatch:  regexp.MustCompile(`InsecureSkipVerify\s*:\s*true`),
		},
	}
}
