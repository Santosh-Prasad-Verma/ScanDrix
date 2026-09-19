package agentfirewall

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// DangerTier classifies the risk boundary of an AI tool execution.
type DangerTier string

const (
	Tier1ReadOnly    DangerTier = "TIER_1_READ_ONLY"
	Tier2SafeCompute DangerTier = "TIER_2_SAFE_COMPUTE"
	Tier3ActiveProbe DangerTier = "TIER_3_ACTIVE_PROBE"
	Tier4Mutating    DangerTier = "TIER_4_MUTATING"
)

// ToolCallRequest represents an AI agent's request to execute an external tool.
type ToolCallRequest struct {
	ToolName    string         `json:"tool_name"`
	Parameters  map[string]any `json:"parameters"`
	WorkspaceID uuid.UUID      `json:"workspace_id"`
	AgentID     string         `json:"agent_id"`
	TargetRepo  string         `json:"target_repo,omitempty"`
	TargetURL   string         `json:"target_url,omitempty"`
	DiffPayload string         `json:"diff_payload,omitempty"`
}

// ApprovalStatus tracks human-in-the-loop governance states.
type ApprovalStatus string

const (
	StatusPending  ApprovalStatus = "PENDING"
	StatusApproved ApprovalStatus = "APPROVED"
	StatusRejected ApprovalStatus = "REJECTED"
)

// ApprovalTicket is an immutable request requiring human cryptographic approval.
type ApprovalTicket struct {
	ID           string         `json:"ticket_id"`
	ToolName     string         `json:"tool_name"`
	DangerTier   DangerTier     `json:"danger_tier"`
	Status       ApprovalStatus `json:"status"`
	WorkspaceID  uuid.UUID      `json:"workspace_id"`
	Requester    string         `json:"requester"`
	DiffSHA256   string         `json:"diff_sha256,omitempty"`
	ApprovedBy   string         `json:"approved_by,omitempty"`
	Signature    string         `json:"signature,omitempty"`
	RejectReason string         `json:"reject_reason,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	ResolvedAt   *time.Time     `json:"resolved_at,omitempty"`
}

// EvaluationResult contains the verdict of the Agent Firewall.
type EvaluationResult struct {
	Allowed         bool            `json:"allowed"`
	RequiresHITL    bool            `json:"requires_hitl"`
	DangerTier      DangerTier      `json:"danger_tier"`
	Reason          string          `json:"reason"`
	ApprovalTicket  *ApprovalTicket `json:"approval_ticket,omitempty"`
	PromptInjection bool            `json:"prompt_injection"`
}

var promptInjectionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)ignore\s+(all\s+)?(previous|prior)\s+instructions`),
	regexp.MustCompile(`(?i)disregard\s+(all\s+)?(prior|previous|security)\s+(instructions|guidelines|rules)`),
	regexp.MustCompile(`(?i)you\s+are\s+now\s+(in\s+)?(dan|jailbreak|unfiltered)\s+mode`),
	regexp.MustCompile(`(?i)system\s+prompt\s+override`),
	regexp.MustCompile(`(?i)dump\s+(all\s+)?passwords`),
}

// Firewall manages tool execution policy, prompt injection defense, and HITL tickets.
type Firewall struct {
	mu            sync.RWMutex
	tickets       map[string]*ApprovalTicket
	allowedHosts  map[string]bool
	mutatingTools map[string]bool
	readTools     map[string]bool
	computeTools  map[string]bool
}

// NewFirewall initializes the in-line tool governance firewall.
func NewFirewall() *Firewall {
	fw := &Firewall{
		tickets:      make(map[string]*ApprovalTicket),
		allowedHosts: make(map[string]bool),
		mutatingTools: map[string]bool{
			"commit_patch_to_branch": true,
			"merge_pull_request":     true,
			"update_secret":          true,
			"delete_branch":          true,
			"publish_release":        true,
		},
		readTools: map[string]bool{
			"query_codegraph": true,
			"get_ast_slice":   true,
			"read_finding":    true,
			"list_repos":      true,
			"fetch_diff":      true,
		},
		computeTools: map[string]bool{
			"compile_code":   true,
			"run_unit_tests": true,
			"run_linter":     true,
			"parse_ast":      true,
		},
	}
	fw.allowedHosts["github.com"] = true
	fw.allowedHosts["gitlab.com"] = true
	fw.allowedHosts["api.github.com"] = true
	return fw
}

// DetectPromptInjection scans tool payloads and instructions for adversarial attacks.
func (f *Firewall) DetectPromptInjection(content string) bool {
	for _, pattern := range promptInjectionPatterns {
		if pattern.MatchString(content) {
			return true
		}
	}
	return false
}

