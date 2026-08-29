package rules

import (
	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// DefaultCatalog returns the standard suite of production security and defect rules.
func DefaultCatalog() []RuleSpec {
	return []RuleSpec{
		{
			ID:          uuid.MustParse("00000000-0000-0000-0001-000000000001"),
			Name:        "Hardcoded AWS Access Key",
			PathPattern: "*",
			RegexRule:   `(?i)(AKIA|A3T|AGPA|AIDA|AROA|AIPA|ANPA|ANVA|ASIA)[A-Z0-9]{16}`,
			Severity:    models.SeverityCritical,
			Category:    "SECURITY_SECRET",
			Description: "Detected potential hardcoded AWS Access Key ID in source code (Master Rule 1.1).",
			Remediation: "Remove the key immediately, rotate it in AWS IAM, and load from environment variables (process.env or os.Getenv).",
		},
		{
			ID:          uuid.MustParse("00000000-0000-0000-0001-000000000002"),
			Name:        "Hardcoded GitHub Personal Access Token",
			PathPattern: "*",
			RegexRule:   `ghp_[0-9a-zA-Z]{36}`,
			Severity:    models.SeverityCritical,
			Category:    "SECURITY_SECRET",
			Description: "Detected cleartext GitHub Personal Access Token.",
			Remediation: "Revoke the token on GitHub immediately and inject via Doppler or environment secrets.",
		},
		{
			ID:          uuid.MustParse("00000000-0000-0000-0001-000000000003"),
			Name:        "Hardcoded Private Key Block",
			PathPattern: "*",
			RegexRule:   `-----BEGIN (RSA|EC|OPENSSH|PGP|ENCRYPTED)? ?PRIVATE KEY-----`,
			Severity:    models.SeverityCritical,
			Category:    "SECURITY_SECRET",
			Description: "Asymmetric private cryptographic key committed to repository.",
			Remediation: "Delete the key, revoke certificates associated with it, and store in HashiCorp Vault or Cloud KMS.",
		},
		{
			ID:          uuid.MustParse("00000000-0000-0000-0001-000000000004"),
			Name:        "SQL Injection via String Formatting",
			PathPattern: "*.go",
			RegexRule:   `fmt\.Sprintf\(["'].*(SELECT|INSERT|UPDATE|DELETE).*FROM`,
			Severity:    models.SeverityCritical,
			Category:    "SECURITY_INJECTION",
			Description: "Concatenating or formatting untrusted user input directly into SQL statements causes SQL injection.",
			Remediation: "Use parameterized queries or prepared statements ($1, $2) via pgx or database/sql.",
		},
		{
			ID:          uuid.MustParse("00000000-0000-0000-0001-000000000005"),
			Name:        "Shell Command Injection Vulnerability",
			PathPattern: "*.go",
			RegexRule:   `exec\.Command\(["'](sh|bash|zsh)["'],\s*["']-c["']`,
			Severity:    models.SeverityHigh,
			Category:    "SECURITY_INJECTION",
			Description: "Spawning a subshell with -c and concatenated arguments exposes the host to arbitrary command injection.",
			Remediation: "Pass individual argument slices to exec.Command without invoking an intermediary shell.",
		},
		{
			ID:          uuid.MustParse("00000000-0000-0000-0001-000000000006"),
			Name:        "Weak Cryptographic Hash (MD5 / SHA-1)",
			PathPattern: "*.go",
			RegexRule:   `crypto\/(md5|sha1)\.New\(\)`,
			Severity:    models.SeverityHigh,
			Category:    "SECURITY_CRYPTO",
			Description: "MD5 and SHA-1 suffer from known collision attacks and are cryptographically broken.",
			Remediation: "Upgrade to crypto/sha256 or crypto/sha512. For password hashing, use argon2 or bcrypt.",
		},
	}
}
