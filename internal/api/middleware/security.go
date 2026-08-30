package middleware

import (
	"net/http"
	"strings"
)

// SecurityHeaders applies enterprise-grade HTTP security headers (Helmet equivalent, Master Rule 5.4).
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
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
	return CORSConfig{
		AllowedOrigins: []string{
			"http://localhost:3000",
			"http://localhost:3001",
			"https://app.scandrix.dev",
			"https://dashboard.scandrix.dev",
		},
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{
			"Accept",
			"Authorization",
			"Content-Type",
			"X-Workspace-Key",
			"X-Team-Key",
			"X-Request-ID",
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
