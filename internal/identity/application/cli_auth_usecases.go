package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/identity/domain"
)

const (
	cliSessionTTLSeconds   = 10 * 60 // 10 minutes
	cliPollIntervalSeconds = 5
	userCodeAlphabet       = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // Excludes 0, O, 1, I to avoid confusion
)

var allowedLoopbackHosts = map[string]struct{}{
	"127.0.0.1": {},
	"localhost": {},
	"[::1]":     {},
	"::1":       {},
}

// InitiateCliLoginInput contains loopback options.
type InitiateCliLoginInput struct {
	Port      int    `json:"port"`
	UserAgent string `json:"user_agent,omitempty"`
}

// InitiateCliLoginResult contains the authorization challenge details.
type InitiateCliLoginResult struct {
	VerificationURI string `json:"verification_uri"`
	State           string `json:"state"`
	ExpiresIn       int    `json:"expires_in"`
}

// InitiateCliLoginUseCase begins local browser loopback authentication for the CLI.
type InitiateCliLoginUseCase struct {
	sessionRepo domain.CliAuthSessionRepository
	frontendURL string
}

func NewInitiateCliLoginUseCase(repo domain.CliAuthSessionRepository, frontendURL string) *InitiateCliLoginUseCase {
	fe := strings.TrimRight(frontendURL, "/")
	if fe == "" {
		fe = "https://app.scandrix.dev"
	}
	return &InitiateCliLoginUseCase{
		sessionRepo: repo,
		frontendURL: fe,
	}
}

func (uc *InitiateCliLoginUseCase) Execute(ctx context.Context, input InitiateCliLoginInput) (*InitiateCliLoginResult, error) {
	if input.Port < 1024 || input.Port > 65535 {
		return nil, errors.New("invalid loopback port: must be between 1024 and 65535")
	}

	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, fmt.Errorf("failed generating cryptographic state: %w", err)
	}
	state := hex.EncodeToString(stateBytes)

	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", input.Port)
	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(cliSessionTTLSeconds) * time.Second)

	var ua *string
	if input.UserAgent != "" {
		ua = &input.UserAgent
	}

	session := domain.CliAuthSession{
		UUID:        uuid.New(),
		State:       state,
		RedirectURI: &redirectURI,
		Mode:        domain.CliAuthModeLoopback,
		Status:      domain.CliAuthStatusPending,
		UserAgent:   ua,
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if _, err := uc.sessionRepo.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("failed creating CLI auth session: %w", err)
	}

	verificationURI := fmt.Sprintf("%s/cli/authorize?state=%s", uc.frontendURL, url.QueryEscape(state))

	return &InitiateCliLoginResult{
		VerificationURI: verificationURI,
		State:           state,
		ExpiresIn:       cliSessionTTLSeconds,
	}, nil
}

// IsLoopbackRedirect verifies that a URI targets a safe local loopback endpoint.
func IsLoopbackRedirect(uri string) bool {
	if uri == "" {
		return false
	}
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "http" {
		return false
	}
	hostname := parsed.Hostname()
	_, exists := allowedLoopbackHosts[hostname]
	return exists
}

// InitiateCliDeviceLoginInput contains device flow arguments.
type InitiateCliDeviceLoginInput struct {
	UserAgent string `json:"user_agent,omitempty"`
}

// InitiateCliDeviceLoginResult contains device code verification endpoints.
type InitiateCliDeviceLoginResult struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// InitiateCliDeviceLoginUseCase initiates headless/SSH device authorization.
type InitiateCliDeviceLoginUseCase struct {
	sessionRepo domain.CliAuthSessionRepository
	frontendURL string
}

func NewInitiateCliDeviceLoginUseCase(repo domain.CliAuthSessionRepository, frontendURL string) *InitiateCliDeviceLoginUseCase {
	fe := strings.TrimRight(frontendURL, "/")
	if fe == "" {
		fe = "https://app.scandrix.dev"
	}
	return &InitiateCliDeviceLoginUseCase{
		sessionRepo: repo,
		frontendURL: fe,
	}
}

