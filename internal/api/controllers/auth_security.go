package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	scandrixMiddleware "github.com/scandrix/backend/internal/api/middleware"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/cache/limiter"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/telemetry"
)

// isNilInterface safely returns true if an interface is nil or points to a nil concrete value.
// In Go, assigning a typed nil pointer (e.g. (*database.Repository)(nil)) to an interface
// results in an interface where iface != nil. This helper unwraps and detects nil pointers.
func isNilInterface(i any) bool {
	if i == nil {
		return true
	}
	v := reflect.ValueOf(i)
	return v.Kind() == reflect.Ptr && v.IsNil()
}

// isSecureRequest reports whether authentication cookies should carry the
// Secure attribute.
//
// AUDIT_REMEDIATION.md F-19: this used to fail OPEN. It returned false for any
// request without TLS, without `X-Forwarded-Proto: https`, and outside a
// "production" environment name -- so a staging or misconfigured deployment
// silently issued session cookies without Secure, which browsers then send
// over plaintext HTTP. It also consulted X-Forwarded-Proto, a header any client
// can set, as an input to a security decision.
//
// The decision is now: Secure unless an operator has explicitly opted out.
// Relaxation requires COOKIE_SECURE=false, which is a deliberate setting rather
// than an accident of environment naming, and it is refused outright in
// production so a misconfigured prod task cannot downgrade itself.
func isSecureRequest(_ *http.Request) bool {
	if strings.EqualFold(os.Getenv("COOKIE_SECURE"), "false") {
		if isProductionEnv() {
			// Fail closed: never downgrade cookies in production.
			return true
		}
		return false
	}
	return true
}

// isProductionEnv reports whether the process is running as production.
// Checked from several environment variables because the deployment sets more
// than one of them, and getting this wrong must not weaken cookie security.
func isProductionEnv() bool {
	for _, key := range []string{"ENVIRONMENT", "APP_ENV", "SCANDRIX_ENV", "GO_ENV"} {
		if strings.EqualFold(os.Getenv(key), "production") {
			return true
		}
	}
	return false
}

// setAuthCookie writes an authentication session cookie with production security flags.
func setAuthCookie(w http.ResponseWriter, r *http.Request, name, value string, maxAge int) {
	secure := isSecureRequest(r)
	cookieName := name
	// __Host- prefix requires Secure, so it is only valid when we actually set
	// the attribute. It also mandates Path=/ and forbids Domain, which is what
	// makes it resistant to subdomain takeover.
	if secure && isProductionEnv() && strings.HasPrefix(name, "scandrix_") {
		cookieName = "__Host-" + name
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// getAuthCookie reads a cookie set by setAuthCookie.
//
// In production setAuthCookie renames cookies to the __Host- form, so reading
// the bare name silently missed them. Callers that used r.Cookie(name)
// directly therefore stopped working once deployed. This helper tries the
// prefixed name first and falls back to the bare one.
func getAuthCookie(r *http.Request, name string) string {
	if c, err := r.Cookie("__Host-" + name); err == nil && c.Value != "" {
		return c.Value
	}
	if c, err := r.Cookie(name); err == nil {
		return c.Value
	}
	return ""
}

type failedLoginEntry struct {
	attempts    int
	expiresAt   time.Time
	lockedUntil time.Time
}

// AuthMiddleware returns the authentication middleware that validates the
// session token and populates the account profile in the request context.
//
// It exists so sibling controllers in this package can protect their own route
// groups with the *same* middleware the main auth group uses, instead of each
// one inventing a weaker check. See AUDIT_REMEDIATION.md F-02, where a route
// group labelled "Protected" was mounted with no middleware at all.
//
// If no authenticator is configured it FAILS CLOSED with 503 rather than
// letting the request through unauthenticated (Master Rule 5.7).
func (c *AuthController) AuthMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if c == nil || c.authService == nil {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, `{"error":"authentication service unavailable"}`, http.StatusServiceUnavailable)
			})
		}
		return c.authService.Middleware(next)
	}
}

// CredentialAuthStatus enumerates the outcome of a credential check.
type CredentialAuthStatus int

const (
	// CredentialAuthOK means the credentials were verified and User is populated.
	CredentialAuthOK CredentialAuthStatus = iota
	// CredentialAuthInvalid means the credentials did not match. The caller
	// must respond 401 with a generic message.
	CredentialAuthInvalid
	// CredentialAuthLocked means the account is in a lockout window. RetryAfter
	// is the remaining time.
	CredentialAuthLocked
	// CredentialAuthRateLimited means the per-account rate limiter rejected the
	// attempt. RetryAfter is the remaining time.
	CredentialAuthRateLimited
	// CredentialAuthUnavailable means the repository is not configured, so no
	// credential check could be performed at all.
	CredentialAuthUnavailable
)

