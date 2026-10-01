package middleware

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/cache/limiter"
	"github.com/scandrix/backend/pkg/models"
)

// SecurityHeaders applies enterprise-grade HTTP security headers (Helmet equivalent, Master Rule 5.4).
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com data:; img-src 'self' data: https:; connect-src 'self'")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		next.ServeHTTP(w, r)
	})
}

// CORSConfig defines configuration for Cross-Origin Resource Sharing.
type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	AllowCredentials bool
}

// DefaultCORSConfig provides standard enterprise CORS settings.
func DefaultCORSConfig() CORSConfig {
	isProd := strings.EqualFold(os.Getenv("APP_ENV"), "production") ||
		strings.EqualFold(os.Getenv("ENVIRONMENT"), "production")

	var origins []string
	if !isProd {
		origins = []string{
			"http://localhost:3000",
			"http://localhost:3001",
			"http://localhost:8080",
			"http://127.0.0.1:3000",
			"http://127.0.0.1:3001",
			"http://127.0.0.1:8080",
			"http://localhost:5500",
			"http://127.0.0.1:5500",
		}
	} else {
		origins = []string{
			"https://app.scandrix.dev",
			"https://dashboard.scandrix.dev",
		}
	}

	if appBase := os.Getenv("APP_BASE_URL"); appBase != "" {
		origins = append(origins, appBase)
	}
	if envOrigins := os.Getenv("CORS_ALLOWED_ORIGINS"); envOrigins != "" {
		for _, o := range strings.Split(envOrigins, ",") {
			trimmed := strings.TrimSpace(o)
			if trimmed != "" {
				origins = append(origins, trimmed)
			}
		}
	}

	return CORSConfig{
		AllowedOrigins: origins,
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{
			"Accept",
			"Authorization",
			"Content-Type",
			"X-Workspace-Key",
			"X-Team-Key",
			"X-Request-ID",
			"X-Requested-With",
		},
		AllowCredentials: true,
	}
}

