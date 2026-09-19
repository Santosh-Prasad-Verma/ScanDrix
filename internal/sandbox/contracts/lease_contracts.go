package contracts

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var uuidRegex = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// SandboxInvalidatedError is returned when a lease was invalidated (PR was closed or force-pushed)
// while the sandbox was being acquired or created. This is a SUPERSEDE, not a failure.
type SandboxInvalidatedError struct {
	PrKey     string
	MidCreate bool
}

func (e *SandboxInvalidatedError) Error() string {
	midCreateStr := ""
	if e.MidCreate {
		midCreateStr = " mid-create"
	}
	return fmt.Sprintf("SandboxLeaseManager: sandbox invalidated%s for prKey=%q", midCreateStr, e.PrKey)
}

// SandboxCreateTimeoutError is returned when polling for a CREATING sandbox exceeds MAX_POLL_WAIT.
type SandboxCreateTimeoutError struct {
	PrKey string
}

func (e *SandboxCreateTimeoutError) Error() string {
	return fmt.Sprintf("SandboxLeaseManager: timed out waiting for sandbox to become READY for prKey=%q", e.PrKey)
}

// SandboxStaleConnectionError is returned internally when connecting to an existing sandbox fails
// because the sandbox no longer exists (idle-kill, reaper, or external termination).
type SandboxStaleConnectionError struct {
	PrKey     string
	SandboxID string
}

func (e *SandboxStaleConnectionError) Error() string {
	return fmt.Sprintf("SandboxLeaseManager: stale sandbox connection — sandboxId=%q no longer exists for prKey=%q; lease cleaned, retry expected", e.SandboxID, e.PrKey)
}

// AcquireResult encapsulates the outcome of a sandbox lease acquisition.
type AcquireResult struct {
	Sandbox    SandboxInstance `json:"-"`
	LeaseID    string          `json:"lease_id"`
	SandboxID  string          `json:"sandbox_id"`
	WasCreated bool            `json:"was_created"` // True if cold-created, false if warm-resumed
}

// ReleaseOptions contains optional parameters for releasing a sandbox lease.
type ReleaseOptions struct {
	IdleTimeout time.Duration
}

// ISandboxLeaseManager coordinates shared sandbox microVMs and worktrees across reviews and chat flows.
type ISandboxLeaseManager interface {
	// Acquire acquires a lease on the sandbox for the given prKey. If no sandbox exists yet,
	// the manager cold-creates one using cloneParams. Subsequent acquires return the existing sandbox.
	Acquire(ctx context.Context, prKey, consumer string, leaseTTL time.Duration, cloneParams *CreateSandboxParams) (*AcquireResult, error)

	// Release releases a held lease. When the last lease is released, an idle-kill is scheduled or local directory is cleaned.
	Release(ctx context.Context, leaseID string, opts *ReleaseOptions) error

	// Invalidate invalidates a sandbox for the given prKey on PR close or force-push.
	Invalidate(ctx context.Context, prKey string) error
}

// BuildPrKey builds the canonical prKey "{organizationId}:{repositoryId}:{prNumber}".
// SECURITY: validates each segment to prevent malformed keys that could collide across tenants.
func BuildPrKey(organizationID, repositoryID string, prNumber any) (string, error) {
	if !uuidRegex.MatchString(organizationID) {
		return "", fmt.Errorf("buildPrKey: organizationId must be a UUID, got %q", organizationID)
	}

	repoStr := strings.TrimSpace(repositoryID)
	if repoStr == "" || strings.Contains(repoStr, ":") {
		return "", fmt.Errorf("buildPrKey: repositoryId must be non-empty and contain no \":\" — got %q", repositoryID)
	}

	prStr := strings.TrimSpace(fmt.Sprintf("%v", prNumber))
	if prStr == "" || strings.Contains(prStr, ":") {
		return "", fmt.Errorf("buildPrKey: prNumber must be non-empty and contain no \":\" — got %v", prNumber)
	}

	return fmt.Sprintf("%s:%s:%s", organizationID, repoStr, prStr), nil
}

// AssertValidPrKey validates a prKey passed by an internal caller as defense-in-depth.
func AssertValidPrKey(prKey string) error {
	parts := strings.Split(prKey, ":")
	if len(parts) != 3 && len(parts) != 4 {
		return fmt.Errorf("invalid prKey shape: %q (expected 3 or 4 \":\"-separated segments)", prKey)
	}

	// Accept literal 'trial' for demo/anonymous flows, otherwise UUID
	if parts[0] != "trial" && !uuidRegex.MatchString(parts[0]) {
		return fmt.Errorf("invalid prKey: first segment must be a UUID organizationId or 'trial', got %q", parts[0])
	}

	if strings.TrimSpace(parts[1]) == "" {
		return fmt.Errorf("invalid prKey: missing repositoryId in %q", prKey)
	}

	if strings.TrimSpace(parts[2]) == "" {
		return fmt.Errorf("invalid prKey: missing prNumber/marker in %q", prKey)
	}

	return nil
}

// DecomposedPrKey holds the parsed components of a canonical prKey.
type DecomposedPrKey struct {
	OrganizationID string
	RepositoryID   string
	PRNumber       string
}

// DecomposePrKey parses a validated prKey into its constituent fields.
func DecomposePrKey(prKey string) (*DecomposedPrKey, error) {
	if err := AssertValidPrKey(prKey); err != nil {
		return nil, fmt.Errorf("decomposePrKey: %w", err)
	}

	parts := strings.Split(prKey, ":")
	if len(parts) == 3 {
		return &DecomposedPrKey{
			OrganizationID: parts[0],
			RepositoryID:   parts[1],
			PRNumber:       parts[2],
		}, nil
	}

	// 4 parts: CLI mode <orgId>:<repoId>:cli:<branch>
	return &DecomposedPrKey{
		OrganizationID: parts[0],
		RepositoryID:   parts[1],
		PRNumber:       parts[3],
	}, nil
}
