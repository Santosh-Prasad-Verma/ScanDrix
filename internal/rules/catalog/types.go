package catalog

import (
	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// RuleDefinition encapsulates an enterprise-grade out-of-the-box rule specification.
type RuleDefinition struct {
	ID          uuid.UUID              `json:"id"`
	Code        string                 `json:"code"`
	Name        string                 `json:"name"`
	Category    string                 `json:"category"`
	Language    string                 `json:"language"` // "ALL", "GO", "PYTHON", "TYPESCRIPT", "JAVA", "CPP", "DOCKER", "CLOUD"
	PathPattern string                 `json:"path_pattern"`
	Severity    models.FindingSeverity `json:"severity"`
	CWE         string                 `json:"cwe,omitempty"`
	OWASP       string                 `json:"owasp,omitempty"`
	RegexRule   string                 `json:"regex_rule"`
	Description string                 `json:"description"`
	Remediation string                 `json:"remediation"`
}

// GenerateDeterministicUUID generates a fixed deterministic UUID based on rule code.
func GenerateDeterministicUUID(code string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceDNS, []byte("rules.scandrix.dev:"+code))
}