// CredentialAuthResult is the complete outcome of a credential check.
type CredentialAuthResult struct {
	Status     CredentialAuthStatus
	User       *database.UserRecord
	RetryAfter time.Duration
}

// AuthenticateCredentials is the ONLY supported way to verify an email/password
// pair in this codebase. It performs, in order:
//
//  1. account lockout check     — per-account, distributed through Redis when available
//  2. per-account rate limiting — defeats distributed proxy blasting at one email
//  3. password verification     — with a real-KDF dummy verify on an unknown
//     email, so a missing account costs the same as a
//     wrong password and cannot be timed apart
//  4. failure accounting        — increments the counter, may trip the lockout
//  5. success bookkeeping       — clears the counter and transparently upgrades a
//     stale password hash
//
// Callers must branch on Status and translate it to an HTTP response. They must
// NOT call auth.VerifyPassword directly: that bypasses steps 1, 2 and 4, which is
// exactly how POST /cli/authorize/approve came to accept unlimited password
// guesses against an account that was locked out on /auth/login
// (AUDIT_REMEDIATION.md F-03).
func (c *AuthController) AuthenticateCredentials(
	ctx context.Context, email, password, clientIP string,
) CredentialAuthResult {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || password == "" {
		return CredentialAuthResult{Status: CredentialAuthInvalid}
	}

	// 1. Account lockout.
	if locked, retryAfter := c.IsAccountLocked(ctx, email); locked {
		return CredentialAuthResult{
			Status:     CredentialAuthLocked,
			RetryAfter: retryAfter,
		}
	}

	// 2. Per-account rate limit.
	if c.accountLimiter != nil {
		res, err := c.accountLimiter.Allow(ctx, "account:"+hashEmailKey(email), 1)
		if err == nil && !res.Allowed {
			return CredentialAuthResult{
				Status:     CredentialAuthRateLimited,
				RetryAfter: res.RetryAfter,
			}
		}
	}

	// 3. Verify. A nil repository cannot verify anything, so refuse rather than
	//    treating every password as valid.
	if isNilInterface(c.repo) {
		return CredentialAuthResult{Status: CredentialAuthUnavailable}
	}

	user, err := c.repo.GetUserByEmail(ctx, email)
	verified := false
	switch {
	case err != nil || user == nil:
		// Anti-enumeration (CWE-208 / ASVS V2.2): perform a real KDF
		// verification with the same cost parameters as a genuine check.
		_ = auth.DummyVerify(password)
	case user.Status != "active":
		// An inactive account is not an authentication failure to be counted as
		// a password guess, but it must not succeed either. The hash is still
		// verified so the timing profile matches the active-account path.
		verified = auth.VerifyPassword(password, user.Password)
	default:
		verified = auth.VerifyPassword(password, user.Password)
	}

	if !verified {
		// 4. Failure accounting.
		locked, retryAfter := c.RecordFailedLogin(ctx, email)
		status := "invalid_credentials"
		if locked {
			status = "account_locked"
		}
		slog.Warn("auth.credentials.failed",
			"event", "auth.credentials.failed",
			"client_ip", clientIP,
			"email_hash", hashEmailKey(email),
			"reason", status,
		)
		telemetry.GetMetrics().AuthAttemptsTotal.WithLabelValues("failure", status).Inc()

		if locked {
			return CredentialAuthResult{
				Status:     CredentialAuthLocked,
				RetryAfter: retryAfter,
			}
		}
		return CredentialAuthResult{Status: CredentialAuthInvalid}
	}

	// 5. Success bookkeeping.
	c.ClearFailedLogins(ctx, email)
	if auth.NeedsRehash(user.Password) {
		if updatedHash, hashErr := auth.HashPassword(password); hashErr == nil {
			if err := c.repo.UpdateUserPassword(ctx, user.Email, updatedHash); err != nil {
				// The rehash is an optimisation; failing it must not fail the
				// login. Logged so the failure is visible.
				slog.Warn("auth.password.rehash_failed",
					"event", "auth.password.rehash_failed",
					"email_hash", hashEmailKey(email),
					"error", err,
				)
			}
		}
	}

	telemetry.GetMetrics().AuthAttemptsTotal.WithLabelValues("success", "ok").Inc()
	return CredentialAuthResult{Status: CredentialAuthOK, User: user}
}