func (uc *InitiateCliDeviceLoginUseCase) Execute(ctx context.Context, input InitiateCliDeviceLoginInput) (*InitiateCliDeviceLoginResult, error) {
	stateBytes := make([]byte, 32)
	_, _ = rand.Read(stateBytes)
	state := hex.EncodeToString(stateBytes)

	deviceBytes := make([]byte, 32)
	_, _ = rand.Read(deviceBytes)
	deviceCode := hex.EncodeToString(deviceBytes)

	userCode := generateUnbiasedUserCode()
	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(cliSessionTTLSeconds) * time.Second)

	var ua *string
	if input.UserAgent != "" {
		ua = &input.UserAgent
	}

	session := domain.CliAuthSession{
		UUID:       uuid.New(),
		State:      state,
		DeviceCode: &deviceCode,
		UserCode:   &userCode,
		Mode:       domain.CliAuthModeDevice,
		Status:     domain.CliAuthStatusPending,
		UserAgent:  ua,
		ExpiresAt:  expiresAt,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if _, err := uc.sessionRepo.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("failed creating device CLI session: %w", err)
	}

	verificationURI := fmt.Sprintf("%s/cli/authorize", uc.frontendURL)
	completeURI := fmt.Sprintf("%s?code=%s", verificationURI, url.QueryEscape(userCode))

	return &InitiateCliDeviceLoginResult{
		DeviceCode:              deviceCode,
		UserCode:                userCode,
		VerificationURI:         verificationURI,
		VerificationURIComplete: completeURI,
		ExpiresIn:               cliSessionTTLSeconds,
		Interval:                cliPollIntervalSeconds,
	}, nil
}

// generateUnbiasedUserCode produces an 8-character code using rejection sampling.
func generateUnbiasedUserCode() string {
	alphabetLen := len(userCodeAlphabet)
	maxUnbiased := (256 / alphabetLen) * alphabetLen

	out := make([]byte, 8)
	filled := 0
	buf := make([]byte, 16)

	for filled < 8 {
		_, _ = rand.Read(buf)
		for _, b := range buf {
			if int(b) >= maxUnbiased {
				continue
			}
			out[filled] = userCodeAlphabet[int(b)%alphabetLen]
			filled++
			if filled == 8 {
				break
			}
		}
	}

	return fmt.Sprintf("%s-%s", string(out[0:4]), string(out[4:8]))
}

// PollCliLoginInput contains lookup tokens.
type PollCliLoginInput struct {
	State      string `json:"state,omitempty"`
	DeviceCode string `json:"device_code,omitempty"`
}

// PollCliLoginResult contains the state or resolved credentials.
type PollCliLoginResult struct {
	Status       domain.CliAuthSessionStatus `json:"status"`
	AccessToken  *string                     `json:"access_token,omitempty"`
	RefreshToken *string                     `json:"refresh_token,omitempty"`
	UserEmail    *string                     `json:"user_email,omitempty"`
}

// PollCliLoginUseCase checks whether the user has confirmed authentication in their browser.
type PollCliLoginUseCase struct {
	sessionRepo domain.CliAuthSessionRepository
}

func NewPollCliLoginUseCase(repo domain.CliAuthSessionRepository) *PollCliLoginUseCase {
	return &PollCliLoginUseCase{sessionRepo: repo}
}

func (uc *PollCliLoginUseCase) Execute(ctx context.Context, input PollCliLoginInput) (*PollCliLoginResult, error) {
	if input.State == "" && input.DeviceCode == "" {
		return &PollCliLoginResult{Status: "not_found"}, nil
	}

	var session *domain.CliAuthSession
	var err error

	if input.State != "" {
		session, err = uc.sessionRepo.FindByState(ctx, input.State)
	} else {
		session, err = uc.sessionRepo.FindByDeviceCode(ctx, input.DeviceCode)
	}

	if err != nil || session == nil {
		return &PollCliLoginResult{Status: "not_found"}, nil
	}

	now := time.Now().UTC()
	if session.Status == domain.CliAuthStatusPending && now.After(session.ExpiresAt) {
		return &PollCliLoginResult{Status: domain.CliAuthStatusExpired}, nil
	}

	if session.Status != domain.CliAuthStatusCompleted {
		return &PollCliLoginResult{Status: session.Status}, nil
	}

	// One-shot retrieval: mark session consumed so tokens cannot be fetched a second time
	_ = uc.sessionRepo.MarkConsumed(ctx, session.UUID)

	return &PollCliLoginResult{
		Status:       domain.CliAuthStatusCompleted,
		AccessToken:  session.AccessToken,
		RefreshToken: session.RefreshToken,
		UserEmail:    session.UserEmail,
	}, nil
}

// CompleteCliLoginInput specifies the confirmed identity and session handle.
type CompleteCliLoginInput struct {
	State    string
	UserCode string
	User     domain.User
}