// CORS returns a middleware handler enforcing explicit CORS policies (Master Rule 4.2).
func CORS(cfg CORSConfig) func(http.Handler) http.Handler {
	allowedOriginsMap := make(map[string]bool)
	for _, o := range cfg.AllowedOrigins {
		allowedOriginsMap[strings.TrimRight(strings.ToLower(o), "/")] = true
	}

	methodsStr := strings.Join(cfg.AllowedMethods, ", ")
	headersStr := strings.Join(cfg.AllowedHeaders, ", ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			originNorm := strings.TrimRight(strings.ToLower(origin), "/")

			if origin != "" && (allowedOriginsMap[originNorm] || len(cfg.AllowedOrigins) == 0) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				if cfg.AllowCredentials {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
				w.Header().Set("Access-Control-Allow-Methods", methodsStr)
				w.Header().Set("Access-Control-Allow-Headers", headersStr)
				w.Header().Set("Access-Control-Max-Age", "86400")
			}

			// Respond immediately to preflight requests
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// CSRFProtection validates cross-origin state-mutating requests (POST/PUT/PATCH/DELETE)
// against unauthorized cross-site invocations (Master Rule 5.4).
// isCSRFExemptCLIStartPath reports whether a /cli path is a CLI device-flow
// initiation endpoint that legitimately arrives without an Authorization header.
//
// The previous rule was `strings.HasPrefix(path, "/cli")`, which exempted every
// /cli route from CSRF (AUDIT_REMEDIATION.md F-26) -- including
// /cli/business-validation, which turned out to be reachable with nothing but an
// ambient session cookie. A prefix match cannot express "this specific route is
// safe", so the safe set is listed.
//
// Everything else under /cli now gets the standard Sec-Fetch-Site / Origin
// check. Routes that authenticate with a bearer token or an API key are already
// exempt above via the Authorization header test, which is why they need no entry
// here.
func isCSRFExemptCLIStartPath(path string) bool {
	switch path {
	case "/api/v1/cli/auth/login-init",
		"/api/v1/cli/auth/device-init",
		"/api/v1/cli/authorize/approve":
		return true
	default:
		return false
	}
}

func CSRFProtection(allowedOrigins []string) func(http.Handler) http.Handler {
	allowedMap := make(map[string]bool)
	for _, o := range allowedOrigins {
		allowedMap[strings.TrimRight(strings.ToLower(o), "/")] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Safe HTTP methods do not mutate state
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
				next.ServeHTTP(w, r)
				return
			}

			// CLI or API clients passing explicit Authorization or workspace keys are safe from ambient credential CSRF
			authHeader := r.Header.Get("Authorization")
			wsKeyHeader := r.Header.Get("X-Workspace-Key")
			teamKeyHeader := r.Header.Get("X-Team-Key")
			if authHeader != "" || wsKeyHeader != "" || teamKeyHeader != "" {
				next.ServeHTTP(w, r)
				return
			}

			// Webhooks and OAuth callbacks bypass browser CSRF checks. Neither
			// consumes an ambient session cookie: webhooks are authenticated by
			// provider signature, OAuth callbacks by the state parameter.
			if strings.HasPrefix(r.URL.Path, "/api/v1/webhooks") ||
				strings.HasPrefix(r.URL.Path, "/api/v1/auth/oauth") {
				next.ServeHTTP(w, r)
				return
			}

			// CLI device-flow *initiation* must stay exempt: the browser is the
			// party being redirected away from, so a strict Sec-Fetch-Site/Origin
			// check on a top-level navigation rejects the real flow. The security
			// decision here rests on what each route authenticates with, not on
			// the path prefix -- so it is enumerated rather than matched loosely.
			if isCSRFExemptCLIStartPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			// For browser requests with ambient credentials, inspect Sec-Fetch-Site and Origin
			secFetchSite := r.Header.Get("Sec-Fetch-Site")
			if secFetchSite == "cross-site" {
				http.Error(w, `{"error":"forbidden: cross-site request blocked by CSRF policy"}`, http.StatusForbidden)
				return
			}

			origin := r.Header.Get("Origin")
			if origin != "" {
				originNorm := strings.TrimRight(strings.ToLower(origin), "/")
				// Allow if in allowedOrigins list or matches same-origin host
				if originNorm == "http://"+r.Host || originNorm == "https://"+r.Host || allowedMap[originNorm] {
					next.ServeHTTP(w, r)
					return
				}
				if len(allowedOrigins) > 0 && !allowedMap[originNorm] {
					http.Error(w, `{"error":"forbidden: invalid request origin for state-changing request"}`, http.StatusForbidden)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

var (
	trustedProxiesLock sync.RWMutex
	trustedProxyNets   []*net.IPNet
	trustedProxiesOnce sync.Once
)

func initTrustedProxies() {
	raw := strings.TrimSpace(os.Getenv("TRUSTED_PROXIES"))
	trustPrivate := strings.EqualFold(os.Getenv("TRUST_PRIVATE_PROXIES"), "true") ||
		strings.EqualFold(os.Getenv("TRUST_PROXY"), "true") ||
		strings.EqualFold(os.Getenv("TRUST_INTERNAL_NETWORKS"), "true")

	var entries []string
	if raw != "" {
		entries = strings.Split(raw, ",")
	}

	// If no explicit proxies configured and trustPrivate is not set, default to loopback
	if len(entries) == 0 && !trustPrivate {
		_, ipv4Loopback, _ := net.ParseCIDR("127.0.0.0/8")
		_, ipv6Loopback, _ := net.ParseCIDR("::1/128")
		trustedProxyNets = []*net.IPNet{ipv4Loopback, ipv6Loopback}
		return
	}

	// Expand "private", "rfc1918", or "internal" shorthand, or apply trustPrivate flag
	finalEntries := make([]string, 0, len(entries)+8)
	addPrivate := trustPrivate
	for _, e := range entries {
		norm := strings.ToLower(strings.TrimSpace(e))
		if norm == "private" || norm == "rfc1918" || norm == "internal" {
			addPrivate = true
		} else if norm != "" {
			finalEntries = append(finalEntries, e)
		}
	}

	if addPrivate {
		finalEntries = append(finalEntries,
			"127.0.0.0/8",
			"::1/128",
			"10.0.0.0/8",
			"172.16.0.0/12",
			"192.168.0.0/16",
			"100.64.0.0/10",
			"fc00::/7",
			"fe80::/10",
		)
	}

	SetTrustedProxies(finalEntries)
}

// ResetTrustedProxies clears cached proxy CIDRs and forces re-evaluation on next request.
func ResetTrustedProxies() {
	trustedProxiesLock.Lock()
	defer trustedProxiesLock.Unlock()
	trustedProxiesOnce = sync.Once{}
	trustedProxyNets = nil
}

// SetTrustedProxies configures the list of trusted proxy CIDRs/IPs.
func SetTrustedProxies(entries []string) {
	trustedProxiesLock.Lock()
	defer trustedProxiesLock.Unlock()

	var nets []*net.IPNet
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if !strings.Contains(entry, "/") {
			// Single IP
			ip := net.ParseIP(entry)
			if ip != nil {
				if ip.To4() != nil {
					entry += "/32"
				} else {
					entry += "/128"
				}
			}
		}
		_, ipNet, err := net.ParseCIDR(entry)
		if err == nil && ipNet != nil {
			nets = append(nets, ipNet)
		}
	}
	trustedProxyNets = nets
}

// isIPInTrustedProxies checks if an IP is within the configured trusted proxies.
func isIPInTrustedProxies(ip net.IP) bool {
	trustedProxiesOnce.Do(initTrustedProxies)
	trustedProxiesLock.RLock()
	defer trustedProxiesLock.RUnlock()

	if ip == nil {
		return false
	}
	for _, network := range trustedProxyNets {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// ExtractClientIP retrieves the real client IP safely, inspecting Cloudflare headers,
// X-Real-IP, and right-to-left XFF strictly when the direct connection matches
// explicitly configured trusted proxies or private VPC ingress networks.
func ExtractClientIP(r *http.Request) string {
	clientIP := r.RemoteAddr
	directHost := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		directHost = host
		clientIP = host
	}

	directParsed := net.ParseIP(directHost)
	if directParsed == nil {
		return clientIP
	}

	// Only trust forwarding headers if the direct connecting peer is in the explicit trusted proxies list
	if !isIPInTrustedProxies(directParsed) {
		return clientIP
	}

	// 1. Cloudflare or Enterprise CDN verified client IP
	if cfIP := r.Header.Get("CF-Connecting-IP"); cfIP != "" {
		if ip := net.ParseIP(strings.TrimSpace(cfIP)); ip != nil {
			return ip.String()
		}
	}
	if trueClientIP := r.Header.Get("True-Client-IP"); trueClientIP != "" {
		if ip := net.ParseIP(strings.TrimSpace(trueClientIP)); ip != nil {
			return ip.String()
		}
	}

	// 2. Explicit reverse proxy client IP
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		if ip := net.ParseIP(strings.TrimSpace(xrip)); ip != nil {
			return ip.String()
		}
	}

	// 3. Right-to-left X-Forwarded-For traversal to identify first untrusted client hop
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			candidate := strings.TrimSpace(parts[i])
			if ip := net.ParseIP(candidate); ip != nil {
				clientIP = ip.String()
				if !isIPInTrustedProxies(ip) {
					break
				}
			}
		}
	}
	return clientIP
}

// RateLimit creates an HTTP middleware that throttles client IPs using a RateLimiter (in-memory or distributed Redis).
func RateLimit(tb limiter.RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tb == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Don't rate limit probe endpoints
			path := r.URL.Path
			if path == "/healthz" || path == "/livez" || path == "/metrics" {
				next.ServeHTTP(w, r)
				return
			}

			ip := ExtractClientIP(r)
			res, err := tb.Allow(r.Context(), ip, 1)
			if err == nil && res != nil && !res.Allowed {
				w.Header().Set("Retry-After", "1")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"too many requests, please slow down"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// TierPlanResolver resolves the active plan tier for a workspace.
type TierPlanResolver interface {
	GetWorkspacePlanDetails(ctx context.Context, wsID uuid.UUID) (*models.WorkspacePlanDetails, error)
}

type cachedTier struct {
	tier      string
	expiresAt time.Time
}

// TierAwareRateLimiter orchestrates per-tier workspace rate limiting.
type TierAwareRateLimiter struct {
	communityLimiter  *limiter.TokenBucketLimiter
	teamLimiter       *limiter.TokenBucketLimiter
	enterpriseLimiter *limiter.TokenBucketLimiter
	tierCache         sync.Map // map[uuid.UUID]cachedTier
}

// NewTierAwareRateLimiter initializes token buckets for Community, Team, and Enterprise tiers.
func NewTierAwareRateLimiter() *TierAwareRateLimiter {
	return &TierAwareRateLimiter{
		communityLimiter: limiter.NewTokenBucketLimiter(limiter.RateLimitConfig{
			Capacity:         120,
			RefillRatePerSec: 2, // 120 requests/minute
		}),
		teamLimiter: limiter.NewTokenBucketLimiter(limiter.RateLimitConfig{
			Capacity:         1200,
			RefillRatePerSec: 20, // 1,200 requests/minute
		}),
		enterpriseLimiter: limiter.NewTokenBucketLimiter(limiter.RateLimitConfig{
			Capacity:         6000,
			RefillRatePerSec: 100, // 6,000 requests/minute
		}),
	}
}

// Stop halts all active tier limiter pruning loops.
func (t *TierAwareRateLimiter) Stop() {
	if t.communityLimiter != nil {
		t.communityLimiter.Stop()
	}
	if t.teamLimiter != nil {
		t.teamLimiter.Stop()
	}
	if t.enterpriseLimiter != nil {
		t.enterpriseLimiter.Stop()
	}
}

// TierRateLimit throttles authenticated tenant requests according to workspace plan tier.
func TierRateLimit(tl *TierAwareRateLimiter, resolver TierPlanResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tl == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Bypass health and metrics probes
			path := r.URL.Path
			if path == "/healthz" || path == "/livez" || path == "/metrics" {
				next.ServeHTTP(w, r)
				return
			}

			wsID, err := auth.WorkspaceFromContext(r.Context())
			if err != nil || wsID == uuid.Nil {
				// No workspace in context, pass through to downstream handlers
				next.ServeHTTP(w, r)
				return
			}

			// Resolve plan tier with short-lived memory cache (60 seconds)
			tier := "COMMUNITY"
			now := time.Now()
			if val, ok := tl.tierCache.Load(wsID); ok {
				if c, ok := val.(cachedTier); ok && now.Before(c.expiresAt) {
					tier = c.tier
				}
			}

			if tier == "COMMUNITY" && resolver != nil {
				if planDetails, err := resolver.GetWorkspacePlanDetails(r.Context(), wsID); err == nil && planDetails != nil {
					tier = planDetails.PlanTier
					tl.tierCache.Store(wsID, cachedTier{tier: tier, expiresAt: now.Add(60 * time.Second)})
				}
			}

			normTier := strings.ToUpper(strings.TrimSpace(tier))
			var selectedLimiter *limiter.TokenBucketLimiter
			switch normTier {
			case "ENTERPRISE", "CUSTOM", "ENT":
				selectedLimiter = tl.enterpriseLimiter
			case "TEAM", "PLUS", "PRO", "TEAMS", "STARTER":
				selectedLimiter = tl.teamLimiter
			default:
				selectedLimiter = tl.communityLimiter
			}

			res, err := selectedLimiter.Allow(r.Context(), wsID.String(), 1)
			if err == nil && res != nil && !res.Allowed {
				w.Header().Set("Retry-After", "1")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error":               "rate limit exceeded for workspace tier",
					"tier":                normTier,
					"retry_after_seconds": 1,
					"message":             "Too many requests. Please slow down or upgrade your subscription plan.",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
