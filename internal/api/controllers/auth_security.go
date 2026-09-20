package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"time"

	scandrixMiddleware "github.com/scandrix/backend/internal/api/middleware"
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

// isSecureRequest dynamically evaluates whether the active connection or deployment context enforces HTTPS.
// Avoids hardcoding; dynamically checks TLS handshake, reverse proxy headers, and production environment flags.
func isSecureRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	if strings.EqualFold(os.Getenv("COOKIE_SECURE"), "true") ||
		strings.EqualFold(os.Getenv("ENVIRONMENT"), "production") ||
		strings.EqualFold(os.Getenv("APP_ENV"), "production") {
		return true
	}
	return false
}

// setAuthCookie writes an authentication session cookie with production security flags.
func setAuthCookie(w http.ResponseWriter, r *http.Request, name, value string, maxAge int) {
	secure := isSecureRequest(r)
	cookieName := name
	if secure && strings.EqualFold(os.Getenv("ENVIRONMENT"), "production") && strings.HasPrefix(name, "scandrix_") {
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

type failedLoginEntry struct {
	attempts    int
	expiresAt   time.Time
	lockedUntil time.Time
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

	// Distributed Redis lockout check
	if c.cacheClient != nil && c.cacheClient.RawClient() != nil {
		val, err := c.cacheClient.RawClient().Get(ctx, "auth:lockout:"+key).Result()
		if err == nil && val != "" {
			ttl, _ := c.cacheClient.RawClient().TTL(ctx, "auth:lockout:"+key).Result()
			if ttl <= 0 {
				ttl = AccountLockoutDuration
			}
			return true, ttl
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

