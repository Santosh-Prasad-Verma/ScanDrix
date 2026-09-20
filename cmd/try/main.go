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
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/internal/try"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	port := 8082
	slog.Info("Starting Scandrix Public Try PR Review Service (apps/try equivalent)", "port", port)

	evaluator, err := rules.NewEvaluator(rules.DefaultCatalog())
	if err != nil {
		slog.Error("Failed initializing default rules", "error", err)
		os.Exit(1)
	}

	rateLimiter := try.NewRateLimiter(2, 1*time.Hour)
	featuredRegistry := try.NewFeaturedRegistry()
	publicService := try.NewPublicReviewService(rateLimiter, featuredRegistry, evaluator)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// Global CORS Handler
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Fingerprint")
			if req.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}
			next.ServeHTTP(w, req)
		})
	})

	// Embedded Web Playground UI
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Try ScanDrix — Live Public PR & Diff Code Review Playground</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #0b0f19; color: #f8fafc; margin: 0; padding: 40px; }
        .container { max-width: 900px; margin: 0 auto; background: #131b2e; padding: 36px; border-radius: 16px; border: 1px solid #1e293b; box-shadow: 0 20px 40px rgba(0,0,0,0.6); }
        h1 { color: #38bdf8; margin-top: 0; font-size: 28px; }
        p { color: #94a3b8; line-height: 1.5; }
        .tabs { display: flex; gap: 12px; margin-bottom: 20px; border-bottom: 1px solid #334155; padding-bottom: 12px; }
        .tab-btn { background: transparent; border: none; color: #94a3b8; font-size: 15px; font-weight: 600; cursor: pointer; padding: 8px 16px; border-radius: 6px; }
        .tab-btn.active { background: #0284c7; color: white; }
        input[type="text"] { width: 100%; background: #0b0f19; color: #e2e8f0; border: 1px solid #334155; border-radius: 8px; padding: 14px; font-size: 15px; box-sizing: border-box; margin-bottom: 12px; }
        textarea { width: 100%; height: 220px; background: #0b0f19; color: #e2e8f0; border: 1px solid #334155; border-radius: 8px; padding: 14px; font-family: monospace; font-size: 14px; box-sizing: border-box; }
        button.submit-btn { background: #0284c7; color: white; border: none; padding: 14px 28px; border-radius: 8px; font-size: 15px; font-weight: bold; cursor: pointer; margin-top: 12px; transition: background 0.2s; }
        button.submit-btn:hover { background: #0369a1; }
        #results { margin-top: 24px; padding: 20px; background: #0b0f19; border: 1px solid #1e293b; border-radius: 10px; white-space: pre-wrap; font-family: monospace; font-size: 13px; display: none; }
        .card-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; margin-top: 16px; }
        .card { background: #0b0f19; border: 1px solid #334155; border-radius: 8px; padding: 16px; cursor: pointer; transition: border-color 0.2s; }
        .card:hover { border-color: #38bdf8; }
        .card-title { font-weight: bold; color: #38bdf8; margin-bottom: 6px; }
        .card-desc { font-size: 13px; color: #94a3b8; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🛡️ ScanDrix Public Assurance Engine</h1>
        <p>Review any public GitHub Pull Request or paste a unified diff for real-time automated security analysis.</p>

        <div class="tabs">
            <button class="tab-btn active" onclick="setMode('pr')">GitHub PR URL</button>
            <button class="tab-btn" onclick="setMode('featured')">Featured PRs</button>
        </div>

        <div id="prSection">
            <input type="text" id="prUrlInput" placeholder="https://github.com/owner/repo/pull/123" value="https://github.com/facebook/react/pull/24589" />
            <button class="submit-btn" onclick="reviewPublicPR()">Review Public Pull Request</button>
        </div>

        <div id="featuredSection" style="display:none;">
            <div class="card-grid" id="featuredGrid"></div>
        </div>

        <div id="results"></div>
    </div>

    <script>
        var mode = 'pr';
        function setMode(m) {
            mode = m;
            document.querySelectorAll('.tab-btn').forEach(function(b, idx) {
                b.classList.toggle('active', (m === 'pr' && idx === 0) || (m === 'featured' && idx === 1));
            });
            document.getElementById('prSection').style.display = m === 'pr' ? 'block' : 'none';
            document.getElementById('featuredSection').style.display = m === 'featured' ? 'block' : 'none';
            if (m === 'featured') loadFeatured();
        }

        function loadFeatured() {
            fetch('/cli/public/featured-reviews')
                .then(function(res) { return res.json(); })
                .then(function(data) {
                    var grid = document.getElementById('featuredGrid');
                    grid.innerHTML = data.items.map(function(i) {
                        return '<div class="card" onclick="loadFeaturedDetail(\'' + i.slug + '\')">' +
                            '<div class="card-title">' + i.pr.owner + '/' + i.pr.repo + ' #' + i.pr.prNumber + '</div>' +
                            '<div class="card-desc">' + (i.highlight || i.pr.title) + '</div>' +
                            '<div style="margin-top: 8px; color: #ef4444; font-weight: bold; font-size: 12px;">' + i.issuesCount + ' defect(s) detected</div>' +
                        '</div>';
                    }).join('');
                });
        }

        function loadFeaturedDetail(slug) {
            var resDiv = document.getElementById('results');
            resDiv.style.display = 'block';
            resDiv.innerText = 'Loading snapshot...';
            fetch('/cli/public/featured-reviews/' + slug)
                .then(function(res) { return res.json(); })
                .then(function(data) {
                    resDiv.innerText = JSON.stringify(data, null, 2);
                });
        }

        function reviewPublicPR() {
            var prUrl = document.getElementById('prUrlInput').value;
            var resDiv = document.getElementById('results');
            resDiv.style.display = 'block';
            resDiv.innerText = 'Scraping and analyzing public pull request...';

            var fp = localStorage.getItem('scandrix_fp');
            if (!fp) {
                fp = 'fp-' + Math.random().toString(36).substring(2, 15);
                localStorage.setItem('scandrix_fp', fp);
            }

            fetch('/cli/public/review-pr', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({ prUrl: prUrl, fingerprint: fp })
            })
            .then(function(resp) {
                return resp.json().then(function(data) {
                    if (!resp.ok) {
                        resDiv.innerText = 'Error: ' + JSON.stringify(data, null, 2);
                        return;
                    }

                    var jobId = data.jobId;
                    resDiv.innerText = 'Queued (Job ' + jobId + '). Polling for results...\n' + JSON.stringify(data.pr, null, 2);
                    var pollInterval = setInterval(function() {
                        fetch('/cli/public/review/jobs/' + jobId + '?omit=payload')
                            .then(function(pResp) { return pResp.json(); })
                            .then(function(pData) {
                                if (pData.status === 'COMPLETED' || pData.status === 'FAILED') {
                                    clearInterval(pollInterval);
                                    resDiv.innerText = JSON.stringify(pData, null, 2);
                                }
                            });
                    }, 1500);
                });
            })
            .catch(function(e) {
                resDiv.innerText = 'Request error: ' + e.message;
            });
        }
    </script>
</body>
</html>`
		_, _ = w.Write([]byte(html))
	})

	// Public API Endpoints

	// 1. GET /cli/public/featured-reviews
	r.Get("/cli/public/featured-reviews", func(w http.ResponseWriter, r *http.Request) {
		summaries := featuredRegistry.ListSummaries()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": summaries,
		})
	})

	// 2. GET /cli/public/featured-reviews/{slug}
	r.Get("/cli/public/featured-reviews/{slug}", func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		detail, ok := featuredRegistry.GetDetail(slug)
		if !ok {
			http.Error(w, `{"error":"featured review not found"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(detail)
	})

	// 3. POST /cli/public/review-pr
	r.Post("/cli/public/review-pr", func(w http.ResponseWriter, r *http.Request) {
		var req try.EnqueueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"code":"invalid_request","message":"invalid json payload"}`, http.StatusBadRequest)
			return
		}

		res, err := publicService.EnqueueReview(r.Context(), req)
		if err != nil {
			errStr := err.Error()
			code := "upstream_error"
			status := http.StatusBadRequest

			if strings.HasPrefix(errStr, "rate_limited") {
				code = "rate_limited"
				status = http.StatusTooManyRequests
			} else if strings.HasPrefix(errStr, "too_large") {
				code = "too_large"
			} else if strings.HasPrefix(errStr, "requires_auth") {
				code = "requires_auth"
			} else if strings.HasPrefix(errStr, "invalid_url") {
				code = "invalid_url"
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"code":    code,
				"message": errStr,
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(res)
	})

	// 4. GET /cli/public/review/jobs/{jobId}
	r.Get("/cli/public/review/jobs/{jobId}", func(w http.ResponseWriter, r *http.Request) {
		rawID := chi.URLParam(r, "jobId")
		jobID, err := uuid.Parse(rawID)
		if err != nil {
			http.Error(w, `{"code":"invalid_job_id","message":"malformed job UUID"}`, http.StatusBadRequest)
			return
		}

		omitPayload := r.URL.Query().Get("omit") == "payload"
		job, ok := publicService.GetJob(jobID, omitPayload)
		if !ok {
			http.Error(w, `{"code":"not_found","message":"job not found"}`, http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(job)
	})

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("Try playground server failed", "error", err)
		os.Exit(1)
	}
}
