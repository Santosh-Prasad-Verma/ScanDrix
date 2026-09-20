---
name: scandrix-perf-debug
description: End-to-end front→back performance debugging for the ScanDrix platform (Next.js 15 dashboard, Go REST API, PostgreSQL + pgvector, Redis, RabbitMQ). Use when a screen or endpoint is slow, blank, looping, or you need to trace a UI perf issue down to the PostgreSQL query or worker queue backlog. Drives Playwright/Chrome to open + measure the screen, inspects API handlers and SQL queries via EXPLAIN (ANALYZE, BUFFERS), and isolates memory/goroutine leaks.
---

# ScanDrix Performance Debugging (Front → Back)

Repeatable, rigorous diagnostic loop for resolving ScanDrix platform performance degradation,
from the browser-rendered DOM down to the PostgreSQL execution plan, Redis cache hits, and worker queue latency.
Built from real production scenarios on the token-usage, repository-review, cockpit, and settings views.

---

## 1. Diagnostic Loop

### Step 1: Open & Observe via Browser (Playwright / Chrome)
- **Never trust the first load in development**: HMR produces phantom hydration errors, bundle-compilation spikes, or truncated-parse warnings. Reload **twice** on a settled dev server before profiling.
- Capture:
  ```js
  browser_console_messages // inspect unhandled rejections, React hydration warnings
  browser_network_requests // inspect request waterfall and fan-out
  ```
- Identify auth state: App is auth-gated. Test with seed team keys or login session.

### Step 2: Measure Client & Network Metrics via Performance API
Run in browser console via `browser_evaluate`:
```js
(() => {
  const [nav] = performance.getEntriesByType('navigation');
  const paint = performance.getEntriesByType('paint');
  const fcp = paint.find(p => p.name === 'first-contentful-paint')?.startTime || 0;
  return {
    ttfb: Math.round(nav.responseStart - nav.requestStart),
    domInteractive: Math.round(nav.domInteractive),
    fcp: Math.round(fcp),
    totalLoadTime: Math.round(nav.loadEventEnd - nav.startTime),
    encodedBodySizeKB: Math.round(nav.encodedBodySize / 1024),
  };
})();
```

**Classify the bottleneck:**
* **High TTFB (> 400ms)**: Backend-bound. Next.js Server Component or Go API is awaiting slow database queries or remote SCM provider calls.
* **Low TTFB, high FCP (> 1.2s)**: Bundle/Hydration-bound. Heavy client chunk parsing (e.g. syntax highlighter, AST tree graph viewer). Code-split with `next/dynamic`.
* **Janky UI / FPS drops**: Render-bound. Check for unmemoized complex selectors, massive table re-renders without virtualization (`@tanstack/react-virtual`), or SVG reflows.

### Step 3: Audit Network Fan-Out & API Handlers
- From `browser_network_requests`, check for:
  - Redundant duplicate calls to `/api/v1/auth/session` or `/api/v1/workspaces/current`.
  - N+1 client waterfall fetches (fetching repo list, then issuing a separate request per repo for review status).
  - Unbounded payload sizes: missing cursor pagination (`limit=50&cursor=...`).

### Step 4: Trace into Backend & Database (PostgreSQL + pgvector)
Connect directly to PostgreSQL to inspect execution plans:
```sql
-- Check running queries and transaction locks
SELECT pid, now() - pg_stat_activity.query_start AS duration, query, state
FROM pg_stat_activity
WHERE state != 'idle' AND query NOT LIKE '%pg_stat_activity%'
ORDER BY duration DESC;

-- Run EXPLAIN with execution buffers on slow queries
EXPLAIN (ANALYZE, BUFFERS, SETTINGS)
SELECT * FROM reviews 
WHERE workspace_id = '00000000-0000-0000-0000-000000000001' 
ORDER BY created_at DESC 
LIMIT 20;
```

**What to look for in EXPLAIN:**
* `Seq Scan` on tables with > 1,000 rows: Missing composite index on `(workspace_id, created_at DESC)`.
* `Buffers: shared read=...`: High disk reads indicate cold cache or missing index. Aim for `shared hit` > 99%.
* `Sort Method: external merge Disk`: Work memory (`work_mem`) exhausted by massive ORDER BY. Switch to keyset pagination instead of `OFFSET`.
* `Bitmap Heap Scan` with huge `Recheck Cond`: Index selectivity is too low.

### Step 5: Profiling Go API & Worker Runtime (pprof)
If CPU or memory is pegged on `cmd/server` or `cmd/worker`:
```bash
# 1. Capture 30s CPU profile
curl -s http://localhost:8080/debug/pprof/profile?seconds=30 > cpu.pprof
go tool pprof -top cpu.pprof

# 2. Check goroutine count & detect channel leaks
curl -s http://localhost:8080/debug/pprof/goroutine?debug=1 | head -n 30

# 3. Inspect heap allocations (in-use vs allocated memory)
curl -s http://localhost:8080/debug/pprof/heap > heap.pprof
go tool pprof -alloc_space -top heap.pprof
```

---

## 2. Dev-Environment & Production Gotchas (Costly Pitfalls)

1. **PgBouncer Port Traps (6543 vs 5432)**:
   - Port `6543` runs in **transaction mode**: It immediately closes sessions after queries, breaking prepared statements and DDL locks (`VACUUM`, `CREATE INDEX CONCURRENTLY`).
   - Port `5432` runs in **session mode**: Required for migrations (`cmd/migrate`) and long-lived connection pooling.
2. **Reload Loops on Client Components**:
   - A `useEffect` that updates URL query params or router state while including `searchParams` in its dependency array triggers an infinite reload loop (`GET /...` thousands of times, freezing the tab).
   - *Fix*: Extract query mutation to discrete event handlers, never naked effects.
3. **Turbopack Truncated-Parse Artifacts**:
   - When editing files rapidly, bind-mount file watchers (`chokidar`) can trigger on partial writes resulting in `Unexpected EOF`. Run `touch <file>` to force Turbopack to invalidate its disk cache.
4. **Vector Distance Search Latency (`pgvector`)**:
   - Querying `embedding <=> query_vector` without an `HNSW` or `IVFFlat` index causes full sequential table scans across high-dimensional vector embeddings.
   - *Fix*: Ensure `CREATE INDEX ON code_embeddings USING hnsw (embedding vector_cosine_ops);`.
5. **RabbitMQ Prefetch Overload**:
   - If worker consumers have unconstrained `PrefetchCount`, a single worker pod will hoard 500 review jobs in unacknowledged memory, stalling other idle workers and risking OOM kills.
   - *Fix*: Enforce `channel.Qos(5, 0, false)`.

---

## 3. Surgical Fix & Live Verification Protocol

1. Apply **one** fix at a time (e.g. add composite index or wrap query in Redis cache).
2. Measure baseline before vs after under identical load using:
   ```bash
   go test -bench=BenchmarkReviewPipeline -benchmem ./test/benchmark/...
   ```
3. Verify in browser with clean cache (`Shift + F5` or incognito context) and verify zero console errors and clean TTFB under 150ms.