// writeCredentialAuthFailure renders a non-OK credential result as an HTTP
// error. Both credential entry points use this so their responses cannot drift.
func writeCredentialAuthFailure(w http.ResponseWriter, res CredentialAuthResult) {
	switch res.Status {
	case CredentialAuthLocked:
		retrySec := int(res.RetryAfter.Seconds())
		if retrySec < 1 {
			retrySec = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(retrySec))
		http.Error(w, fmt.Sprintf(
			`{"error":"account temporarily locked due to multiple failed login attempts, please retry after %d seconds"}`,
			retrySec), http.StatusTooManyRequests)
	case CredentialAuthRateLimited:
		retrySec := int(res.RetryAfter.Seconds())
		if retrySec < 1 {
			retrySec = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(retrySec))
		http.Error(w, fmt.Sprintf(
			`{"error":"too many login attempts for this account, please retry after %d seconds"}`,
			retrySec), http.StatusTooManyRequests)
	case CredentialAuthUnavailable:
		http.Error(w, `{"error":"database service unavailable"}`, http.StatusServiceUnavailable)
	default:
		http.Error(w, `{"error":"invalid email or password"}`, http.StatusUnauthorized)
	}
}

// RateLimitMiddleware returns an HTTP middleware enforcing rate limits on authentication operations (Master Rule 4.5).
func (c *AuthController) RateLimitMiddleware() func(http.Handler) http.Handler {
	return c.rateLimitMiddleware
}

func (c *AuthController) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c.rateLimiter == nil {
			next.ServeHTTP(w, r)
			return
		}

		// Strictly resolve client IP via trusted proxy CIDR whitelist, rejecting spoofed headers from untrusted sockets
		clientIP := scandrixMiddleware.ExtractClientIP(r)

		res, err := c.rateLimiter.Allow(r.Context(), clientIP, 1)
		if err == nil && !res.Allowed {
			retrySeconds := int(res.RetryAfter.Seconds())
			if retrySeconds < 1 {
				retrySeconds = 1
			}
			w.Header().Set("Retry-After", fmt.Sprintf("%d", retrySeconds))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write(fmt.Appendf(nil, `{"error":"too many authentication requests, please retry after %d seconds"}`, retrySeconds))
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RegisterRateLimitMiddleware returns an HTTP middleware enforcing tight rate limits on account creation (Master Rule 4.5).
func (c *AuthController) RegisterRateLimitMiddleware() func(http.Handler) http.Handler {
	return c.registerRateLimitMiddleware
}

func (c *AuthController) registerRateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c.registerRateLimiter == nil {
			next.ServeHTTP(w, r)
			return
		}

		clientIP := scandrixMiddleware.ExtractClientIP(r)

		res, err := c.registerRateLimiter.Allow(r.Context(), clientIP, 1)
		if err == nil && !res.Allowed {
			retrySeconds := int(res.RetryAfter.Seconds())
			if retrySeconds < 1 {
				retrySeconds = 1
			}
			w.Header().Set("Retry-After", fmt.Sprintf("%d", retrySeconds))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write(fmt.Appendf(nil, `{"error":"too many registration attempts from this IP, please retry after %d seconds"}`, retrySeconds))
			return
		}

		next.ServeHTTP(w, r)
	})
}

const (
	// MaxFailedLoginAttempts sets the brute-force threshold before an account is temporarily locked (OWASP ASVS V2.2.1).
	MaxFailedLoginAttempts = 5
	// AccountLockoutDuration specifies how long an account is locked after exceeding failed attempts.
	AccountLockoutDuration = 15 * time.Minute
)

func hashEmailKey(email string) string {
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(h[:])
}

// IsAccountLocked checks whether an account is temporarily locked due to exceeding failed attempts.
func (c *AuthController) IsAccountLocked(ctx context.Context, email string) (bool, time.Duration) {
	if email == "" {
		return false, 0
	}
	key := hashEmailKey(email)

	// No shared store at all, but this environment requires one: the per-process
	// fallback below would be N separate counters for N replicas, so the lockout
	// would be trivially bypassed by retrying across instances (F-28/F-29/F-32).
	if limiter.DistributedRequired() && (c.cacheClient == nil || c.cacheClient.RawClient() == nil) {
		slog.Error("no shared lockout store configured; treating account as locked " +
			"because a per-process counter would not be shared between replicas")
		return true, AccountLockoutDuration
	}

	// Distributed Redis lockout check
	if c.cacheClient != nil && c.cacheClient.RawClient() != nil {
		val, err := c.cacheClient.RawClient().Get(ctx, "auth:lockout:"+key).Result()
		if err == nil {
			if val != "" {
				ttl, _ := c.cacheClient.RawClient().TTL(ctx, "auth:lockout:"+key).Result()
				if ttl <= 0 {
					ttl = AccountLockoutDuration
				}
				return true, ttl
			}
		} else if limiter.DistributedRequired() && !errors.Is(err, redis.Nil) {
			// The store is configured but unreachable. Treating that as "not
			// locked" removes the control for as long as the outage lasts, and
			// with N replicas an attacker simply retries until they land on a
			// replica whose store call fails (F-28/F-29/F-32). Deny instead:
			// an account that cannot be checked is not an account that should be
			// guessed at.
			slog.Error("account lockout store unreachable; treating account as locked",
				"error", err)
			return true, AccountLockoutDuration
		}
	}

	// In-memory fallback check
	c.failedLock.Lock()
	defer c.failedLock.Unlock()
	if entry, exists := c.failedLogins[key]; exists {
		if time.Now().Before(entry.lockedUntil) {
			return true, time.Until(entry.lockedUntil)
		}
	}
	return false, 0
}

