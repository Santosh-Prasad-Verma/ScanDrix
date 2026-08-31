package middleware

import (
	"net/http"
	"os"
	"strings"
)

// SecurityHeaders applies enterprise-grade HTTP security headers (Helmet equivalent, Master Rule 5.4).
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com data:; img-src 'self' data: https:; connect-src 'self'")
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
	origins := []string{
		"http://localhost:3000",
		"http://localhost:3001",
		"http://localhost:8080",
		"http://127.0.0.1:3000",
		"http://127.0.0.1:3001",
		"http://127.0.0.1:8080",
		"http://localhost:5500",
		"http://127.0.0.1:5500",
		"https://app.scandrix.dev",
		"https://dashboard.scandrix.dev",
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
			customCSRFHeader := r.Header.Get("X-Requested-With")
			if authHeader != "" || wsKeyHeader != "" || teamKeyHeader != "" || customCSRFHeader != "" {
				next.ServeHTTP(w, r)
				return
			}

			// Webhooks, OAuth callbacks, and CLI device authorization bypass browser CSRF checks
			if strings.HasPrefix(r.URL.Path, "/api/v1/webhooks") ||
				strings.HasPrefix(r.URL.Path, "/api/v1/auth/oauth") ||
				strings.HasPrefix(r.URL.Path, "/api/v1/auth/cli") ||
				strings.HasPrefix(r.URL.Path, "/cli") {
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
