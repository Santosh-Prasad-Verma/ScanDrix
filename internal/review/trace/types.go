// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package trace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// DecisionType categorizes the architectural scope of a recorded decision.
type DecisionType string

const (
	DecisionArchitectural   DecisionType = "architectural_decision"
	DecisionConvention      DecisionType = "convention"
	DecisionTradeoff        DecisionType = "tradeoff"
	DecisionImplementation  DecisionType = "implementation_detail"
	DecisionSecurityPolicy  DecisionType = "security_policy"
	DecisionTooling         DecisionType = "tooling"
)

// DecisionStatus indicates the lifecycle state of an architectural decision.
type DecisionStatus string

const (
	StatusProposed   DecisionStatus = "proposed"
	StatusAccepted   DecisionStatus = "accepted"
	StatusDeprecated DecisionStatus = "deprecated"
	StatusSuperseded DecisionStatus = "superseded"
)

// DecisionOrigin indicates how the decision was authored.
type DecisionOrigin string

const (
	OriginHuman         DecisionOrigin = "human"
	OriginAgent         DecisionOrigin = "agent"
	OriginCollaborative DecisionOrigin = "collaborative"
	OriginInRepoADR     DecisionOrigin = "in_repo_adr"
)

// AuthorInfo holds metadata about who authored or proposed the decision.
type AuthorInfo struct {
	Name     string `json:"name"`
	Email    string `json:"email,omitempty"`
	Handle   string `json:"handle,omitempty"`
	AgentID  string `json:"agent_id,omitempty"` // e.g. "claude-code", "cursor", "human"
}

// TraceDecision is the canonical representation of an Architectural Decision Record or Trace entry.
type TraceDecision struct {
	ID             uuid.UUID      `json:"id"`
	OrgID          string         `json:"org_id"`
	RepoID         string         `json:"repo_id"`
	DecisionKey    string         `json:"decision_key"` // e.g. "ADR-0012" or "trace-sec-jwt"
	Title          string         `json:"title"`
	Decision       string         `json:"decision"`
	Rationale      string         `json:"rationale,omitempty"`
	Type           DecisionType   `json:"type"`
	Status         DecisionStatus `json:"status"`
	Origin         DecisionOrigin `json:"origin"`
	Author         AuthorInfo     `json:"author"`
	Scope          []string       `json:"scope,omitempty"` // Repo-relative path globs (e.g. "pkg/auth/**")
	Branch         string         `json:"branch,omitempty"`
	CommitSHA      string         `json:"commit_sha,omitempty"`
	Confidence     float64        `json:"confidence"` // 0.0 to 1.0
	Pinned         bool           `json:"pinned"`     // Never dropped by token budget
	SupersededBy   string         `json:"superseded_by,omitempty"`
	Evidence       []string       `json:"evidence,omitempty"`
	Tags           []string       `json:"tags,omitempty"`
	SourceFilePath string         `json:"source_file_path,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// EstimateTokens calculates an approximate token count for a decision in a prompt pack.
func (d *TraceDecision) EstimateTokens() int {
	textLen := len(d.Title) + len(d.Decision) + len(d.Rationale)
	for _, s := range d.Scope {
		textLen += len(s)
	}
	return (textLen + 3) / 4
}

// Fingerprint generates a deterministic hash for deduplication.
func (d *TraceDecision) Fingerprint() string {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%s:%s:%s", d.OrgID, d.RepoID, d.DecisionKey)))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// IsActive returns true if the decision is currently in effect.
func (d *TraceDecision) IsActive() bool {
	return d.Status == StatusAccepted || d.Status == StatusProposed
}

// MatchesPath checks if this decision's scope applies to a given file path.
func (d *TraceDecision) MatchesPath(filePath string) bool {
	if len(d.Scope) == 0 {
		return true // Unscoped decisions apply to the whole repository
	}
	clean := strings.TrimPrefix(strings.TrimPrefix(filePath, "./"), "/")
	for _, pattern := range d.Scope {
		patClean := strings.TrimPrefix(strings.TrimPrefix(pattern, "./"), "/")
		if matchTracePathGlob(patClean, clean) {
			return true
		}
	}
	return false
}

// matchTracePathGlob checks if a file matches a path pattern supporting ** wildcards.
func matchTracePathGlob(pattern, filePath string) bool {
	if pattern == "" || pattern == "*" || pattern == "**" {
		return true
	}
	if pattern == filePath {
		return true
	}
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return strings.HasPrefix(filePath, prefix+"/") || filePath == prefix
	}
	if strings.HasPrefix(pattern, "**/") {
		suffix := strings.TrimPrefix(pattern, "**/")
		return strings.HasSuffix(filePath, "/"+suffix) || filePath == suffix
	}
	return strings.Contains(filePath, pattern)
}

// TracePackResult represents the formatted decision context loaded for a pull request.
type TracePackResult struct {
	Decisions        []*TraceDecision `json:"decisions"`
	TotalTokens      int              `json:"total_tokens"`
	DroppedForBudget int              `json:"dropped_for_budget"`
}

// FormatPromptSlice formats decisions into a markdown prompt block.
func (p *TracePackResult) FormatPromptSlice() string {
	if len(p.Decisions) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("### ScanDrix Architectural Decisions & Trace Context\n")
	sb.WriteString("The following active architectural decisions govern this repository. Your review recommendations MUST respect these decisions:\n\n")

	for i, d := range p.Decisions {
		sb.WriteString(fmt.Sprintf("%d. **[%s] %s** (Status: `%s`, Type: `%s`)\n", i+1, d.DecisionKey, d.Title, d.Status, d.Type))
		sb.WriteString(fmt.Sprintf("   - **Decision**: %s\n", d.Decision))
		if d.Rationale != "" {
			sb.WriteString(fmt.Sprintf("   - **Rationale**: %s\n", d.Rationale))
		}
		if len(d.Scope) > 0 {
			sb.WriteString(fmt.Sprintf("   - **Scope**: `%s`\n", strings.Join(d.Scope, "`, `")))
		}
		if d.Author.Name != "" {
			sb.WriteString(fmt.Sprintf("   - **Author**: %s\n", d.Author.Name))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
