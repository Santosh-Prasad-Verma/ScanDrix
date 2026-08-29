# Scandrix — REST API & Streaming Protocol Specification

**Classification:** AUTHORITATIVE SPECIFICATION  
**Status:** APPROVED  
**Version:** 2.0.0  
**Base URL:** `/api/v1`  
**Transport:** HTTP/2 over TLS 1.3 | Server-Sent Events (SSE)  
**Package:** `github.com/scandrix/scandrix/internal/api`

---

## 1. Executive Summary & Authentication Protocols

The Scandrix API exposes synchronous management endpoints, long-lived Server-Sent Event (SSE) progress streams, and asynchronous batch review mechanisms. All requests must be authenticated via one of two schemes:
1. **Interactive User Sessions**: Bearer JWT tokens issued by Supabase Auth (`Authorization: Bearer <JWT>`), validated against Supabase public keys and evaluated with PostgreSQL Row-Level Security (RLS).
2. **Automated CI / CLI Keys**: High-entropy API keys with prefix `scandrix_` transmitted in the `x-scandrix-api-key` header or standard Bearer authorization.

---

## 2. Core API Endpoint Matrix

| Method | Path | Description | Authentication | Workload |
|---|---|---|---|---|
| `POST` | `/api/v1/reviews/staged` | Ingests developer staged diff from CLI for instant pre-commit review | Team API Key | Synchronous Triage |
| `GET` | `/api/v1/reviews/{id}` | Retrieves full audit status and finding breakdown for a scan run | JWT / API Key | Read |
| `GET` | `/api/v1/reviews/{id}/stream` | Long-lived SSE stream emitting stage execution events and live findings | JWT / API Key | Streaming SSE |
| `POST` | `/api/v1/findings/{id}/remediate` | Triggers autonomous patch synthesis and dual-tier sandbox verification | JWT / API Key | Async Worker Task |
| `GET` | `/api/v1/graphs/attack-path/{repo_id}` | Returns nodes and weighted edges for Next.js React Flow visualization | JWT / API Key | Read |
| `POST` | `/api/v1/assurance/manifests/sign` | Generates and cryptographically signs an In-Toto v1 release manifest | SecOps Key / JWT | Crypto Attestation |

---

## 3. Server-Sent Events (SSE) Protocol Contract

When streaming real-time scan progress (`GET /api/v1/reviews/{id}/stream`), the server emits SSE events with `Content-Type: text/event-stream`:

```
event: stage_progress
data: {"stage": "AST_TAINT_ANALYSIS", "status": "RUNNING", "progress_pct": 65, "timestamp": "2026-08-29T03:30:10Z"}

event: finding_discovered
data: {"finding_id": "F-019482fa", "rule_id": "SEC-002", "severity": "CRITICAL", "file": "internal/auth/jwt.go", "line": 42}

event: scan_completed
data: {"review_id": "REV-1234", "decision": "BLOCK", "composite_risk_score": 14.2, "duration_ms": 2850}
```

---

## 4. Standard RFC 7807 Error Response

All non-2xx HTTP errors adhere to the IETF RFC 7807 Problem Details format:

```json
{
  "type": "https://scandrix.dev/errors/policy-violation",
  "title": "Merge Policy Blocked",
  "status": 403,
  "detail": "Repository policy requires zero CRITICAL vulnerabilities, but 1 was confirmed.",
  "instance": "/api/v1/reviews/REV-1234/admit",
  "invalid_params": [
    {
      "name": "rule_id",
      "reason": "SEC-002: Insecure JWT Signing Method"
    }
  ],
  "trace_id": "trace-7e4b9-1234"
}
```

---

## 5. Per-Tenant Rate Limiting & Envoy Token Bucket Contract (API-003)

Rate limiting is enforced at the Envoy reverse proxy and API middleware layer using a distributed token-bucket algorithm backed by Redis:

| Tenant Plan Tier | Sustained Rate Limit | Burst Allowance | SSE Concurrent Streams | Applicable Scope |
|---|---|---|---|---|
| **Free / Community** | $60\text{ req/min}$ | $10\text{ requests}$ | $2\text{ active}$ | Per Tenant API Key |
| **Pro / Team** | $300\text{ req/min}$ | $50\text{ requests}$ | $10\text{ active}$ | Per Tenant API Key |
| **Enterprise Dedicated** | $1,200\text{ req/min}$ | $200\text{ requests}$ | $50\text{ active}$ | Per Tenant API Key |

### 5.1 Standard Rate Limit Headers
Every response includes standard IETF rate limiting metadata:
```http
X-RateLimit-Limit: 300
X-RateLimit-Remaining: 284
X-RateLimit-Reset: 1756447810
Retry-After: 30
```

When rate limits are exceeded, the API returns an **HTTP 429 Too Many Requests** RFC 7807 problem payload.

---

## 6. Compilable Go 1.24+ Chi Router Implementation

```go
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Server encapsulates the Chi HTTP multiplexer.
type Server struct {
	router *chi.Mux
}

// NewServer builds and configures a production Chi router.
func NewServer() *Server {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(middleware.Throttle(60))

	s := &Server{router: r}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	s.router.Route("/api/v1", func(r chi.Router) {
		r.Get("/healthz", s.handleHealthCheck)
		r.Get("/reviews/{id}/stream", s.handleReviewStream)
	})
}

func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(`{"status":"healthy","version":"2.0.0"}`)); err != nil {
		// Log write error; response already committed
		return
	}
}

// handleReviewStream streams live scan progress over Server-Sent Events.
// In production, this binds to a Redis PubSub / Go channel event listener for the reviewID.
func (s *Server) handleReviewStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported by client", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	reviewID := chi.URLParam(r, "id")
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	// Illustrative event loop demonstrating protocol wire framing
	for step := 0; step <= 100; step += 25 {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			eventData, _ := json.Marshal(map[string]any{
				"review_id":    reviewID,
				"progress_pct": step,
				"timestamp":    time.Now().UTC(),
			})
			fmt.Fprintf(w, "event: stage_progress\ndata: %s\n\n", eventData)
			flusher.Flush()
		}
	}

	fmt.Fprintf(w, "event: scan_completed\ndata: {\"review_id\":\"%s\",\"status\":\"COMPLETED\"}\n\n", reviewID)
	flusher.Flush()
}
```
