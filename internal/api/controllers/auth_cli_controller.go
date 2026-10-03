package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	scandrixMiddleware "github.com/scandrix/backend/internal/api/middleware"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/auth/cliauth"
	"github.com/scandrix/backend/internal/auth/clitokens"
	"github.com/scandrix/backend/pkg/models"
)

func (c *AuthController) handleCreateCLIKey(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var req dtos.CreateAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, `{"error":"key name is required"}`, http.StatusBadRequest)
		return
	}

	if c.cliTokenService != nil {
		var ttl time.Duration
		if req.ExpiresAt != nil {
			ttl = time.Until(*req.ExpiresAt)
			if ttl < 0 {
				ttl = 0
			}
		}
		mintResp, err := c.cliTokenService.MintToken(r.Context(), clitokens.MintTokenRequest{
			WorkspaceID: wsID,
			Name:        req.Name,
			Scopes:      []clitokens.TokenScope{clitokens.ScopeReviewRead, clitokens.ScopeReviewWrite, clitokens.ScopeRulesSync},
			TTL:         ttl,
		})
		if err != nil {
			http.Error(w, `{"error":"failed generating key"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(dtos.APIKeyResponse{
			ID:        mintResp.Record.ID,
			Name:      mintResp.Record.Name,
			KeyPrefix: mintResp.Record.MaskedToken,
			PlainKey:  mintResp.Token,
			CreatedAt: mintResp.Record.CreatedAt,
			ExpiresAt: mintResp.Record.ExpiresAt,
		})
		return
	}

	plainKey, hashedKey, err := auth.GenerateAPIKey()
	if err != nil {
		http.Error(w, `{"error":"failed generating key"}`, http.StatusInternalServerError)
		return
	}

	keyID := uuid.New()
	prefix := plainKey[:14]

	// Live database storage adhering to Master Rule 2.1
	if c.repo != nil {
		_ = c.repo.SaveAPIKey(r.Context(), keyID, wsID, req.Name, hashedKey, prefix, req.ExpiresAt)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(dtos.APIKeyResponse{
		ID:        keyID,
		Name:      req.Name,
		KeyPrefix: prefix,
		PlainKey:  plainKey, // Shown only once to caller
		CreatedAt: time.Now().UTC(),
		ExpiresAt: req.ExpiresAt,
	})
}

// ============================================================================
// RFC 8628 CLI Device Authorization Flow
// ============================================================================

func (c *AuthController) handleDeviceInitiate(w http.ResponseWriter, r *http.Request) {
	if c.deviceFlow == nil {
		http.Error(w, `{"error":"CLI device flow not configured"}`, http.StatusServiceUnavailable)
		return
	}

	result, err := c.deviceFlow.InitiateDeviceLogin(r.Context(), r.UserAgent())
	if err != nil {
		slog.Error("Failed initiating device flow", "error", err)
		http.Error(w, `{"error":"failed initiating device flow"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (c *AuthController) handleDevicePoll(w http.ResponseWriter, r *http.Request) {
	if c.deviceFlow == nil {
		http.Error(w, `{"error":"CLI device flow not configured"}`, http.StatusServiceUnavailable)
		return
	}

	deviceCode := r.URL.Query().Get("device_code")
	if deviceCode == "" {
		http.Error(w, `{"error":"device_code query parameter is required"}`, http.StatusBadRequest)
		return
	}

	result, err := c.deviceFlow.PollDeviceLogin(r.Context(), deviceCode)
	if err != nil {
		if errors.Is(err, cliauth.ErrSlowDown) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":             "slow_down",
				"error_description": "client polling too frequently; poll interval increased",
				"interval":          result.Interval,
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"session not found or expired"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (c *AuthController) handleDeviceComplete(w http.ResponseWriter, r *http.Request) {
	if c.deviceFlow == nil {
		http.Error(w, `{"error":"CLI device flow not configured"}`, http.StatusServiceUnavailable)
		return
	}

	var req dtos.CompleteDeviceLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserCode == "" {
		http.Error(w, `{"error":"user_code is required"}`, http.StatusBadRequest)
		return
	}

	profile, ok := auth.AccountProfileFromContext(r.Context())
	if !ok || profile == nil {
		http.Error(w, `{"error":"authenticated user context required"}`, http.StatusUnauthorized)
		return
	}

	// Validate hardware device quota if device identifier header is provided
	devID := r.Header.Get("X-ScanDrix-Device-Id")
	if devID == "" {
		devID = r.Header.Get("X-Device-Id")
	}
	if devID != "" && c.deviceQuota != nil {
		if _, err := c.deviceQuota.ValidateOrRegisterDevice(r.Context(), profile.WorkspaceID, devID, r.UserAgent()); err != nil {
			if errors.Is(err, auth.ErrDeviceLimitReached) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"message": fmt.Sprintf("Device limit reached (%d). Remove an existing device or increase the limit.", c.deviceQuota.DeviceLimit()),
					"code":    "DEVICE_LIMIT_REACHED",
					"details": map[string]any{
						"limit":   c.deviceQuota.DeviceLimit(),
						"current": c.deviceQuota.DeviceLimit(),
					},
				})
				return
			}
		}
	}

	// Generate fresh tokens for the CLI session
	accessToken, refreshToken, err := c.authService.GenerateTokenPairWithEmail(profile.ID, profile.WorkspaceID, profile.Role, profile.Email)
	if err != nil {
		http.Error(w, `{"error":"failed generating tokens for device session"}`, http.StatusInternalServerError)
		return
	}

	// Client IP must come from the proxy-aware extractor, not from a raw header
	// read. A client sending its own X-Forwarded-For could otherwise pin its
	// address to any value it liked and walk straight past a per-IP rate limit
	// or lockout (AUDIT_REMEDIATION.md F-25).
	clientIP := scandrixMiddleware.ExtractClientIP(r)
	ctxWithIP := cliauth.WithClientIP(r.Context(), clientIP)

	if err := c.deviceFlow.CompleteDeviceLogin(ctxWithIP, req.UserCode, accessToken, refreshToken, profile); err != nil {
		slog.Warn("Failed completing device login", "user_code", req.UserCode, "error", err)
		if errors.Is(err, cliauth.ErrTooManyAttempts) {
			http.Error(w, `{"error":"too many invalid verification attempts; user code locked"}`, http.StatusTooManyRequests)
			return
		}
		http.Error(w, `{"error":"invalid or expired device login session"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"device login approved"}`))
}

// HandleCLILoginInit starts a local loopback authentication session (/cli/auth/login-init).
func (c *AuthController) HandleCLILoginInit(w http.ResponseWriter, r *http.Request) {
	c.handleLoopbackInitiate(w, r)
}

// handleLoopbackInitiate starts a local loopback authentication session (RFC 8252).
func (c *AuthController) handleLoopbackInitiate(w http.ResponseWriter, r *http.Request) {
	if c.loopbackMgr == nil {
		http.Error(w, `{"error":"CLI loopback flow not configured"}`, http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Port int `json:"port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Port <= 0 {
		http.Error(w, `{"error":"valid localhost port is required"}`, http.StatusBadRequest)
		return
	}

	result, err := c.loopbackMgr.InitLoopback(r.Context(), req.Port, r.UserAgent())
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// handleLoopbackPoll checks if the loopback authorization was completed.
func (c *AuthController) handleLoopbackPoll(w http.ResponseWriter, r *http.Request) {
	if c.loopbackMgr == nil {
		http.Error(w, `{"error":"CLI loopback flow not configured"}`, http.StatusServiceUnavailable)
		return
	}

	state := r.URL.Query().Get("state")
	if state == "" {
		http.Error(w, `{"error":"state query parameter is required"}`, http.StatusBadRequest)
		return
	}

	result, err := c.loopbackMgr.PollLoopback(r.Context(), state)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// handleGenerateHelpdeskToken mints a short-lived RS256 single sign-on token for the helpdesk iframe.
func (c *AuthController) handleGenerateHelpdeskToken(w http.ResponseWriter, r *http.Request) {
	if c.helpdeskSvc == nil {
		http.Error(w, `{"error":"helpdesk SSO service not configured"}`, http.StatusServiceUnavailable)
		return
	}

	profile, ok := auth.AccountProfileFromContext(r.Context())
	if !ok || profile == nil {
		http.Error(w, `{"error":"authenticated user required"}`, http.StatusUnauthorized)
		return
	}
	if profile.Email == "" && c.repo != nil {
		if u, err := c.repo.GetUserByID(r.Context(), profile.ID); err == nil && u != nil {
			profile.Email = u.Email
			profile.Role = models.UserRole(u.Role)
		}
	}

	token, err := c.helpdeskSvc.GenerateHelpdeskToken(profile)
	if err != nil {
		slog.Error("Failed generating helpdesk token", "error", err, "user_id", profile.ID)
		http.Error(w, `{"error":"failed generating helpdesk SSO token"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"token":      token,
		"token_type": "Bearer",
		"expires_in": 300,
	})
}

// HandleCLIAuthorizePage serves the clean minimalist confirmation UI with GitHub, GitLab, Bitbucket OAuth and email options.
func (c *AuthController) HandleCLIAuthorizePage(w http.ResponseWriter, r *http.Request) {
	rawCode := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("code")))
	cleanCode := strings.ReplaceAll(rawCode, "-", "")

	// Extract up to 8 characters for the boxes
	c1, c2, c3, c4, c5, c6, c7, c8 := "", "", "", "", "", "", "", ""
	chars := []rune(cleanCode)
	if len(chars) > 0 {
		c1 = string(chars[0])
	}
	if len(chars) > 1 {
		c2 = string(chars[1])
	}
	if len(chars) > 2 {
		c3 = string(chars[2])
	}
	if len(chars) > 3 {
		c4 = string(chars[3])
	}
	if len(chars) > 4 {
		c5 = string(chars[4])
	}
	if len(chars) > 5 {
		c6 = string(chars[5])
	}
	if len(chars) > 6 {
		c7 = string(chars[6])
	}
	if len(chars) > 7 {
		c8 = string(chars[7])
	}

	// Check if user has an active session cookie — always verify against DB
	loggedInEmail := ""
	if cookie, err := r.Cookie("scandrix_token"); err == nil && cookie.Value != "" && c.authService != nil {
		if claims, err := c.authService.VerifyToken(cookie.Value); err == nil && claims != nil {
			// Always verify the user actually exists in the database
			if c.repo != nil {
				if u, err := c.repo.GetUserByID(r.Context(), claims.UserID); err == nil && u != nil {
					loggedInEmail = u.Email
				}
			}
			// If DB is unavailable, do NOT fall back to JWT email claim — require fresh login
		}
	}

	isLoggedIn := loggedInEmail != ""

	htmlTemplate := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Enter Your code - ScanDrix</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Plus+Jakarta+Sans:wght@400;500;600;700;800&family=JetBrains+Mono:wght@600;700&display=swap" rel="stylesheet">
    <style>
        :root {
            --bg-page: #f8fafc;
            --card-bg: #ffffff;
            --text-main: #0f172a;
            --text-sub: #64748b;
            --box-bg: #f8fafc;
            --box-filled-bg: #1e293b;
            --box-filled-text: #ffffff;
            --box-border: #cbd5e1;
            --box-focus: #0f172a;
            --btn-bg: #0e381b;
            --btn-hover: #0a2914;
            --btn-text: #ffffff;
            --success-color: #15803d;
        }

        * {
            box-sizing: border-box;
            margin: 0;
            padding: 0;
        }

        body {
            font-family: 'Plus Jakarta Sans', system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background-color: var(--bg-page);
            color: var(--text-main);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 1.5rem;
        }

        .container {
            width: 100%;
            max-width: 440px;
            background: var(--card-bg);
            border-radius: 24px;
            padding: 2.5rem 2.25rem 2.25rem;
            box-shadow: 0 10px 30px -5px rgba(0, 0, 0, 0.05), 0 20px 25px -5px rgba(0, 0, 0, 0.03);
            text-align: center;
            display: flex;
            flex-direction: column;
            align-items: center;
            border: 1px solid rgba(226, 232, 240, 0.8);
        }

        .flower-icon {
            width: 54px;
            height: 54px;
            margin-bottom: 1.25rem;
            display: block;
        }

        h1 {
            font-size: 1.75rem;
            font-weight: 700;
            color: var(--text-main);
            margin-bottom: 0.4rem;
            letter-spacing: -0.02em;
        }

        p.description {
            font-size: 0.9rem;
            color: var(--text-sub);
            line-height: 1.45;
            max-width: 340px;
            margin-bottom: 1.5rem;
        }

        /* OAuth Provider Buttons */
        .oauth-stack {
            width: 100%;
            display: flex;
            flex-direction: column;
            gap: 0.65rem;
            margin-bottom: 1.25rem;
        }

        .btn-oauth {
            width: 100%;
            display: flex;
            align-items: center;
            justify-content: center;
            gap: 0.75rem;
            padding: 0.75rem 1rem;
            border-radius: 10px;
            font-size: 0.92rem;
            font-weight: 600;
            cursor: pointer;
            transition: all 0.15s ease;
            outline: none;
            text-decoration: none;
        }

        .btn-github {
            background: #0f172a;
            color: #ffffff;
            border: 1px solid #0f172a;
        }
        .btn-github:hover {
            background: #1e293b;
            border-color: #1e293b;
        }

        .btn-gitlab {
            background: #ffffff;
            color: #0f172a;
            border: 1.5px solid #e2e8f0;
        }
        .btn-gitlab:hover {
            background: #f8fafc;
            border-color: #cbd5e1;
        }

        .btn-bitbucket {
            background: #ffffff;
            color: #0f172a;
            border: 1.5px solid #e2e8f0;
        }
        .btn-bitbucket:hover {
            background: #f8fafc;
            border-color: #cbd5e1;
        }

        .oauth-divider {
            width: 100%;
            display: flex;
            align-items: center;
            margin: 0.5rem 0 1.25rem;
            color: #94a3b8;
            font-size: 0.8rem;
            font-weight: 600;
            text-transform: uppercase;
            letter-spacing: 0.05em;
        }

        .oauth-divider::before, .oauth-divider::after {
            content: "";
            flex: 1;
            border-bottom: 1px solid #e2e8f0;
        }

        .oauth-divider span {
            padding: 0 0.85rem;
        }

        /* User Profile Pill for already logged in users */
        .user-pill {
            display: flex;
            align-items: center;
            gap: 0.65rem;
            background: #f1f5f9;
            border: 1px solid #e2e8f0;
            padding: 0.6rem 1rem;
            border-radius: 999px;
            font-size: 0.88rem;
            font-weight: 600;
            color: var(--text-main);
            margin-bottom: 1.5rem;
        }

        .user-avatar {
            width: 24px;
            height: 24px;
            border-radius: 50%;
            background: #0e381b;
            color: white;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 0.75rem;
            font-weight: 700;
        }

        /* Segmented Auth Tabs */
        .auth-tabs {
            display: flex;
            width: 100%;
            background: #f1f5f9;
            padding: 3px;
            border-radius: 10px;
            margin-bottom: 1.25rem;
            gap: 4px;
        }

        .tab-btn {
            flex: 1;
            border: none;
            background: transparent;
            padding: 0.55rem 0.75rem;
            border-radius: 7px;
            font-size: 0.88rem;
            font-weight: 600;
            color: var(--text-sub);
            cursor: pointer;
            transition: all 0.15s ease;
        }

        .tab-btn.active {
            background: #ffffff;
            color: var(--text-main);
            box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
        }

        /* Form Inputs */
        .input-group {
            width: 100%;
            text-align: left;
            margin-bottom: 0.85rem;
        }

        .input-label {
            font-size: 0.8rem;
            font-weight: 600;
            color: var(--text-main);
            margin-bottom: 0.3rem;
            display: block;
        }

        .form-input {
            width: 100%;
            padding: 0.7rem 0.85rem;
            border-radius: 8px;
            border: 1.5px solid var(--box-border);
            background: #ffffff;
            font-family: inherit;
            font-size: 0.9rem;
            color: var(--text-main);
            outline: none;
            transition: border-color 0.15s ease;
        }

        .form-input:focus {
            border-color: var(--box-focus);
            box-shadow: 0 0 0 3px rgba(15, 23, 42, 0.08);
        }

        .code-row {
            display: flex;
            align-items: center;
            justify-content: center;
            gap: 0.45rem;
            margin: 1rem 0 1.25rem;
            width: 100%;
        }

        .divider {
            font-size: 1.25rem;
            font-weight: 700;
            color: #94a3b8;
            padding: 0 0.15rem;
            user-select: none;
        }

        .otp-input {
            width: 38px;
            height: 48px;
            font-family: 'JetBrains Mono', monospace;
            font-size: 1.25rem;
            font-weight: 700;
            text-align: center;
            border: 1.5px solid var(--box-border);
            border-radius: 6px;
            background: var(--box-bg);
            color: var(--text-main);
            outline: none;
            transition: all 0.15s ease;
            text-transform: uppercase;
        }

        .otp-input:focus {
            border-color: var(--box-focus);
            background: #ffffff;
            box-shadow: 0 0 0 3px rgba(15, 23, 42, 0.08);
        }

        .otp-input.filled {
            background: var(--box-filled-bg);
            color: var(--box-filled-text);
            border-color: var(--box-filled-bg);
        }

        .help-text {
            font-size: 0.82rem;
            color: var(--text-sub);
            margin-bottom: 1.5rem;
            line-height: 1.4;
        }

        .help-text a {
            color: var(--btn-bg);
            font-weight: 600;
            text-decoration: none;
            cursor: pointer;
        }

        .help-text a:hover {
            text-decoration: underline;
        }

        .btn-wrapper {
            width: 100%;
            padding: 4px;
            border: 1.5px solid var(--btn-bg);
            border-radius: 8px;
            background: transparent;
        }

        .btn-submit {
            width: 100%;
            background: var(--btn-bg);
            color: var(--btn-text);
            border: none;
            border-radius: 4px;
            padding: 0.85rem 1.5rem;
            font-size: 0.98rem;
            font-weight: 600;
            cursor: pointer;
            transition: background-color 0.15s ease, opacity 0.15s ease;
        }

        .btn-submit:hover {
            background: var(--btn-hover);
        }

        .btn-submit:disabled {
            opacity: 0.6;
            cursor: not-allowed;
        }

        .success-banner {
            display: none;
            margin-top: 1.5rem;
            padding: 1.25rem;
            border-radius: 8px;
            background: #f0fdf4;
            border: 1px solid #bbf7d0;
            color: var(--success-color);
            font-size: 0.95rem;
            font-weight: 600;
            width: 100%;
            line-height: 1.5;
        }
    </style>
</head>
<body>
    <div class="container">
        <!-- Elegant Geometric Rosette Logo -->
        <svg class="flower-icon" width="54" height="54" viewBox="0 0 100 100" fill="none" xmlns="http://www.w3.org/2000/svg">
            <g transform="translate(50,50)">
                <g transform="rotate(0)"><path d="M0 -15 C-6 -26 -10 -36 0 -44 C10 -36 6 -26 0 -15 Z" fill="#0e381b"/><circle cx="0" cy="-28" r="2.5" fill="#f8fafc"/></g>
                <g transform="rotate(45)"><path d="M0 -15 C-6 -26 -10 -36 0 -44 C10 -36 6 -26 0 -15 Z" fill="#881337"/><circle cx="0" cy="-28" r="2.5" fill="#f8fafc"/></g>
                <g transform="rotate(90)"><path d="M0 -15 C-6 -26 -10 -36 0 -44 C10 -36 6 -26 0 -15 Z" fill="#0e381b"/><circle cx="0" cy="-28" r="2.5" fill="#f8fafc"/></g>
                <g transform="rotate(135)"><path d="M0 -15 C-6 -26 -10 -36 0 -44 C10 -36 6 -26 0 -15 Z" fill="#881337"/><circle cx="0" cy="-28" r="2.5" fill="#f8fafc"/></g>
                <g transform="rotate(180)"><path d="M0 -15 C-6 -26 -10 -36 0 -44 C10 -36 6 -26 0 -15 Z" fill="#0e381b"/><circle cx="0" cy="-28" r="2.5" fill="#f8fafc"/></g>
                <g transform="rotate(225)"><path d="M0 -15 C-6 -26 -10 -36 0 -44 C10 -36 6 -26 0 -15 Z" fill="#881337"/><circle cx="0" cy="-28" r="2.5" fill="#f8fafc"/></g>
                <g transform="rotate(270)"><path d="M0 -15 C-6 -26 -10 -36 0 -44 C10 -36 6 -26 0 -15 Z" fill="#0e381b"/><circle cx="0" cy="-28" r="2.5" fill="#f8fafc"/></g>
                <g transform="rotate(315)"><path d="M0 -15 C-6 -26 -10 -36 0 -44 C10 -36 6 -26 0 -15 Z" fill="#881337"/><circle cx="0" cy="-28" r="2.5" fill="#f8fafc"/></g>
                <circle cx="0" cy="0" r="10" fill="#0e381b"/><circle cx="0" cy="0" r="6" fill="#e11d48"/><circle cx="0" cy="0" r="3" fill="#ffffff"/>
            </g>
        </svg>

        <h1>Enter Your code</h1>
        <p class="description" id="pageDesc">`

	if isLoggedIn {
		htmlTemplate += `Authorize this CLI terminal session for your account.</p>
        <div class="user-pill">
            <div class="user-avatar">✓</div>
            <span>Signed in as <strong>` + loggedInEmail + `</strong></span>
        </div>`
	} else {
		htmlTemplate += `Sign in or register to bind your identity to this CLI session.</p>

        <!-- Social OAuth Login Buttons -->
        <div class="oauth-stack" id="oauthSection">
            <button type="button" class="btn-oauth btn-github" onclick="handleSocialAuth('github')">
                <svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor">
                    <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z"/>
                </svg>
                <span>Continue with GitHub</span>
            </button>

            <button type="button" class="btn-oauth btn-gitlab" onclick="handleSocialAuth('gitlab')">
                <svg viewBox="0 0 24 24" width="20" height="20">
                    <path fill="#E24329" d="M23.955 13.587l-1.342-4.135-2.664-8.189c-.135-.423-.73-.423-.867 0L16.418 9.45H7.582L4.918 1.263c-.137-.423-.732-.423-.867 0L1.387 9.452.045 13.587c-.121.375.014.787.331 1.017l11.624 8.445 11.624-8.445c.317-.23.452-.642.331-1.017"/>
                    <path fill="#FC6D26" d="M12 23.049l-4.418-13.6h8.836L12 23.049z"/>
                    <path fill="#FCA326" d="M12 23.049l4.418-13.6h4.664L12 23.049z"/>
                    <path fill="#E24329" d="M21.082 9.45l1.531 4.137c.121.375-.014.787-.331 1.017L12 23.049l9.082-13.599z"/>
                    <path fill="#FCA326" d="M12 23.049L7.582 9.45H2.918L12 23.049z"/>
                    <path fill="#E24329" d="M2.918 9.45l-1.531 4.137c-.121.375.014.787.331 1.017L12 23.049 2.918 9.45z"/>
                </svg>
                <span>Continue with GitLab</span>
            </button>

            <button type="button" class="btn-oauth btn-bitbucket" onclick="handleSocialAuth('bitbucket')">
                <svg viewBox="0 0 24 24" width="20" height="20" fill="#0052CC">
                    <path d="M.75 2.5a.75.75 0 00-.742.846l3.35 20.088c.07.419.43.733.855.733h15.574a.75.75 0 00.742-.631L23.992 3.346a.75.75 0 00-.742-.846H.75zm13.882 13.914H9.368L8.14 8.76h7.72l-1.228 7.654z"/>
                </svg>
                <span>Continue with Bitbucket</span>
            </button>
        </div>

        <div class="oauth-divider" id="oauthDivider">
            <span>or with email</span>
        </div>`
	}

	htmlTemplate += `
        <!-- Real Error Banner -->
        <div id="errorBanner" style="display:none; width: 100%; color: #991b1b; background: #fef2f2; border: 1.5px solid #fecaca; border-radius: 10px; padding: 0.75rem 0.9rem; font-size: 0.84rem; margin-bottom: 1.25rem; text-align: left; line-height: 1.45;"></div>`

	if !isLoggedIn {
		htmlTemplate += `
        <!-- Segmented Auth Tabs -->
        <div class="auth-tabs" id="authTabs">
            <button type="button" class="tab-btn active" id="tabLogin" onclick="switchAuthTab('login')">Sign In</button>
            <button type="button" class="tab-btn" id="tabRegister" onclick="switchAuthTab('register')">Create Account</button>
        </div>`
	}

	htmlTemplate += `
        <form id="codeForm" onsubmit="handleSubmit(event)" style="width: 100%;">`

	if !isLoggedIn {
		htmlTemplate += `
            <div class="input-group">
                <label class="input-label" for="emailInput">Work Email</label>
                <input type="email" id="emailInput" class="form-input" placeholder="developer@company.com" required autocomplete="email" autofocus>
            </div>
            <div class="input-group">
                <label class="input-label" for="passwordInput">Password</label>
                <input type="password" id="passwordInput" class="form-input" placeholder="••••••••" required autocomplete="current-password">
            </div>`
	}

	htmlTemplate += `
            <div class="code-row">
                <input type="text" maxlength="1" class="otp-input" id="box1" value="{{C1}}" autocomplete="off">
                <input type="text" maxlength="1" class="otp-input" id="box2" value="{{C2}}" autocomplete="off">
                <input type="text" maxlength="1" class="otp-input" id="box3" value="{{C3}}" autocomplete="off">
                <input type="text" maxlength="1" class="otp-input" id="box4" value="{{C4}}" autocomplete="off">
                <span class="divider">-</span>
                <input type="text" maxlength="1" class="otp-input" id="box5" value="{{C5}}" autocomplete="off">
                <input type="text" maxlength="1" class="otp-input" id="box6" value="{{C6}}" autocomplete="off">
                <input type="text" maxlength="1" class="otp-input" id="box7" value="{{C7}}" autocomplete="off">
                <input type="text" maxlength="1" class="otp-input" id="box8" value="{{C8}}" autocomplete="off">
            </div>

            <p class="help-text" id="authHelpText">`

	if isLoggedIn {
		htmlTemplate += `Not your account? <a onclick="handleSwitchAccount()">Switch account.</a>`
	} else {
		htmlTemplate += `Don't have an account? <a onclick="switchAuthTab('register')">Create an account.</a>`
	}

	htmlTemplate += `</p>

            <div class="btn-wrapper">
                <button type="submit" class="btn-submit" id="submitBtn">`

	if isLoggedIn {
		htmlTemplate += `Authorize Terminal`
	} else {
		htmlTemplate += `Sign In & Authorize`
	}

	htmlTemplate += `</button>
            </div>
        </form>

        <div class="success-banner" id="successBanner">
            ✓ Terminal authorized successfully!<br>
            <span style="font-size: 0.85rem; font-weight: normal; color: #166534;" id="successDetail"></span>
        </div>
    </div>

    <script>
        const inputs = [
            document.getElementById('box1'),
            document.getElementById('box2'),
            document.getElementById('box3'),
            document.getElementById('box4'),
            document.getElementById('box5'),
            document.getElementById('box6'),
            document.getElementById('box7'),
            document.getElementById('box8')
        ];

        function getFullCode() {
            const codePart1 = inputs.slice(0, 4).map(i => i.value).join('');
            const codePart2 = inputs.slice(4, 8).map(i => i.value).join('');
            return (codePart1 + '-' + codePart2).toUpperCase();
        }

        function updateFilledStyles() {
            inputs.forEach(input => {
                if (input && input.value && input.value.trim() !== '') {
                    input.classList.add('filled');
                } else if (input) {
                    input.classList.remove('filled');
                }
            });
        }

        updateFilledStyles();

        inputs.forEach((input, index) => {
            if (!input) return;
            input.addEventListener('input', () => {
                input.value = input.value.toUpperCase();
                updateFilledStyles();
                if (input.value && index < inputs.length - 1) {
                    inputs[index + 1].focus();
                }
            });

            input.addEventListener('keydown', (e) => {
                if (e.key === 'Backspace' && !input.value && index > 0) {
                    inputs[index - 1].focus();
                }
            });

            input.addEventListener('paste', (e) => {
                e.preventDefault();
                const paste = (e.clipboardData || window.clipboardData).getData('text');
                const clean = paste.replace(/[^a-zA-Z0-9]/g, '').toUpperCase();
                for (let i = 0; i < inputs.length && i < clean.length; i++) {
                    inputs[i].value = clean[i];
                }
                updateFilledStyles();
                const nextIndex = Math.min(clean.length, inputs.length - 1);
                inputs[nextIndex].focus();
            });
        });

        function handleSwitchAccount() {
            document.cookie = 'scandrix_token=; Max-Age=0; path=/';
            location.reload();
        }

        function showError(msg) {
            const errBox = document.getElementById('errorBanner');
            if (errBox) {
                errBox.innerHTML = msg;
                errBox.style.display = 'block';
            } else {
                alert(msg);
            }
        }

        function hideError() {
            const errBox = document.getElementById('errorBanner');
            if (errBox) {
                errBox.style.display = 'none';
                errBox.innerHTML = '';
            }
        }

        async function handleSocialAuth(provider) {
            hideError();
            const fullCode = getFullCode();
            if (fullCode.length < 9 || fullCode.includes(' ')) {
                showError('Please ensure your 8-character confirmation code is entered.');
                return;
            }

            document.cookie = 'scandrix_cli_code=' + encodeURIComponent(fullCode) + '; path=/; max-age=600; SameSite=Lax';

            try {
                const res = await fetch('/api/v1/auth/oauth/' + provider + '/authorize?code=' + encodeURIComponent(fullCode));
                const data = await res.json().catch(() => ({}));
                if (res.ok && data.authorization_url) {
                    window.location.href = data.authorization_url;
                } else {
                    const provUpper = provider.toUpperCase();
                    const reason = (data && data.error) ? data.error : (provUpper + ' OAuth is not configured on this server');
                    showError('<strong>' + provUpper + ' OAuth Unavailable:</strong> ' + reason + '.<br><span style="font-size:0.8rem; color:#475569;">To enable ' + provUpper + ' login, set <code>' + provUpper + '_OAUTH_CLIENT_ID</code> and <code>' + provUpper + '_OAUTH_CLIENT_SECRET</code> in your <code>.env</code> file. Alternatively, sign in using your Work Email and Password below.</span>');
                }
            } catch (err) {
                showError('Connection error: ' + err.message);
            }
        }

        let currentAuthMode = 'login';
        function switchAuthTab(mode) {
            currentAuthMode = mode;
            hideError();
            const tabLogin = document.getElementById('tabLogin');
            const tabReg = document.getElementById('tabRegister');
            const submitBtn = document.getElementById('submitBtn');
            const helpText = document.getElementById('authHelpText');

            if (mode === 'register') {
                if (tabLogin) tabLogin.classList.remove('active');
                if (tabReg) tabReg.classList.add('active');
                if (submitBtn) submitBtn.textContent = 'Create Account & Authorize';
                if (helpText) helpText.innerHTML = 'Already have an account? <a onclick="switchAuthTab(\'login\')">Sign in instead.</a>';
            } else {
                if (tabReg) tabReg.classList.remove('active');
                if (tabLogin) tabLogin.classList.add('active');
                if (submitBtn) submitBtn.textContent = 'Sign In & Authorize';
                if (helpText) helpText.innerHTML = 'Don\'t have an account? <a onclick="switchAuthTab(\'register\')">Create an account.</a>';
            }
        }

        async function handleSubmit(e) {
            e.preventDefault();
            hideError();
            const fullCode = getFullCode();

            if (fullCode.length < 9 || fullCode.includes(' ')) {
                showError('Please enter all 8 characters of your authorization code.');
                return;
            }

            const emailInput = document.getElementById('emailInput');
            const passInput = document.getElementById('passwordInput');
            const emailVal = emailInput ? emailInput.value.trim() : '';
            const passVal = passInput ? passInput.value : '';

            const btn = document.getElementById('submitBtn');
            btn.disabled = true;
            btn.textContent = 'Verifying & Authorizing...';

            try {
                const res = await fetch('/api/v1/auth/cli/authorize/approve', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'X-Requested-With': 'XMLHttpRequest'
                    },
                    body: JSON.stringify({
                        user_code: fullCode,
                        email: emailVal,
                        password: passVal,
                        action: currentAuthMode
                    })
                });

                const data = await res.json().catch(() => ({}));

                if (res.ok) {
                    document.getElementById('codeForm').style.display = 'none';
                    const tabs = document.getElementById('authTabs');
                    if (tabs) tabs.style.display = 'none';
                    const oauthSec = document.getElementById('oauthSection');
                    if (oauthSec) oauthSec.style.display = 'none';
                    const divider = document.getElementById('oauthDivider');
                    if (divider) divider.style.display = 'none';
                    const detail = document.getElementById('successDetail');
                    if (data.email) {
                        detail.textContent = 'Logged in as ' + data.email + '. You can now return to your IDE terminal.';
                    } else {
                        detail.textContent = 'You can now return to your IDE terminal.';
                    }
                    document.getElementById('successBanner').style.display = 'block';
                } else {
                    showError('<strong>Authorization Failed:</strong> ' + (data.error || 'Invalid credentials or expired confirmation code.'));
                    btn.disabled = false;
                    btn.textContent = currentAuthMode === 'register' ? 'Create Account & Authorize' : 'Sign In & Authorize';
                }
            } catch (err) {
                showError('Network connection error: ' + err.message);
                btn.disabled = false;
                btn.textContent = currentAuthMode === 'register' ? 'Create Account & Authorize' : 'Sign In & Authorize';
            }
        }
    </script>
</body>
</html>`

	html := htmlTemplate
	html = strings.ReplaceAll(html, "{{C1}}", c1)
	html = strings.ReplaceAll(html, "{{C2}}", c2)
	html = strings.ReplaceAll(html, "{{C3}}", c3)
	html = strings.ReplaceAll(html, "{{C4}}", c4)
	html = strings.ReplaceAll(html, "{{C5}}", c5)
	html = strings.ReplaceAll(html, "{{C6}}", c6)
	html = strings.ReplaceAll(html, "{{C7}}", c7)
	html = strings.ReplaceAll(html, "{{C8}}", c8)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

// HandleCLIAuthorizeApprove completes the terminal authorization session with real credentials, OAuth, or active sessions.
// cliPublicRegistrationAllowed reports whether the CLI approve endpoint may
// create a brand-new account and workspace for an unauthenticated caller.
//
// This is closed by default and requires an explicit opt-in
// (ALLOW_PUBLIC_CLI_REGISTRATION). It is an unauthenticated endpoint that is deliberately
// exempt from the registration rate limiter, so leaving it open turns it into an
// unthrottled owner-account factory — see AUDIT_REMEDIATION.md F-13.
//
// AUTH_STRICT_INVITES_ONLY is a second, independent kill switch: it forces
// invite-only even if public registration was previously enabled, so an
// operator can lock a running instance down without redeploying.
func cliPublicRegistrationAllowed() bool {
	if strings.EqualFold(os.Getenv("AUTH_STRICT_INVITES_ONLY"), "true") {
		return false
	}
	return strings.EqualFold(os.Getenv("ALLOW_PUBLIC_CLI_REGISTRATION"), "true")
}

func (c *AuthController) HandleCLIAuthorizeApprove(w http.ResponseWriter, r *http.Request) {
	if c.deviceFlow == nil && c.loopbackMgr == nil {
		http.Error(w, `{"error":"CLI authorization service not configured"}`, http.StatusServiceUnavailable)
		return
	}

	var req struct {
		UserCode string `json:"user_code"`
		State    string `json:"state,omitempty"`
		Email    string `json:"email,omitempty"`
		Password string `json:"password,omitempty"`
		Action   string `json:"action,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || (req.UserCode == "" && req.State == "") {
		http.Error(w, `{"error":"user_code or state is required"}`, http.StatusBadRequest)
		return
	}

	var userID uuid.UUID
	var wsID uuid.UUID
	var userEmail string
	var userRole models.UserRole = models.RoleOwner

	if req.Email != "" && req.Password != "" {
		req.Email = strings.ToLower(strings.TrimSpace(req.Email))
		action := strings.ToLower(strings.TrimSpace(req.Action))
		if action == "" {
			action = "login"
		}

		if c.repo == nil {
			http.Error(w, `{"error":"authentication database is unavailable; cannot verify credentials"}`, http.StatusServiceUnavailable)
			return
		}

		if action == "login" {
			// Route through the shared credential path. This handler previously
			// called auth.VerifyPassword directly, which skipped the account
			// lockout, the per-account rate limiter, and failure accounting, so
			// an attacker could brute-force a locked account through this route
			// and obtain a full token pair (AUDIT_REMEDIATION.md F-03).
			clientIP := scandrixMiddleware.ExtractClientIP(r)
			res := c.AuthenticateCredentials(r.Context(), req.Email, req.Password, clientIP)
			if res.Status != CredentialAuthOK {
				writeCredentialAuthFailure(w, res)
				return
			}
			user := res.User

			if user.Status != "active" {
				http.Error(w, `{"error":"account is not active"}`, http.StatusForbidden)
				return
			}
			if user.OrganizationID == nil || *user.OrganizationID == uuid.Nil {
				http.Error(w, `{"error":"user is not assigned to an active workspace"}`, http.StatusForbidden)
				return
			}
			userID = user.UUID
			userEmail = user.Email
			wsID = *user.OrganizationID
			userRole = models.UserRole(user.Role)
			_ = c.repo.TouchAccountActivity(r.Context(), wsID, user.Email)
		} else if action == "register" {
			// Public self-registration is CLOSED by default.
			//
			// AUDIT_REMEDIATION.md F-13: both guards used to be opt-in, so an
			// unset environment fell through to creating an unauthenticated
			// account with role "owner" and its own workspace. This endpoint is
			// also exempt from registerRateLimitMiddleware, so that path was
			// effectively an unthrottled owner-account factory for anyone who
			// knew the URL.
			if !cliPublicRegistrationAllowed() {
				http.Error(w, `{"error":"public self-registration is disabled; please contact your administrator for an invite"}`, http.StatusForbidden)
				return
			}

			// Check if user already exists
			if existingUser, _ := c.repo.GetUserByEmail(r.Context(), req.Email); existingUser != nil {
				http.Error(w, `{"error":"an account with this email already exists, please switch to Sign In"}`, http.StatusConflict)
				return
			}

			// Register new user account and personal workspace in database
			// AUDIT_REMEDIATION.md F-21: was `len(password) < 8`, so the CLI
			// accepted `password1`. Shares the one policy with the web path so
			// the two cannot drift apart again.
			if err := auth.ValidatePassword(req.Password, req.Email); err != nil {
				msg := err.Error()
				if errors.Is(err, auth.ErrPasswordTooShort) {
					msg = fmt.Sprintf("password must be at least %d characters long", auth.MinPasswordLength)
				}
				http.Error(w, fmt.Sprintf(`{"error":%q}`, msg), http.StatusBadRequest)
				return
			}
			wsID = uuid.New()
			slug := "ws-" + wsID.String()[:8]
			ws := &models.Workspace{
				ID:        wsID,
				Slug:      slug,
				Name:      req.Email + "'s Workspace",
				Status:    models.TenantStatusActive,
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			}
			pwHash, err := auth.HashPassword(req.Password)
			if err != nil {
				http.Error(w, `{"error":"failed processing credentials"}`, http.StatusInternalServerError)
				return
			}
			createdUser, err := c.repo.CreateWorkspaceWithUser(r.Context(), ws, req.Email, pwHash, "owner", req.Email)
			if err != nil {
				slog.Error("Failed creating workspace with user in database", "error", err, "email", req.Email)
				http.Error(w, `{"error":"failed creating user account in database"}`, http.StatusInternalServerError)
				return
			}
			userID = createdUser.UUID
			userEmail = req.Email
			userRole = models.RoleOwner
		} else {
			http.Error(w, `{"error":"unsupported action; must be login or register"}`, http.StatusBadRequest)
			return
		}
	} else {
		// Check active session cookie
		cookie, err := r.Cookie("scandrix_token")
		if err == nil && cookie.Value != "" && c.authService != nil {
			claims, err := c.authService.VerifyToken(cookie.Value)
			if err == nil && claims != nil {
				userID = claims.UserID
				wsID = claims.WorkspaceID
				userRole = claims.Role
				// Always verify against database — do NOT trust JWT email claim alone
				if c.repo != nil {
					if u, err := c.repo.GetUserByID(r.Context(), userID); err == nil && u != nil {
						userEmail = u.Email
					}
				}
			}
		}

		if userEmail == "" {
			http.Error(w, `{"error":"Please provide your work email and password, or authenticate using an OAuth provider."}`, http.StatusUnauthorized)
			return
		}
	}

	// Validate hardware device quota if device identifier header is provided
	devID := r.Header.Get("X-ScanDrix-Device-Id")
	if devID == "" {
		devID = r.Header.Get("X-Device-Id")
	}
	if devID != "" && c.deviceQuota != nil {
		if _, err := c.deviceQuota.ValidateOrRegisterDevice(r.Context(), wsID, devID, r.UserAgent()); err != nil {
			if errors.Is(err, auth.ErrDeviceLimitReached) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"message": fmt.Sprintf("Device limit reached (%d). Remove an existing device or increase the limit.", c.deviceQuota.DeviceLimit()),
					"code":    "DEVICE_LIMIT_REACHED",
					"details": map[string]any{
						"limit":   c.deviceQuota.DeviceLimit(),
						"current": c.deviceQuota.DeviceLimit(),
					},
				})
				return
			}
		}
	}

	accessToken, refreshToken, err := c.authService.GenerateTokenPairWithEmail(userID, wsID, userRole, userEmail)
	if err != nil {
		http.Error(w, `{"error":"failed generating tokens"}`, http.StatusInternalServerError)
		return
	}

	if c.repo != nil {
		_ = c.repo.CreateRefreshToken(r.Context(), userID, refreshToken, time.Now().Add(30*24*time.Hour))
	}

	profile := &models.AccountProfile{
		ID:          userID,
		WorkspaceID: wsID,
		Email:       userEmail,
		Role:        userRole,
	}

	// Client IP must come from the proxy-aware extractor, not from a raw header
	// read. A client sending its own X-Forwarded-For could otherwise pin its
	// address to any value it liked and walk straight past a per-IP rate limit
	// or lockout (AUDIT_REMEDIATION.md F-25).
	clientIP := scandrixMiddleware.ExtractClientIP(r)
	ctxWithIP := cliauth.WithClientIP(r.Context(), clientIP)

	if req.State != "" && c.loopbackMgr != nil {
		if err := c.loopbackMgr.CompleteLoopback(ctxWithIP, req.State, accessToken, refreshToken, profile); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed completing loopback authorization: %s"}`, err.Error()), http.StatusBadRequest)
			return
		}
	}

	if req.UserCode != "" {
		if c.deviceFlow == nil {
			http.Error(w, `{"error":"CLI device flow not configured"}`, http.StatusServiceUnavailable)
			return
		}
		if err := c.deviceFlow.CompleteDeviceLogin(ctxWithIP, req.UserCode, accessToken, refreshToken, profile); err != nil {
			http.Error(w, `{"error":"invalid or expired user code"}`, http.StatusBadRequest)
			return
		}
	}

	// Set session cookie for persistent browser login
	setAuthCookie(w, r, "scandrix_token", accessToken, int(auth.DefaultAccessTokenTTL.Seconds()))

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "approved",
		"email":  userEmail,
	})
}

