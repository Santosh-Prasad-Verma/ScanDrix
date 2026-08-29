package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/rules"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	port := 8082
	slog.Info("Starting Scandrix 'Try' Interactive Sandbox Playground (apps/try equivalent)", "port", port)

	evaluator, err := rules.NewEvaluator(rules.DefaultCatalog())
	if err != nil {
		slog.Error("Failed initializing default rules", "error", err)
		os.Exit(1)
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(15 * time.Second))

	// Embedded Trial Web Playground UI
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		html := `<!DOCTYPE html>
<html>
<head>
    <title>Try Scandrix - Live Code Review Playground</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #0f172a; color: #f8fafc; margin: 0; padding: 40px; }
        .container { max-width: 800px; margin: 0 auto; background: #1e293b; padding: 30px; border-radius: 12px; box-shadow: 0 10px 25px rgba(0,0,0,0.5); }
        h1 { color: #38bdf8; margin-top: 0; }
        textarea { width: 100%; height: 200px; background: #0f172a; color: #e2e8f0; border: 1px solid #475569; border-radius: 8px; padding: 12px; font-family: monospace; font-size: 14px; box-sizing: border-box; }
        button { background: #0284c7; color: white; border: none; padding: 12px 24px; border-radius: 6px; font-weight: bold; cursor: pointer; margin-top: 12px; }
        button:hover { background: #0369a1; }
        #results { margin-top: 24px; padding: 16px; background: #0f172a; border-radius: 8px; white-space: pre-wrap; font-family: monospace; font-size: 13px; display: none; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Try Scandrix Playground</h1>
        <p>Paste a unified git diff below to test our real-time deterministic assurance engine:</p>
        <textarea id="diffInput">diff --git a/auth/creds.go b/auth/creds.go
--- a/auth/creds.go
+++ b/auth/creds.go
@@ -10,1 +10,2 @@ func Connect() {
+    awsKey := "AKIAIOSFODNN7EXAMPLE"
+    cmd := exec.Command("bash", "-c", "rm -rf " + input)
}</textarea>
        <button onclick="runReview()">Analyze Diff</button>
        <div id="results"></div>
    </div>
    <script>
        async function runReview() {
            const diff = document.getElementById('diffInput').value;
            const resDiv = document.getElementById('results');
            resDiv.style.display = 'block';
            resDiv.innerText = 'Analyzing...';
            try {
                const resp = await fetch('/api/try/review', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({ diff })
                });
                const data = await resp.json();
                resDiv.innerText = JSON.stringify(data, null, 2);
            } catch (e) {
                resDiv.innerText = 'Error: ' + e.message;
            }
        }
    </script>
</body>
</html>`
		_, _ = w.Write([]byte(html))
	})

	// Public Interactive Review Endpoint
	r.Post("/api/try/review", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Diff string `json:"diff"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Diff == "" {
			http.Error(w, `{"error":"invalid or empty diff"}`, http.StatusBadRequest)
			return
		}

		patches, err := diff.ParseUnifiedDiff(strings.NewReader(req.Diff))
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"diff parse error: %s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		trialID := uuid.New()
		findings := evaluator.EvaluatePatches(trialID, trialID, patches)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"trial_id":       trialID,
			"findings_count": len(findings),
			"findings":       findings,
			"status":         "COMPLETED",
		})
	})

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("Try playground server failed", "error", err)
		os.Exit(1)
	}
}