// RecordFailedLogin records a failed authentication attempt for an account.
// Returns true along with lockout duration if the maximum failure threshold is reached.
func (c *AuthController) RecordFailedLogin(ctx context.Context, email string) (bool, time.Duration) {
	if email == "" {
		return false, 0
	}
	key := hashEmailKey(email)

	// No shared store in an environment that requires one. Recording locally
	// would put the attempt count in one replica's memory while the others keep
	// their own, so the lockout would never trip consistently (F-28/F-29/F-32).
	// Lock the account: an attempt we cannot count is not an attempt we can
	// afford to forget.
	if limiter.DistributedRequired() && (c.cacheClient == nil || c.cacheClient.RawClient() == nil) {
		slog.Error("no shared failed-attempt store configured; locking account " +
			"because a per-process counter is not shared between replicas")
		return true, AccountLockoutDuration
	}

	// Distributed Redis tracking
	if c.cacheClient != nil && c.cacheClient.RawClient() != nil {
		pipe := c.cacheClient.RawClient().TxPipeline()
		incrCmd := pipe.Incr(ctx, "auth:failed:"+key)
		pipe.Expire(ctx, "auth:failed:"+key, AccountLockoutDuration)
		_, err := pipe.Exec(ctx)
		if err == nil {
			count := incrCmd.Val()
			if count >= MaxFailedLoginAttempts {
				c.cacheClient.RawClient().Set(ctx, "auth:lockout:"+key, "1", AccountLockoutDuration)
				return true, AccountLockoutDuration
			}
			return false, 0
		}
		if limiter.DistributedRequired() {
			// The failure count was not recorded anywhere shared, so falling
			// through to a local counter would let the real attempt count drift
			// per replica. Lock the account rather than lose the count.
			slog.Error("failed-attempt store unreachable; locking account rather than losing the count",
				"error", err)
			_ = c.cacheClient.RawClient().Set(ctx, "auth:lockout:"+key, "1", AccountLockoutDuration).Err()
			return true, AccountLockoutDuration
		}
	}

	// In-memory fallback tracking
	c.failedLock.Lock()
	defer c.failedLock.Unlock()
	now := time.Now()
	entry, exists := c.failedLogins[key]
	if !exists || now.After(entry.expiresAt) {
		entry = &failedLoginEntry{
			attempts:  1,
			expiresAt: now.Add(AccountLockoutDuration),
		}
		c.failedLogins[key] = entry
		return false, 0
	}

	entry.attempts++
	entry.expiresAt = now.Add(AccountLockoutDuration)
	if entry.attempts >= MaxFailedLoginAttempts {
		entry.lockedUntil = now.Add(AccountLockoutDuration)
		return true, AccountLockoutDuration
	}
	return false, 0
}

// ClearFailedLogins resets the failed attempts and lifts any lockout on an account after successful login.
func (c *AuthController) ClearFailedLogins(ctx context.Context, email string) {
	if email == "" {
		return
	}
	key := hashEmailKey(email)

	if c.cacheClient != nil && c.cacheClient.RawClient() != nil {
		c.cacheClient.RawClient().Del(ctx, "auth:failed:"+key, "auth:lockout:"+key)
	}

	c.failedLock.Lock()
	delete(c.failedLogins, key)
	c.failedLock.Unlock()
}

// verifyTurnstileToken verifies a Cloudflare Turnstile bot challenge response with the Cloudflare API.
func verifyTurnstileToken(ctx context.Context, secret, token, remoteIP string) error {
	if strings.TrimSpace(token) == "" {
		return errors.New("turnstile token is required")
	}
	formData := url.Values{
		"secret":   {secret},
		"response": {token},
	}
	if remoteIP != "" {
		formData.Set("remoteip", remoteIP)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://challenges.cloudflare.com/turnstile/v0/siteverify", strings.NewReader(formData.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || !result.Success {
		return errors.New("turnstile verification failed")
	}
	return nil
}