// ============================================================================
// Password Reset (HMAC-SHA256 bound to current password hash)
// ============================================================================

// ============================================================================
// CLI Authentication & Device Management Endpoints
// ============================================================================

// HandleValidateCLIKey implements the /cli/validate-key health-check, auth verification,
// and hardware device tracking endpoint.
func (c *AuthController) HandleValidateCLIKey(w http.ResponseWriter, r *http.Request) {
	teamKey := strings.TrimSpace(r.Header.Get("x-team-key"))
	if teamKey == "" {
		teamKey = strings.TrimSpace(r.Header.Get("X-Team-Key"))
	}

	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader == "" {
		authHeader = strings.TrimSpace(r.Header.Get("authorization"))
	}
	bearerToken := ""
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		bearerToken = strings.TrimSpace(authHeader[7:])
	}

	queryTeamID := r.URL.Query().Get("teamId")

	// Hardware device identification headers
	deviceID := strings.TrimSpace(r.Header.Get("x-scandrix-device-id"))
	if deviceID == "" {
		deviceID = strings.TrimSpace(r.Header.Get("x-device-id"))
	}

	deviceToken := strings.TrimSpace(r.Header.Get("x-scandrix-device-token"))
	if deviceToken == "" {
		deviceToken = strings.TrimSpace(r.Header.Get("x-device-token"))
	}

	userAgent := r.Header.Get("user-agent")
	if userAgent == "" {
		userAgent = r.Header.Get("User-Agent")
	}

	w.Header().Set("Content-Type", "application/json")

	buildInvalid := func(status int, errMsg, code string, details map[string]any) {
		w.WriteHeader(status)
		res := map[string]any{
			"valid":        false,
			"error":        errMsg,
			"team":         map[string]any{"id": nil, "name": ""},
			"organization": map[string]any{"id": nil, "name": ""},
			"user":         map[string]any{"email": "", "name": ""},
		}
		if code != "" {
			res["code"] = code
		}
		if details != nil {
			res["details"] = details
		}
		dataCopy := make(map[string]any, len(res))
		for k, v := range res {
			dataCopy[k] = v
		}
		res["data"] = dataCopy
		_ = json.NewEncoder(w).Encode(res)
	}

	var wsID uuid.UUID
	var teamID *uuid.UUID
	var wsName string
	var teamName string
	var userEmail string
	var userName string

	// Route 1: Team CLI key (X-Team-Key or Bearer scandrix_*)
	isCLIKey := teamKey != "" || strings.HasPrefix(bearerToken, "scandrix_")
	candidateKey := teamKey
	if candidateKey == "" && isCLIKey {
		candidateKey = bearerToken
	}

	if isCLIKey {
		if candidateKey == "" {
			buildInvalid(http.StatusUnauthorized, "Team API key required. Provide via X-Team-Key or Authorization: Bearer header.", "", nil)
			return
		}

		if c.repo != nil {
			verifiedWsID, profile, err := c.repo.VerifyCLIToken(r.Context(), candidateKey)
			if err != nil {
				// Also check in-memory TokenService fallback if available
				if c.cliTokenService != nil {
					rec, recErr := c.cliTokenService.ValidateToken(r.Context(), candidateKey, clitokens.ScopeReviewRead)
					if recErr == nil && rec != nil {
						wsID = rec.WorkspaceID
					} else {
						buildInvalid(http.StatusUnauthorized, "Invalid or revoked team API key", "", nil)
						return
					}
				} else {
					buildInvalid(http.StatusUnauthorized, "Invalid or revoked team API key", "", nil)
					return
				}
			} else {
				wsID = verifiedWsID
				if profile != nil {
					userEmail = profile.Email
					userName = profile.DisplayName
				}
			}
		} else if c.cliTokenService != nil {
			rec, err := c.cliTokenService.ValidateToken(r.Context(), candidateKey, clitokens.ScopeReviewRead)
			if err != nil {
				buildInvalid(http.StatusUnauthorized, "Invalid or revoked team API key", "", nil)
				return
			}
			wsID = rec.WorkspaceID
		} else {
			buildInvalid(http.StatusServiceUnavailable, "Authentication service unavailable", "", nil)
			return
		}

		if wsID == uuid.Nil {
			buildInvalid(http.StatusUnauthorized, "Invalid or incomplete team API key", "", nil)
			return
		}

		// Fetch workspace & team details
		if c.repo != nil {
			if ws, err := c.repo.GetWorkspaceByID(r.Context(), wsID); err == nil && ws != nil {
				wsName = ws.Name
			}
			if queryTeamID != "" {
				if parsedTeamID, err := uuid.Parse(queryTeamID); err == nil {
					if team, err := c.repo.GetTeamByID(r.Context(), wsID, parsedTeamID); err == nil && team != nil && team.WorkspaceID == wsID {
						teamID = &team.ID
						teamName = team.Name
					}
				}
			}
			if teamID == nil {
				if teams, err := c.repo.ListTeams(r.Context(), wsID); err == nil && len(teams) > 0 {
					teamID = &teams[0].ID
					teamName = teams[0].Name
				}
			}
		}
		if wsName == "" {
			wsName = "Workspace"
		}
		if teamName == "" {
			teamName = "Default Team"
		}

	} else if bearerToken != "" {
		// Route 2: JWT Bearer Token
		claims, err := c.authService.VerifyToken(bearerToken)
		if err != nil || claims == nil {
			buildInvalid(http.StatusUnauthorized, "Invalid or expired JWT token", "", nil)
			return
		}

		wsID = claims.WorkspaceID
		userEmail = claims.Email
		userName = claims.Email
		if c.repo != nil {
			user, err := c.repo.GetUserByID(r.Context(), claims.UserID)
			if err != nil || user == nil || user.Status != "active" {
				buildInvalid(http.StatusUnauthorized, "User account is inactive or removed", "", nil)
				return
			}
			if ws, err := c.repo.GetWorkspaceByID(r.Context(), wsID); err == nil && ws != nil {
				wsName = ws.Name
			}
			if queryTeamID != "" {
				if parsedTeamID, err := uuid.Parse(queryTeamID); err == nil {
					if team, err := c.repo.GetTeamByID(r.Context(), wsID, parsedTeamID); err == nil && team != nil && team.WorkspaceID == wsID {
						teamID = &team.ID
						teamName = team.Name
					}
				}
			}
			if teamID == nil {
				if teams, err := c.repo.ListTeams(r.Context(), wsID); err == nil && len(teams) > 0 {
					teamID = &teams[0].ID
					teamName = teams[0].Name
				}
			}
		}
		if wsName == "" {
			wsName = "Workspace"
		}
		if teamName == "" {
			teamName = "Default Team"
		}

	} else {
		buildInvalid(http.StatusUnauthorized, "Authentication required. Provide a team API key via X-Team-Key header, or a JWT via Authorization: Bearer header.", "", nil)
		return
	}

	// 2. Device Tracking & Quota Enforcement
	outDeviceToken := deviceToken
	if deviceID != "" && c.deviceQuota != nil && wsID != uuid.Nil {
		dev, err := c.deviceQuota.ValidateOrRegisterDevice(r.Context(), wsID, deviceID, userAgent)
		if err != nil {
			if errors.Is(err, auth.ErrDeviceLimitReached) {
				count, _ := c.deviceQuota.CountDevices(r.Context(), wsID)
				limit := c.deviceQuota.DeviceLimit()
				buildInvalid(http.StatusUnauthorized,
					fmt.Sprintf("Device limit reached (%d/%d). Remove an old device or contact your admin.", count, limit),
					"DEVICE_LIMIT_REACHED",
					map[string]any{
						"limit":         limit,
						"current":       count,
						"activeDevices": count,
					},
				)
				return
			}
			buildInvalid(http.StatusUnauthorized, err.Error(), "", nil)
			return
		}
		if dev != nil && dev.DeviceTokenHash != "" {
			outDeviceToken = dev.DeviceTokenHash
			w.Header().Set("x-scandrix-device-token", dev.DeviceTokenHash)
		}
	}

	var teamIDStr *string
	if teamID != nil {
		s := teamID.String()
		teamIDStr = &s
	}
	wsIDStr := wsID.String()

	payload := map[string]any{
		"valid":            true,
		"teamId":           teamIDStr,
		"organizationId":   wsIDStr,
		"teamName":         teamName,
		"organizationName": wsName,
		"team": map[string]any{
			"id":   teamIDStr,
			"name": teamName,
		},
		"organization": map[string]any{
			"id":   wsIDStr,
			"name": wsName,
		},
		"user": map[string]any{
			"email": userEmail,
			"name":  userName,
		},
		"email":     userEmail,
		"userEmail": userEmail,
	}
	if outDeviceToken != "" {
		payload["deviceToken"] = outDeviceToken
	}
	dataCopy := make(map[string]any, len(payload))
	for k, v := range payload {
		dataCopy[k] = v
	}
	payload["data"] = dataCopy

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(payload)
}