// CompleteCliLoginResult confirms the completion details for redirecting.
type CompleteCliLoginResult struct {
	RedirectURI *string
	State       string
	Mode        domain.CliAuthSessionMode
}

// CompleteCliLoginUseCase marks a CLI session authenticated by the user in the browser.
type CompleteCliLoginUseCase struct {
	sessionRepo  domain.CliAuthSessionRepository
	tokenService domain.TokenService
}

func NewCompleteCliLoginUseCase(
	repo domain.CliAuthSessionRepository,
	tokenService domain.TokenService,
) *CompleteCliLoginUseCase {
	return &CompleteCliLoginUseCase{
		sessionRepo:  repo,
		tokenService: tokenService,
	}
}

func (uc *CompleteCliLoginUseCase) Execute(ctx context.Context, input CompleteCliLoginInput) (*CompleteCliLoginResult, error) {
	if (input.State == "" && input.UserCode == "") || input.User.UUID == uuid.Nil {
		return nil, errors.New("state or userCode is required, plus an authenticated user")
	}

	var session *domain.CliAuthSession
	var err error

	if input.State != "" {
		session, err = uc.sessionRepo.FindByState(ctx, input.State)
	} else {
		session, err = uc.sessionRepo.FindByUserCode(ctx, input.UserCode)
	}

	if err != nil || session == nil {
		return nil, errors.New("CLI auth session not found")
	}

	if session.Status != domain.CliAuthStatusPending {
		return nil, fmt.Errorf("CLI auth session is %s", session.Status)
	}

	if time.Now().UTC().After(session.ExpiresAt) {
		return nil, errors.New("CLI auth session expired")
	}

	if session.Mode == domain.CliAuthModeLoopback {
		if session.RedirectURI == nil || !IsLoopbackRedirect(*session.RedirectURI) {
			return nil, errors.New("stored redirect URI is not a valid loopback address")
		}
	}

	var teamRole *domain.TeamMemberRole
	if len(input.User.TeamMembers) > 0 {
		teamRole = &input.User.TeamMembers[0].TeamRole
	}

	tokens, err := uc.tokenService.CreateTokens(input.User, teamRole)
	if err != nil {
		return nil, fmt.Errorf("failed creating login tokens: %w", err)
	}

	_, err = uc.sessionRepo.Complete(ctx, session.UUID, *tokens, input.User.UUID, input.User.Email)
	if err != nil {
		return nil, fmt.Errorf("failed completing CLI auth session: %w", err)
	}

	return &CompleteCliLoginResult{
		RedirectURI: session.RedirectURI,
		State:       session.State,
		Mode:        session.Mode,
	}, nil
}

// GetCliLoginInfoResult contains safe inspectable details for the web approval UI.
type GetCliLoginInfoResult struct {
	Found     bool                        `json:"found"`
	State     string                      `json:"state,omitempty"`
	Mode      domain.CliAuthSessionMode   `json:"mode,omitempty"`
	Status    domain.CliAuthSessionStatus `json:"status,omitempty"`
	UserAgent *string                     `json:"user_agent,omitempty"`
	ExpiresAt *time.Time                  `json:"expires_at,omitempty"`
}

// GetCliLoginInfoUseCase provides safe session metadata for the web approval screen without leaking tokens.
type GetCliLoginInfoUseCase struct {
	sessionRepo domain.CliAuthSessionRepository
}

func NewGetCliLoginInfoUseCase(repo domain.CliAuthSessionRepository) *GetCliLoginInfoUseCase {
	return &GetCliLoginInfoUseCase{sessionRepo: repo}
}

func (uc *GetCliLoginInfoUseCase) Execute(ctx context.Context, state, userCode string) (*GetCliLoginInfoResult, error) {
	if state == "" && userCode == "" {
		return &GetCliLoginInfoResult{Found: false}, nil
	}

	var session *domain.CliAuthSession
	var err error

	if state != "" {
		session, err = uc.sessionRepo.FindByState(ctx, state)
	} else {
		session, err = uc.sessionRepo.FindByUserCode(ctx, userCode)
	}

	if err != nil || session == nil {
		return &GetCliLoginInfoResult{Found: false}, nil
	}

	return &GetCliLoginInfoResult{
		Found:     true,
		State:     session.State,
		Mode:      session.Mode,
		Status:    session.Status,
		UserAgent: session.UserAgent,
		ExpiresAt: &session.ExpiresAt,
	}, nil
}
