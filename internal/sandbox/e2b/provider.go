package e2b

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	sandbox "github.com/scandrix/backend/internal/sandbox/contracts"
)

const (
	defaultE2BDomain     = "e2b.dev"
	defaultE2BTimeoutSec = 35 * 60 // 35 minutes ceiling
	repoDir              = "/home/user/repo"
)

// Config holds options for connecting to E2B and spinning up microVMs.
type Config struct {
	APIKey          string
	Domain          string
	Endpoint        string
	TemplateID      string
	TemplateGraphID string
	ProxyHost       string
	ProxyPort       string
	ProxyPassword   string
	ProxyMethod     string
	HTTPClient      *http.Client
	DefaultTTL      time.Duration
}

// E2BProvider implements sandbox.ISandboxProvider using cloud-isolated E2B microVMs.
type E2BProvider struct {
	cfg        Config
	httpClient *http.Client
}

// NewE2BProvider initializes an E2B cloud microVM sandbox provider.
func NewE2BProvider(cfg Config) *E2BProvider {
	if cfg.Domain == "" {
		cfg.Domain = defaultE2BDomain
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = fmt.Sprintf("https://api.%s", cfg.Domain)
	}
	if cfg.TemplateID == "" {
		cfg.TemplateID = "scandrix-sandbox"
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 60 * time.Second}
	}
	if cfg.DefaultTTL <= 0 {
		cfg.DefaultTTL = time.Duration(defaultE2BTimeoutSec) * time.Second
	}
	return &E2BProvider{
		cfg:        cfg,
		httpClient: cfg.HTTPClient,
	}
}

func (p *E2BProvider) IsAvailable() bool {
	return p.cfg.APIKey != ""
}