// HandleCLILoginInfo inspects a pending browser or device authorization session
// for confirmation UI rendering without leaking secrets or tokens.
func (c *AuthController) HandleCLILoginInfo(w http.ResponseWriter, r *http.Request) {
	// The CLI polls this to learn whether the browser approved its login.
	//
	// It previously accepted `user_code` on its own. A user code is short and
	// human-typable ("WDJB-MJHT"), so an unauthenticated caller could enumerate
	// codes, confirm which ones exist, and read back the session's user agent --
	// recon for the social-engineering path where an attacker talks a user into
	// approving a device they then control (AUDIT_REMEDIATION.md F-31).
	//
	// Only `state` and `device_code` are accepted now. Both are high-entropy
	// secrets the initiating CLI already holds, and both are what the
	// token-issuing /cli/auth/login-poll endpoint requires. A caller that presents
	// neither is told nothing about whether any session exists.
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	deviceCode := strings.TrimSpace(r.URL.Query().Get("device_code"))

	w.Header().Set("Content-Type", "application/json")

	if state == "" && deviceCode == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":    "state or device_code query parameter is required",
			"required": []string{"state", "device_code"},
		})
		return
	}

	// userAgent is not returned: it is a browser-fingerprinting aid and nothing
	// the CLI needs to complete the flow.
	if state != "" && c.loopbackMgr != nil {
		if sess, found := c.loopbackMgr.GetSessionByState(state); found && sess != nil {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"found":     true,
				"state":     sess.State,
				"mode":      "loopback",
				"status":    string(sess.Status),
				"expiresAt": sess.ExpiresAt,
			})
			return
		}
	}

	if deviceCode != "" && c.deviceFlow != nil {
		if sess, err := c.deviceFlow.GetSessionByDeviceCode(r.Context(), deviceCode); err == nil && sess != nil {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"found":     true,
				"state":     sess.State,
				"mode":      "device",
				"status":    string(sess.Status),
				"expiresAt": sess.ExpiresAt,
			})
			return
		}
	}

	// Same response whether the session is absent or simply not yet approved, so
	// this cannot be used to distinguish states.
	_ = json.NewEncoder(w).Encode(map[string]any{"found": false})
}

