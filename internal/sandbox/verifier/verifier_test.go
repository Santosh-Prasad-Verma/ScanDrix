package verifier

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/sandbox/contracts"
	"github.com/stretchr/testify/assert"
)

type mockSandboxInstance struct {
	files       map[string][]byte
	runCommands []string
	exitCodes   []int
	stderrs     []string
	runIndex    int
}

func newMockSandbox() *mockSandboxInstance {
	return &mockSandboxInstance{
		files:       make(map[string][]byte),
		runCommands: make([]string, 0),
	}
}

func (m *mockSandboxInstance) GetID() uuid.UUID                                        { return uuid.New() }
func (m *mockSandboxInstance) GetTier() contracts.SandboxTier                          { return contracts.TierWorktree }
func (m *mockSandboxInstance) GetRepoDir() string                                      { return "/tmp/mock-repo" }
func (m *mockSandboxInstance) GetBaseBranch() string                                   { return "main" }
func (m *mockSandboxInstance) RemoteCommands() contracts.RemoteCommands                { return nil }
func (m *mockSandboxInstance) Cleanup(ctx context.Context) error                       { return nil }
func (m *mockSandboxInstance) Destroy(ctx context.Context) error                       { return nil }
func (m *mockSandboxInstance) RunCommand(ctx context.Context, req contracts.CommandRequest) (*contracts.CommandResult, error) {
	return nil, nil
}

func (m *mockSandboxInstance) ReadFile(relPath string) ([]byte, error) {
	if data, ok := m.files[relPath]; ok {
		return data, nil
	}
	return nil, errors.New("file not found")
}

func (m *mockSandboxInstance) WriteFile(relPath string, data []byte) error {
	m.files[relPath] = data
	return nil
}

func (m *mockSandboxInstance) Run(ctx context.Context, command string, envs map[string]string, timeout time.Duration) (*contracts.SandboxRunResult, error) {
	m.runCommands = append(m.runCommands, command)
	idx := m.runIndex
	m.runIndex++

	code := 0
	if idx < len(m.exitCodes) {
		code = m.exitCodes[idx]
	}
	stderr := ""
	if idx < len(m.stderrs) {
		stderr = m.stderrs[idx]
	}

	return &contracts.SandboxRunResult{
		ExitCode: code,
		Stderr:   stderr,
		Duration: 10 * time.Millisecond,
	}, nil
}

func TestVerifier_FirstPassSuccess(t *testing.T) {
	v := NewSuggestionVerifier()
	sb := newMockSandbox()
	sb.files["calc.go"] = []byte("package calc\nfunc Add(a, b int) int { return a + b }")
	sb.exitCodes = []int{0}

	res, err := v.VerifyAndRepair(context.Background(), sb, "calc.go", "package calc\nfunc Add(a, b int) int { return a + b }", nil)
	assert.NoError(t, err)
	assert.True(t, res.Verified)
	assert.Equal(t, StatusPass, res.Status)
	assert.Equal(t, 1, res.Iterations)
}

func TestVerifier_SecondPassSelfCorrectionSuccess(t *testing.T) {
	v := NewSuggestionVerifier()
	sb := newMockSandbox()
	sb.files["calc.go"] = []byte("package calc\n")
	// Iteration 1 fails with syntax error, Iteration 2 passes
	sb.exitCodes = []int{1, 0}
	sb.stderrs = []string{"undefined: x", ""}

	repairCallCount := 0
	repairFn := func(ctx context.Context, filePath, brokenCode, compilerError string) (string, error) {
		repairCallCount++
		assert.Equal(t, "calc.go", filePath)
		assert.Contains(t, compilerError, "undefined: x")
		return "package calc\nfunc Fixed() int { return 42 }", nil
	}

	res, err := v.VerifyAndRepair(context.Background(), sb, "calc.go", "package calc\nfunc Bad() { x = 1 }", repairFn)
	assert.NoError(t, err)
	assert.True(t, res.Verified)
	assert.Equal(t, StatusRepairedAndPassed, res.Status)
	assert.Equal(t, 2, res.Iterations)
	assert.Equal(t, 1, repairCallCount)
	assert.Contains(t, res.FinalCode, "Fixed()")
}

func TestVerifier_ExhaustedAttemptsReturnsUnverified(t *testing.T) {
	v := NewSuggestionVerifier()
	sb := newMockSandbox()
	sb.files["calc.go"] = []byte("package calc\n")
	// Both attempts fail
	sb.exitCodes = []int{1, 1}
	sb.stderrs = []string{"syntax error 1", "syntax error 2"}

	repairFn := func(ctx context.Context, filePath, brokenCode, compilerError string) (string, error) {
		return "package calc\nstill broken", nil
	}

	res, err := v.VerifyAndRepair(context.Background(), sb, "calc.go", "broken 1", repairFn)
	assert.NoError(t, err)
	assert.False(t, res.Verified)
	assert.Equal(t, StatusUnverifiedCompilationFailed, res.Status)
	assert.Equal(t, 2, res.Iterations)
	assert.Contains(t, res.ErrorMessage, "syntax error 2")
}

func TestVerifier_NilSandboxDegradesGracefully(t *testing.T) {
	v := NewSuggestionVerifier()
	res, err := v.VerifyAndRepair(context.Background(), nil, "calc.go", "code", nil)
	assert.NoError(t, err)
	assert.False(t, res.Verified)
	assert.Equal(t, StatusUnverifiedSandboxUnavailable, res.Status)
}

func TestVerifier_RollbackPreservesCleanSandbox(t *testing.T) {
	v := NewSuggestionVerifier()
	sb := newMockSandbox()
	original := "package original\n"
	sb.files["calc.go"] = []byte(original)
	sb.exitCodes = []int{1, 1}
	sb.stderrs = []string{"err1", "err2"}

	_, _ = v.VerifyAndRepair(context.Background(), sb, "calc.go", "broken", nil)

	// After execution, calc.go must be restored to original contents
	assert.Equal(t, original, string(sb.files["calc.go"]))
}
