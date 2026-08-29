# Scandrix — Go 1.24+ & TypeScript Enterprise Coding Standards

**Classification:** NORMATIVE DEVELOPER SPECIFICATION  
**Status:** APPROVED FOR IMPLEMENTATION  
**Version:** 3.0.0  
**Domain:** Code Quality, Static Analysis & Enterprise Engineering Discipline

---

## 1. Go Standards (Go 1.24+)

### 1.1 Context Propagation & Idiomatic Error Handling
1. **First-Parameter Context**: Every function performing I/O, database access, or inter-service RPC must accept `ctx context.Context` as its first parameter.
2. **Explicit Error Wrapping**: Errors must be wrapped with contextual operations using `%w`:
   ```go
   if err := s.db.ExecContext(ctx, query, args...); err != nil {
       return fmt.Errorf("failed to persist evidence packet for commit %s: %w", commitSHA, err)
   }
   ```
3. **No Uncaught Panics**: Production services and worker daemons must never panic. Recover middleware is installed at all HTTP router and AMQP queue consumer boundaries.
4. **Custom Typed Errors**: Domain errors must be distinct types implementing `error` and `errors.Is`/`errors.As` interfaces.

### 1.2 Bounded Concurrency & Goroutine Leak Prevention
1. **Semaphore-Bounded Worker Pools**: Unbounded `go func()` invocations are strictly forbidden. All concurrent operations must be throttled:
   ```go
   sem := make(chan struct{}, maxConcurrency)
   var wg sync.WaitGroup
   
   for _, file := range changedFiles {
       select {
       case <-ctx.Done():
           return ctx.Err()
       case sem <- struct{}{}:
       }
       
       wg.Add(1)
       go func(f ChangedFile) {
           defer func() {
               <-sem
               wg.Done()
           }()
           analyzeFile(ctx, f)
       }(file)
   }
   wg.Wait()
   ```
2. **Channel Ownership**: The goroutine that writes to a channel is the sole entity allowed to close it. Never close a channel from a reader.

### 1.3 Memory Optimization & Structured Logging (`log/slog`)
1. **High-Performance Logging**: Use `log/slog` with typed attributes:
   ```go
   slog.InfoContext(ctx, "completed 18-stage assurance scan",
       slog.String("tenant_id", tenantID),
       slog.String("commit_sha", commitSHA),
       slog.Int("finding_count", len(findings)),
       slog.Duration("duration", time.Since(start)),
   )
   ```
2. **Zero-Allocation Byte Reuse**: High-throughput parsing and hashing routines must reuse byte buffers via `sync.Pool`:
   ```go
   var bufferPool = sync.Pool{
       New: func() interface{} {
           return new(bytes.Buffer)
       },
   }
   ```

---

## 2. Next.js 15 & TypeScript Frontend Standards

### 2.1 App Router & Server Component Architecture
1. **Server Components by Default**: Pages and layout wrappers in `src/app/` are React Server Components (RSC) to maximize SEO, performance, and server-side cacheability.
2. **Selective Client Leaves (`'use client'`)**: Client components are restricted to interactive leaves (e.g., interactive React Flow graph visualizers, modal dialogs, real-time SSE progress bars).
3. **Radix UI Accessible Primitives**: All UI components must be built on Radix UI primitives (`@radix-ui/react-dialog`, `@radix-ui/react-dropdown-menu`) styled via Tailwind CSS 4 utility tokens.

### 2.2 Server State Management (TanStack Query v5)
Remote API state is strictly managed using custom React Query hooks:
```typescript
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';

export function useAssuranceRun(runId: string) {
  return useQuery({
    queryKey: ['assurance-runs', runId],
    queryFn: async () => {
      const res = await fetch(`/api/v1/scans/${runId}`);
      if (!res.ok) throw new Error('Failed to fetch assurance run');
      return res.json();
    },
    staleTime: 10_000,
  });
}
```

---

## 3. Database Standards (PostgreSQL 16 & pgvector)

1. **Mandatory Row-Level Security (RLS)**: Every transactional query connection must execute `SET LOCAL app.current_tenant_id = $1` within its transaction boundary.
2. **Parameterized SQL Queries**: Direct string concatenation or raw `fmt.Sprintf` into SQL queries is strictly forbidden and blocked by CI AST linters.
3. **Zero-Downtime Migration Standard**:
   - Adding columns: `ALTER TABLE ... ADD COLUMN ... DEFAULT NULL;`
   - Adding foreign keys: Must use `NOT VALID`, followed by a separate `VALIDATE CONSTRAINT` transaction.
   - Adding indexes: Must use `CREATE INDEX CONCURRENTLY`.

---

## 4. GolangCI-Lint Configuration (`.golangci.yml`)

The platform enforces zero-tolerance linting via `.golangci.yml`:
```yaml
run:
  timeout: 5m
  go: "1.24"

linters:
  enable:
    - errcheck
    - gosimple
    - govet
    - ineffassign
    - staticcheck
    - unused
    - gosec
    - prealloc
    - unconvert
    - revive
    - bodyclose

linters-settings:
  gosec:
    severity: "medium"
    confidence: "medium"
```
