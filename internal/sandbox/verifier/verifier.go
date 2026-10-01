package verifier

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/sandbox/contracts"
)

// VerificationStatus details the exact compiler/test outcome.
type VerificationStatus string

const (
	StatusPass                        VerificationStatus = "PASS"
	StatusRepairedAndPassed           VerificationStatus = "REPAIRED_AND_PASSED"
	StatusUnverifiedCompilationFailed VerificationStatus = "UNVERIFIED_COMPILATION_FAILED"
	StatusUnverifiedSandboxUnavailable VerificationStatus = "UNVERIFIED_SANDBOX_UNAVAILABLE"
	StatusUnverifiedTimeout           VerificationStatus = "UNVERIFIED_TIMEOUT"
)

// VerificationResult summarizes the execution and verification of a suggested code fix.
type VerificationResult struct {
	Verified     bool               `json:"verified"`
	Status       VerificationStatus `json:"status"`
	Iterations   int                `json:"iterations"`
	FinalCode    string             `json:"final_code,omitempty"`
	ErrorMessage string             `json:"error_message,omitempty"`
	Duration     time.Duration      `json:"duration"`
}

// RepairFunc is the signature for self-correction feedback loop (invoking the LLM to fix compile errors).
type RepairFunc func(ctx context.Context, filePath, brokenCode, compilerError string) (string, error)

// SuggestionVerifier validates patches in an isolated sandbox with automated 2-pass repair.
type SuggestionVerifier struct {
	maxIterations int
	timeout       time.Duration
}

// NewSuggestionVerifier initializes the verifier with standard limits (max 2 iterations, 15s timeout per TRD §3.2).
func NewSuggestionVerifier() *SuggestionVerifier {
	return &SuggestionVerifier{
		maxIterations: 2,
		timeout:       15 * time.Second,
	}
}

// VerifyAndRepair executes the build/test check and coordinates up to 2 repair passes if compilation fails.
func (v *SuggestionVerifier) VerifyAndRepair(
	ctx context.Context,
	sandbox contracts.SandboxInstance,
	filePath string,
	suggestedCode string,
	repairFn RepairFunc,
) (*VerificationResult, error) {
	start := time.Now()

	if sandbox == nil {
		return &VerificationResult{
			Verified: false,
			Status:   StatusUnverifiedSandboxUnavailable,
			Duration: time.Since(start),
		}, nil
	}

	// 1. Read original file to permit clean rollback
	originalBytes, readErr := sandbox.ReadFile(filePath)
	hasOriginal := readErr == nil

	defer func() {
		// Restore original state on completion or failure so sandbox remains clean
		if hasOriginal {
			_ = sandbox.WriteFile(filePath, originalBytes)
		}
	}()

	currentCode := suggestedCode
	var lastErr string

	for iteration := 1; iteration <= v.maxIterations; iteration++ {
		// Apply code into sandbox
		if err := sandbox.WriteFile(filePath, []byte(currentCode)); err != nil {
			return &VerificationResult{
				Verified:     false,
				Status:       StatusUnverifiedCompilationFailed,
				Iterations:   iteration,
				ErrorMessage: fmt.Sprintf("failed writing patch to sandbox: %v", err),
				Duration:     time.Since(start),
			}, nil
		}

		// Run language-specific compiler / linter / typecheck command
		checkCmd := v.resolveCheckCommand(filePath)
		runRes, runErr := sandbox.Run(ctx, checkCmd, nil, v.timeout)
		if runErr != nil || (runRes != nil && runRes.TimedOut) {
			return &VerificationResult{
				Verified:     false,
				Status:       StatusUnverifiedTimeout,
				Iterations:   iteration,
				ErrorMessage: "execution timeout reached",
				Duration:     time.Since(start),
			}, nil
		}

		if runRes != nil && runRes.ExitCode == 0 {
			// Success!
			status := StatusPass
			if iteration > 1 {
				status = StatusRepairedAndPassed
			}
			return &VerificationResult{
				Verified:   true,
				Status:     status,
				Iterations: iteration,
				FinalCode:  currentCode,
				Duration:   time.Since(start),
			}, nil
		}

		// Compile failed: capture diagnostics
		if runRes != nil {
			lastErr = strings.TrimSpace(runRes.Stderr)
			if lastErr == "" {
				lastErr = strings.TrimSpace(runRes.Stdout)
			}
		}

		// If we haven't exhausted attempts and a repair function is provided, self-correct!
		if iteration < v.maxIterations && repairFn != nil {
			repaired, err := repairFn(ctx, filePath, currentCode, lastErr)
			if err == nil && strings.TrimSpace(repaired) != "" && repaired != currentCode {
				currentCode = repaired
				continue
			}
		}
	}

	// Exhausted repair iterations without passing
	return &VerificationResult{
		Verified:     false,
		Status:       StatusUnverifiedCompilationFailed,
		Iterations:   v.maxIterations,
		ErrorMessage: lastErr,
		Duration:     time.Since(start),
	}, nil
}

func (v *SuggestionVerifier) resolveCheckCommand(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".go":
		dir := filepath.Dir(filePath)
		if dir == "" || dir == "." {
			dir = "./..."
		} else {
			dir = "./" + dir + "/..."
		}
		return fmt.Sprintf("go vet %s", dir)
	case ".ts", ".tsx":
		return "npx --no-install tsc --noEmit"
	case ".py":
		return fmt.Sprintf("python3 -m py_compile %s", filePath)
	case ".rs":
		return "cargo check"
	default:
		return "true"
	}
}