// CreateSandboxWithRepo provisions an E2B microVM and clones the target repo inside it.
func (p *E2BProvider) CreateSandboxWithRepo(ctx context.Context, params sandbox.CreateSandboxParams) (sandbox.SandboxInstance, error) {
	if !p.IsAvailable() {
		return nil, errors.New("E2B_API_KEY is not configured; cannot provision hardware-isolated microVM")
	}

	// 1. Choose template (graph template for graph build stages, default otherwise)
	templateID := p.cfg.TemplateID
	if params.SandboxMetadata != nil && (params.SandboxMetadata["stage"] == "graph-build" || params.SandboxMetadata["stage"] == "graph-incremental") {
		if p.cfg.TemplateGraphID != "" {
			templateID = p.cfg.TemplateGraphID
		}
	}

	// 2. Call E2B API: POST /sandboxes
	timeoutSec := int(p.cfg.DefaultTTL.Seconds())
	if timeoutSec <= 0 {
		timeoutSec = defaultE2BTimeoutSec
	}

	createPayload := map[string]any{
		"templateID": templateID,
		"timeout":    timeoutSec,
		"metadata":   params.SandboxMetadata,
		"autoPause":  true,
	}
	payloadBytes, _ := json.Marshal(createPayload)

	req, err := http.NewRequestWithContext(ctx, "POST", p.cfg.Endpoint+"/sandboxes", bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed creating sandbox request: %w", err)
	}
	req.Header.Set("X-API-Key", p.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed communicating with E2B API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("E2B API error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var sbxResp struct {
		SandboxID       string `json:"sandboxID"`
		Domain          string `json:"domain"`
		EnvdAccessToken string `json:"envdAccessToken"`
		EnvdVersion     string `json:"envdVersion"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&sbxResp); err != nil {
		return nil, fmt.Errorf("failed decoding E2B create response: %w", err)
	}

	sbxUUID, _ := uuid.Parse(sbxResp.SandboxID)
	if sbxUUID == uuid.Nil {
		sbxUUID = uuid.New()
	}

	inst := &MicroVMSandbox{
		id:              sbxUUID,
		e2bID:           sbxResp.SandboxID,
		apiKey:          p.cfg.APIKey,
		endpoint:        p.cfg.Endpoint,
		domain:          sbxResp.Domain,
		envdAccessToken: sbxResp.EnvdAccessToken,
		httpClient:      p.httpClient,
		repoDir:         repoDir,
		baseBranch:      params.BaseBranch,
		files:           make(map[string][]byte),
		createdAt:       time.Now().UTC(),
		expiresAt:       time.Now().UTC().Add(p.cfg.DefaultTTL),
	}

	cleanupOnFailure := true
	defer func() {
		if cleanupOnFailure {
			_ = inst.Cleanup(ctx)
		}
	}()

	// 3. Configure Shadowsocks proxy if set
	if p.cfg.ProxyHost != "" {
		if err := inst.setupProxy(ctx, p.cfg); err != nil {
			return nil, fmt.Errorf("failed configuring proxy tunnel in sandbox: %w", err)
		}
	}

	// 4. Clone repo inside sandbox
	if err := inst.cloneRepository(ctx, params); err != nil {
		return nil, fmt.Errorf("failed cloning repository in E2B sandbox: %w", err)
	}

	cleanupOnFailure = false
	return inst, nil
}

// MicroVMSandbox implements sandbox.SandboxInstance and sandbox.ISandbox for remote containerized execution.
type MicroVMSandbox struct {
	mu              sync.RWMutex
	id              uuid.UUID
	e2bID           string
	apiKey          string
	endpoint        string
	domain          string
	envdAccessToken string
	httpClient      *http.Client
	repoDir         string
	baseBranch      string
	files           map[string][]byte
	createdAt       time.Time
	expiresAt       time.Time
	destroyed       bool
}

// NewMicroVMSandbox initializes an isolated microVM sandbox lease (compatibility factory).
func NewMicroVMSandbox(apiKey, endpoint string, ttl time.Duration) *MicroVMSandbox {
	now := time.Now().UTC()
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	client := &http.Client{Timeout: 30 * time.Second}
	return &MicroVMSandbox{
		id:         uuid.New(),
		e2bID:      uuid.New().String(),
		apiKey:     apiKey,
		endpoint:   endpoint,
		httpClient: client,
		repoDir:    repoDir,
		files:      make(map[string][]byte),
		createdAt:  now,
		expiresAt:  now.Add(ttl),
		destroyed:  false,
	}
}

func (s *MicroVMSandbox) GetID() uuid.UUID {
	return s.id
}

func (s *MicroVMSandbox) GetTier() sandbox.SandboxTier {
	return sandbox.TierMicroVM
}

func (s *MicroVMSandbox) GetRepoDir() string {
	return s.repoDir
}

func (s *MicroVMSandbox) GetBaseBranch() string {
	return s.baseBranch
}

func (s *MicroVMSandbox) RemoteCommands() sandbox.RemoteCommands {
	return s
}

func (s *MicroVMSandbox) WriteFile(relPath string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.destroyed {
		return fmt.Errorf("sandbox %s already destroyed", s.id)
	}

	cleanRel := filepath.Clean(relPath)
	if filepath.IsAbs(cleanRel) || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) || cleanRel == ".." {
		return fmt.Errorf("security violation: path traversal attempt in sandbox file: %s", relPath)
	}

	s.files[cleanRel] = data
	return nil
}

func (s *MicroVMSandbox) ReadFile(relPath string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.destroyed {
		return nil, fmt.Errorf("sandbox %s already destroyed", s.id)
	}

	cleanRel := filepath.Clean(relPath)
	if filepath.IsAbs(cleanRel) || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) || cleanRel == ".." {
		return nil, fmt.Errorf("security violation: path traversal attempt in sandbox file: %s", relPath)
	}

	data, ok := s.files[cleanRel]
	if !ok {
		return nil, fmt.Errorf("file %s not found in sandbox", relPath)
	}
	return data, nil
}

func (s *MicroVMSandbox) Run(ctx context.Context, command string, envs map[string]string, timeout time.Duration) (*sandbox.SandboxRunResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.destroyed {
		return nil, fmt.Errorf("sandbox %s already destroyed", s.id)
	}

	start := time.Now()

	// 1. Remote execution if E2B endpoint & API key are configured
	if s.apiKey != "" && s.endpoint != "" {
		body, _ := json.Marshal(map[string]any{
			"cmd":     command,
			"env":     envs,
			"timeout": int(timeout.Seconds()),
			"cwd":     s.repoDir,
		})
		cmdURL := fmt.Sprintf("%s/sandboxes/%s/commands", s.endpoint, s.e2bID)
		httpReq, err := http.NewRequestWithContext(ctx, "POST", cmdURL, bytes.NewReader(body))
		if err == nil {
			httpReq.Header.Set("X-API-Key", s.apiKey)
			httpReq.Header.Set("Authorization", "Bearer "+s.apiKey)
			httpReq.Header.Set("Content-Type", "application/json")
			if s.envdAccessToken != "" {
				httpReq.Header.Set("X-Access-Token", s.envdAccessToken)
			}
			resp, err := s.httpClient.Do(httpReq)
			if err == nil {
				defer resp.Body.Close()
				var remoteResult struct {
					Stdout   string `json:"stdout"`
					Stderr   string `json:"stderr"`
					ExitCode int    `json:"exit_code"`
				}
				if resp.StatusCode >= 200 && resp.StatusCode < 300 && json.NewDecoder(resp.Body).Decode(&remoteResult) == nil {
					return &sandbox.SandboxRunResult{
						Stdout:   remoteResult.Stdout,
						Stderr:   remoteResult.Stderr,
						ExitCode: remoteResult.ExitCode,
						Duration: time.Since(start),
					}, nil
				}
			}
		}
	}

	// 2. Production Security Gate: Untrusted code must execute within an isolated microVM.
	// Local host execution is blocked in production unless ALLOW_UNSANDBOXED_COMMAND_EXECUTION=true.
	// Staging is included deliberately (AUDIT_REMEDIATION.md F-38). This gate
	// used to test only for "production", so a staging deployment with no E2B
	// credentials fell through to the development mock below, which returned
	// exit code 0. A staging run that never reached the sandbox therefore
	// reported a clean pass, and the release gate could not tell the difference.
	env := os.Getenv("ENVIRONMENT")
	appEnv := os.Getenv("APP_ENV")
	isProd := strings.EqualFold(env, "production") || strings.EqualFold(appEnv, "production")
	isStaging := strings.EqualFold(env, "staging") || strings.EqualFold(appEnv, "staging")
	requiresIsolation := isProd || isStaging

	allowHost := strings.EqualFold(os.Getenv("ALLOW_UNSANDBOXED_COMMAND_EXECUTION"), "true")
	if requiresIsolation && !allowHost {
		if s.apiKey == "" {
			return nil, errors.New("secure execution failed: E2B_API_KEY required; host command execution is prohibited in production")
		}
		return nil, errors.New("secure execution failed: remote microVM execution failed and host command execution is prohibited in production")
	}

	// Development mock fallback: support cat on staged files in testing
	if strings.HasPrefix(command, "cat ") {
		target := strings.TrimSpace(strings.TrimPrefix(command, "cat "))
		if data, ok := s.files[target]; ok {
			return &sandbox.SandboxRunResult{
				Stdout:   string(data),
				ExitCode: 0,
				Duration: time.Since(start),
			}, nil
		}
	}

	// Development mock. It deliberately does NOT report success.
	//
	// This returned exit code 0 with "dev-mock-exec: <cmd>" on stdout, so any
	// pipeline that gates on the exit code -- notably a vulnerability scan -- saw
	// a clean run for a command that was never executed. A mock must be
	// unmistakably a mock in both its output and its status, so a missing sandbox
	// fails the check instead of quietly passing it (F-38, related to F-39).
	//
	// Callers already treat a non-zero exit as failure
	// (agent_deliberation.go), so this correctly fails the pipeline rather than
	// reporting work that never happened.
	return &sandbox.SandboxRunResult{
		Stdout: "dev-mock-exec: command NOT executed (" + command + "). " +
			"No sandbox was available; treat this result as a failure, not a pass.",
		ExitCode: devMockExitCode,
		Duration: time.Since(start),
	}, nil
}

// devMockExitCode marks a command that was never actually executed. 127 is the
// conventional "command not found", which is an accurate description: the real
// binary was never invoked.
const devMockExitCode = 127

func (s *MicroVMSandbox) Cleanup(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.destroyed = true
	s.files = nil

	if s.apiKey != "" && s.endpoint != "" && s.e2bID != "" {
		delReq, err := http.NewRequestWithContext(ctx, "DELETE", fmt.Sprintf("%s/sandboxes/%s", s.endpoint, s.e2bID), nil)
		if err == nil {
			delReq.Header.Set("X-API-Key", s.apiKey)
			delReq.Header.Set("Authorization", "Bearer "+s.apiKey)
			resp, err := s.httpClient.Do(delReq)
			if err == nil {
				_ = resp.Body.Close()
			}
		}
	}

	return nil
}

// setupProxy configures Shadowsocks inside the microVM
func (s *MicroVMSandbox) setupProxy(ctx context.Context, cfg Config) error {
	port := cfg.ProxyPort
	if port == "" {
		port = "8388"
	}
	method := cfg.ProxyMethod
	if method == "" {
		method = "aes-256-gcm"
	}
	if cfg.ProxyPassword == "" {
		return errors.New("E2B_PROXY_PASSWORD is required when E2B_PROXY_HOST is configured")
	}

	cmd := fmt.Sprintf("ss-local -s %s -p %s -l 1080 -k %s -m %s -d start && git config --global http.proxy socks5://127.0.0.1:1080",
		cfg.ProxyHost, port, sandbox.ShSingleQuote(cfg.ProxyPassword), method)

	res, err := s.Run(ctx, cmd, nil, 10*time.Second)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("proxy startup failed (exit %d): %s", res.ExitCode, res.Stderr)
	}
	return nil
}

// cloneRepository clones the repository inside the microVM
func (s *MicroVMSandbox) cloneRepository(ctx context.Context, params sandbox.CreateSandboxParams) error {
	authHeader, err := sandbox.BuildAuthHeader(params.Platform, params.AuthToken, params.AuthUsername)
	if err != nil {
		return err
	}

	refspec := params.CheckoutSHA
	localRef := "cli-base"
	if refspec == "" {
		if params.PRNumber > 0 {
			refspec = sandbox.GetPRRefspec(params.Platform, params.PRNumber, params.CloneURL, params.Branch)
			localRef = "pr-head"
		} else {
			refspec = "refs/heads/" + params.Branch
			localRef = "cli-head"
		}
	}

	safeCloneURL := sandbox.ShSingleQuote(params.CloneURL)
	safeRefspec := sandbox.ShSingleQuote(refspec)
	safeLocalRef := sandbox.ShSingleQuote(localRef)

	fetchCmd := fmt.Sprintf("git fetch --depth=1 %s %s:%s", safeCloneURL, safeRefspec, safeLocalRef)
	envs := make(map[string]string)
	if authHeader != "" {
		fetchCmd = fmt.Sprintf("git -c http.extraHeader=\"$GIT_AUTH_HEADER\" fetch --depth=1 %s %s:%s", safeCloneURL, safeRefspec, safeLocalRef)
		envs["GIT_AUTH_HEADER"] = authHeader
	}

	fullInitCmd := strings.Join([]string{
		fmt.Sprintf("git init %s", s.repoDir),
		fmt.Sprintf("cd %s", s.repoDir),
		fetchCmd,
		fmt.Sprintf("git checkout %s", safeLocalRef),
		fmt.Sprintf("git remote add origin %s", safeCloneURL),
		"git remote set-url --push origin no-push-allowed",
	}, " && ")

	res, err := s.Run(ctx, fullInitCmd, envs, 5*time.Minute)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("git clone failed in sandbox (exit %d): %s", res.ExitCode, res.Stderr)
	}

	// Apply unifiedDiff if supplied
	if params.UnifiedDiff != "" {
		patchPath := "/tmp/scandrix-cli.patch"
		_ = s.WriteFile(patchPath, []byte(params.UnifiedDiff))
		applyCmd := fmt.Sprintf("cd %s && git config user.email scandrix-cli@scandrix.local && git config user.name 'ScanDrix CLI' && git apply --3way --whitespace=nowarn %s || git apply --whitespace=fix --reject %s || true",
			s.repoDir, patchPath, patchPath)
		_, _ = s.Run(ctx, applyCmd, nil, 30*time.Second)
	}

	// Fetch base branch if provided
	if params.BaseBranch != "" {
		safeBaseBranch := sandbox.ShSingleQuote(params.BaseBranch)
		baseFetchCmd := fmt.Sprintf("cd %s && git fetch --depth=1 %s refs/heads/%s:refs/remotes/origin/%s",
			s.repoDir, safeCloneURL, safeBaseBranch, safeBaseBranch)
		if authHeader != "" {
			baseFetchCmd = fmt.Sprintf("cd %s && git -c http.extraHeader=\"$GIT_AUTH_HEADER\" fetch --depth=1 %s refs/heads/%s:refs/remotes/origin/%s",
				s.repoDir, safeCloneURL, safeBaseBranch, safeBaseBranch)
		}
		_, _ = s.Run(ctx, baseFetchCmd, envs, 2*time.Minute)
	}

	return nil
}

// RemoteCommands methods
func (s *MicroVMSandbox) Grep(ctx context.Context, pattern, path, glob string) (string, error) {
	targetPath, err := sandbox.ResolveRepoPath(s.repoDir, path)
	if err != nil {
		return "", err
	}
	safePattern := strings.ReplaceAll(pattern, "'", "'\\''")
	safePath := strings.ReplaceAll(targetPath, "'", "'\\''")
	globArg := ""
	if glob != "" {
		globArg = fmt.Sprintf(" --glob '%s'", strings.ReplaceAll(glob, "'", "'\\''"))
	}
	cmd := fmt.Sprintf("cd %s && rg --no-heading -n '%s' '%s'%s", s.repoDir, safePattern, safePath, globArg)
	res, err := s.Run(ctx, cmd, nil, 30*time.Second)
	if err != nil {
		return "", err
	}
	if res.ExitCode >= 2 && res.Stderr != "" {
		return "Error: " + res.Stderr, nil
	}
	if res.Stdout == "" {
		return "No matches found.", nil
	}
	return res.Stdout, nil
}

func (s *MicroVMSandbox) Read(ctx context.Context, path string, start, end int) (string, error) {
	targetPath, err := sandbox.ResolveRepoPath(s.repoDir, path)
	if err != nil {
		return "", err
	}
	safePath := strings.ReplaceAll(targetPath, "'", "'\\''")
	cmd := fmt.Sprintf("cat '%s'", safePath)
	if !(start == 0 && end == 0) {
		startLine := start
		if startLine < 1 {
			startLine = 1
		}
		cmd = fmt.Sprintf("sed -n '%d,%dp' '%s'", startLine, end, safePath)
	}
	res, err := s.Run(ctx, cmd, nil, 10*time.Second)
	if err != nil {
		return "", err
	}
	return res.Stdout, nil
}

func (s *MicroVMSandbox) ListDir(ctx context.Context, path string, maxDepth int) (string, error) {
	targetPath, err := sandbox.ResolveRepoPath(s.repoDir, path)
	if err != nil {
		return "", err
	}
	if maxDepth <= 0 {
		maxDepth = 2
	}
	safePath := strings.ReplaceAll(targetPath, "'", "'\\''")
	cmd := fmt.Sprintf("find '%s' -maxdepth %d -type f", safePath, maxDepth)
	res, err := s.Run(ctx, cmd, nil, 30*time.Second)
	if err != nil {
		return "", err
	}
	return res.Stdout, nil
}

func (s *MicroVMSandbox) Exec(ctx context.Context, command string) (*sandbox.SandboxRunResult, error) {
	cmd := fmt.Sprintf("cd %s && %s", s.repoDir, command)
	return s.Run(ctx, cmd, nil, 30*time.Second)
}

// Compatibility ISandbox implementation
func (s *MicroVMSandbox) RunCommand(ctx context.Context, req sandbox.CommandRequest) (*sandbox.CommandResult, error) {
	cmdStr := req.Command
	if len(req.Args) > 0 {
		cmdStr += " " + strings.Join(req.Args, " ")
	}
	res, err := s.Run(ctx, cmdStr, req.Env, time.Duration(req.TimeoutSec)*time.Second)
	if err != nil {
		return nil, err
	}
	return &sandbox.CommandResult{
		Stdout:   res.Stdout,
		Stderr:   res.Stderr,
		ExitCode: res.ExitCode,
		Duration: res.Duration,
		TimedOut: res.TimedOut,
	}, nil
}

func (s *MicroVMSandbox) Destroy(ctx context.Context) error {
	return s.Cleanup(ctx)
}