// HandleCLIDeviceInit starts an RFC 8628 device authorization session (/cli/auth/device-init).
func (c *AuthController) HandleCLIDeviceInit(w http.ResponseWriter, r *http.Request) {
	if c.deviceFlow == nil {
		http.Error(w, `{"error":"CLI device flow not configured"}`, http.StatusServiceUnavailable)
		return
	}

	result, err := c.deviceFlow.InitiateDeviceLogin(r.Context(), r.UserAgent())
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// HandleCLILoginPoll checks pending authorization status and returns tokens once (/cli/auth/login-poll).
func (c *AuthController) HandleCLILoginPoll(w http.ResponseWriter, r *http.Request) {
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	deviceCode := strings.TrimSpace(r.URL.Query().Get("device_code"))
	if deviceCode == "" {
		deviceCode = strings.TrimSpace(r.URL.Query().Get("deviceCode"))
	}

	w.Header().Set("Content-Type", "application/json")

	if state == "" && deviceCode == "" {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "not_found"})
		return
	}

	if state != "" && c.loopbackMgr != nil {
		res, err := c.loopbackMgr.PollLoopback(r.Context(), state)
		if err != nil {
			if errors.Is(err, cliauth.ErrLoopbackExpired) {
				_ = json.NewEncoder(w).Encode(map[string]any{"status": "expired"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "not_found"})
			return
		}
		if res.Status == cliauth.StatusCompleted {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":        "completed",
				"accessToken":   res.AccessToken,
				"refreshToken":  res.RefreshToken,
				"userEmail":     res.UserEmail,
				"access_token":  res.AccessToken,
				"refresh_token": res.RefreshToken,
				"user_email":    res.UserEmail,
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": string(res.Status)})
		return
	}

	if deviceCode != "" && c.deviceFlow != nil {
		res, err := c.deviceFlow.PollDeviceLogin(r.Context(), deviceCode)
		if err != nil {
			if errors.Is(err, cliauth.ErrSlowDown) {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error":             "slow_down",
					"error_description": "client polling too frequently; poll interval increased",
					"interval":          res.Interval,
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "not_found"})
			return
		}
		if res.Status == cliauth.StatusCompleted {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":        "completed",
				"accessToken":   res.AccessToken,
				"refreshToken":  res.RefreshToken,
				"userEmail":     res.UserEmail,
				"access_token":  res.AccessToken,
				"refresh_token": res.RefreshToken,
				"user_email":    res.UserEmail,
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": string(res.Status)})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{"status": "not_found"})
}

// HandleCLILoginComplete authorizes a pending CLI login session for the authenticated user (/cli/auth/login-complete).
func (c *AuthController) HandleCLILoginComplete(w http.ResponseWriter, r *http.Request) {
	profile, ok := auth.AccountProfileFromContext(r.Context())
	if !ok || profile == nil {
		http.Error(w, `{"error":"authenticated user required"}`, http.StatusUnauthorized)
		return
	}

	var req struct {
		State         string `json:"state"`
		UserCode      string `json:"userCode"`
		UserCodeSnake string `json:"user_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}
	if req.UserCode == "" && req.UserCodeSnake != "" {
		req.UserCode = req.UserCodeSnake
	}
	req.State = strings.TrimSpace(req.State)
	req.UserCode = strings.TrimSpace(req.UserCode)

	if req.State == "" && req.UserCode == "" {
		http.Error(w, `{"error":"state or userCode is required"}`, http.StatusBadRequest)
		return
	}

	// Validate hardware device quota if device identifier header is provided
	devID := r.Header.Get("X-ScanDrix-Device-Id")
	if devID == "" {
		devID = r.Header.Get("X-Device-Id")
	}
	if devID != "" && c.deviceQuota != nil {
		if _, err := c.deviceQuota.ValidateOrRegisterDevice(r.Context(), profile.WorkspaceID, devID, r.UserAgent()); err != nil {
			if errors.Is(err, auth.ErrDeviceLimitReached) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"message": fmt.Sprintf("Device limit reached (%d). Remove an existing device or increase the limit.", c.deviceQuota.DeviceLimit()),
					"code":    "DEVICE_LIMIT_REACHED",
					"details": map[string]any{
						"limit":   c.deviceQuota.DeviceLimit(),
						"current": c.deviceQuota.DeviceLimit(),
					},
				})
				return
			}
		}
	}

	accessToken, refreshToken, err := c.authService.GenerateTokenPairWithEmail(profile.ID, profile.WorkspaceID, profile.Role, profile.Email)
	if err != nil {
		http.Error(w, `{"error":"failed generating tokens"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if req.State != "" {
		if c.loopbackMgr == nil {
			http.Error(w, `{"error":"loopback manager not configured"}`, http.StatusServiceUnavailable)
			return
		}
		if err := c.loopbackMgr.CompleteLoopback(r.Context(), req.State, accessToken, refreshToken, profile); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
			return
		}
		redirectURI := ""
		if sess, found := c.loopbackMgr.GetSessionByState(req.State); found && sess != nil {
			redirectURI = sess.RedirectURI
		}
		res := map[string]any{
			"redirectUri": redirectURI,
			"state":       req.State,
			"mode":        "loopback",
		}
		dataCopy := make(map[string]any, len(res))
		for k, v := range res {
			dataCopy[k] = v
		}
		res["data"] = dataCopy
		_ = json.NewEncoder(w).Encode(res)
		return
	}

	if req.UserCode != "" {
		if c.deviceFlow == nil {
			http.Error(w, `{"error":"device flow manager not configured"}`, http.StatusServiceUnavailable)
			return
		}
		// Proxy-aware extraction; see F-25 -- a raw X-Forwarded-For read lets the
		// caller spoof the address that the rate limiter keys on.
		clientIP := scandrixMiddleware.ExtractClientIP(r)
		ctxWithIP := cliauth.WithClientIP(r.Context(), clientIP)
		if err := c.deviceFlow.CompleteDeviceLogin(ctxWithIP, req.UserCode, accessToken, refreshToken, profile); err != nil {
			if errors.Is(err, cliauth.ErrTooManyAttempts) {
				http.Error(w, `{"error":"too many invalid verification attempts; user code locked"}`, http.StatusTooManyRequests)
				return
			}
			http.Error(w, `{"error":"invalid or expired device login session"}`, http.StatusBadRequest)
			return
		}
		res := map[string]any{
			"redirectUri": nil,
			"state":       req.UserCode,
			"mode":        "device",
		}
		dataCopy := make(map[string]any, len(res))
		for k, v := range res {
			dataCopy[k] = v
		}
		res["data"] = dataCopy
		_ = json.NewEncoder(w).Encode(res)
		return
	}
}

// handleListCLIKeys retrieves all active CLI API keys for the caller's workspace.
func (c *AuthController) handleListCLIKeys(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	if c.repo == nil {
		http.Error(w, `{"error":"database repository unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	keys, err := c.repo.ListAPIKeys(r.Context(), wsID)
	if err != nil {
		http.Error(w, `{"error":"failed listing CLI keys"}`, http.StatusInternalServerError)
		return
	}

	res := make([]dtos.APIKeyResponse, 0, len(keys))
	for _, k := range keys {
		res = append(res, dtos.APIKeyResponse{
			ID:        k.ID,
			Name:      k.Name,
			KeyPrefix: k.KeyPrefix,
			CreatedAt: k.CreatedAt,
			ExpiresAt: k.ExpiresAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

// handleRevokeCLIKey revokes a CLI API key belonging to the caller's workspace.
func (c *AuthController) handleRevokeCLIKey(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	keyIDStr := chi.URLParam(r, "keyId")
	keyID, err := uuid.Parse(keyIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid key id"}`, http.StatusBadRequest)
		return
	}

	if c.repo == nil {
		http.Error(w, `{"error":"database repository unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	if err := c.repo.RevokeAPIKey(r.Context(), wsID, keyID); err != nil {
		http.Error(w, `{"error":"failed revoking key"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "key revoked successfully"})
}