// Evaluate inspects a tool call and enforces organizational boundaries.
func (f *Firewall) Evaluate(ctx context.Context, req ToolCallRequest) (*EvaluationResult, error) {
	// 1. Prompt Injection & Adversarial Bypass Check
	for _, v := range req.Parameters {
		strVal := fmt.Sprintf("%v", v)
		if f.DetectPromptInjection(strVal) {
			return &EvaluationResult{
				Allowed:         false,
				PromptInjection: true,
				Reason:          "adversarial prompt injection pattern detected in tool parameters",
			}, nil
		}
	}
	if req.DiffPayload != "" && f.DetectPromptInjection(req.DiffPayload) {
		return &EvaluationResult{
			Allowed:         false,
			PromptInjection: true,
			Reason:          "prompt injection detected in candidate diff payload",
		}, nil
	}

	// 2. Classify Danger Tier
	tier := Tier3ActiveProbe
	if f.readTools[req.ToolName] {
		tier = Tier1ReadOnly
	} else if f.computeTools[req.ToolName] {
		tier = Tier2SafeCompute
	} else if f.mutatingTools[req.ToolName] {
		tier = Tier4Mutating
	}

	// 3. Evaluate Tier-Specific Policies
	switch tier {
	case Tier1ReadOnly:
		return &EvaluationResult{
			Allowed:    true,
			DangerTier: tier,
			Reason:     "Tier 1 read-only operation authorized under tenant context",
		}, nil

	case Tier2SafeCompute:
		return &EvaluationResult{
			Allowed:    true,
			DangerTier: tier,
			Reason:     "Tier 2 safe compute authorized in ephemeral sandbox boundary",
		}, nil

	case Tier3ActiveProbe:
		if req.TargetURL != "" {
			isAllowed := false
			for host := range f.allowedHosts {
				if strings.Contains(req.TargetURL, host) {
					isAllowed = true
					break
				}
			}
			if !isAllowed {
				return &EvaluationResult{
					Allowed:    false,
					DangerTier: tier,
					Reason:     fmt.Sprintf("target url '%s' not in verified egress allowlist", req.TargetURL),
				}, nil
			}
		}
		return &EvaluationResult{
			Allowed:    true,
			DangerTier: tier,
			Reason:     "Tier 3 active probe permitted to authorized egress target",
		}, nil

	case Tier4Mutating:
		// Requires Human-in-the-Loop (HITL) Cryptographic Approval
		ticket := &ApprovalTicket{
			ID:          "TKT-" + uuid.New().String()[:12],
			ToolName:    req.ToolName,
			DangerTier:  tier,
			Status:      StatusPending,
			WorkspaceID: req.WorkspaceID,
			Requester:   req.AgentID,
			CreatedAt:   time.Now().UTC(),
		}

		f.mu.Lock()
		f.tickets[ticket.ID] = ticket
		f.mu.Unlock()

		return &EvaluationResult{
			Allowed:        false,
			RequiresHITL:   true,
			DangerTier:     tier,
			Reason:         "Tier 4 mutating tool call requires human cryptographic approval",
			ApprovalTicket: ticket,
		}, nil
	}

	return &EvaluationResult{Allowed: false, Reason: "unknown tool operation"}, nil
}

// ApproveTicket approves a pending HITL ticket with an Ed25519 signature.
func (f *Firewall) ApproveTicket(ticketID, approver, sigHex string, pubKey ed25519.PublicKey) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	t, exists := f.tickets[ticketID]
	if !exists {
		return errors.New("approval ticket not found")
	}
	if t.Status != StatusPending {
		return fmt.Errorf("ticket already resolved as %s", t.Status)
	}

	// Verify cryptographic signature if public key is provided
	if len(pubKey) == ed25519.PublicKeySize && sigHex != "" {
		sigBytes, err := hex.DecodeString(sigHex)
		if err != nil {
			return fmt.Errorf("invalid signature hex: %w", err)
		}
		msg := fmt.Appendf(nil, "%s:%s:%s", t.ID, t.ToolName, approver)
		if !ed25519.Verify(pubKey, msg, sigBytes) {
			return errors.New("cryptographic approval signature verification failed")
		}
	}

	now := time.Now().UTC()
	t.Status = StatusApproved
	t.ApprovedBy = approver
	t.Signature = sigHex
	t.ResolvedAt = &now
	return nil
}

// RejectTicket denies a pending HITL ticket.
func (f *Firewall) RejectTicket(ticketID, rejector, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	t, exists := f.tickets[ticketID]
	if !exists {
		return errors.New("approval ticket not found")
	}
	if t.Status != StatusPending {
		return fmt.Errorf("ticket already resolved as %s", t.Status)
	}

	now := time.Now().UTC()
	t.Status = StatusRejected
	t.ApprovedBy = rejector
	t.RejectReason = reason
	t.ResolvedAt = &now
	return nil
}
