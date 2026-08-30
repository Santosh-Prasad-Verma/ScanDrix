package deliberation

import (
	"regexp"
	"strings"

	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/pkg/models"
)

var (
	// Concurrency Patterns
	reGoLoopVarCapture = regexp.MustCompile(`for\s+.*(:=|\bin\b).*\n[\s\S]*?go\s+func\s*\(\s*\)`)
	reMapConcurrent    = regexp.MustCompile(`(make\s*\(\s*map\[|var\s+[a-zA-Z0-9_]+\s+map\[)`)

	// Memory Leak Patterns
	reHttpBodyNoClose = regexp.MustCompile(`resp,\s*err\s*:=\s*http\.(Get|Post|Do)`)
	reSqlRowsNoClose  = regexp.MustCompile(`rows,\s*err\s*:=\s*(db|tx|conn)\.Query`)
	reTickerNoStop    = regexp.MustCompile(`time\.NewTicker\s*\(`)

	// SQL Optimization Patterns
	reSqlInLoop      = regexp.MustCompile(`for\s+.*\{\s*[\s\S]*?(db|tx|ctx|repo)\.(Query|Exec|Find|Select)`)
	reSelectStarJoin = regexp.MustCompile(`(?i)SELECT\s+\*\s+FROM\s+[a-zA-Z0-9_]+\s+(INNER\s+|LEFT\s+|RIGHT\s+)?JOIN`)
	reOrderByRand    = regexp.MustCompile(`(?i)ORDER\s+BY\s+(RAND\(\)|RANDOM\(\))`)
)

// DetectSpecializedCandidates runs specialized domain heuristics for the new expert personas.
func DetectSpecializedCandidates(patches []*diff.FilePatch) []CandidateFinding {
	var candidates []CandidateFinding

	for _, patch := range patches {
		content := extractAddedContent(patch)

		// 1. Concurrency Auditor Checks
		if reHttpBodyNoClose.MatchString(content) && !strings.Contains(content, "Body.Close") {
			candidates = append(candidates, ProposeFinding(
				PersonaMemoryLeakSpecialist,
				patch.NewPath,
				1,
				15,
				"Unclosed HTTP Response Body Leak",
				models.SeverityHigh,
				0.92,
				"HTTP response body is not closed via `defer resp.Body.Close()`. This leads to connection pooling starvation and socket descriptor leaks.",
				"defer resp.Body.Close()",
			))
		}

		if reSqlRowsNoClose.MatchString(content) && !strings.Contains(content, "rows.Close") {
			candidates = append(candidates, ProposeFinding(
				PersonaMemoryLeakSpecialist,
				patch.NewPath,
				1,
				20,
				"Unclosed SQL Database Rows Resource Leak",
				models.SeverityHigh,
				0.90,
				"Database `rows` object is not closed via `defer rows.Close()`, leaking database connections from the pool.",
				"defer rows.Close()",
			))
		}

		if reTickerNoStop.MatchString(content) && !strings.Contains(content, "Stop()") {
			candidates = append(candidates, ProposeFinding(
				PersonaMemoryLeakSpecialist,
				patch.NewPath,
				1,
				10,
				"Unstopped time.Ticker Goroutine Leak",
				models.SeverityMedium,
				0.85,
				"time.NewTicker was instantiated without `defer ticker.Stop()`, causing GC memory retention.",
				"defer ticker.Stop()",
			))
		}

		// 2. Concurrency Auditor Checks
		if reGoLoopVarCapture.MatchString(content) {
			candidates = append(candidates, ProposeFinding(
				PersonaConcurrencyAuditor,
				patch.NewPath,
				1,
				20,
				"Goroutine Loop Variable Capture Race",
				models.SeverityHigh,
				0.90,
				"Loop variable captured by closure inside goroutine without explicit argument passing or rebinding.",
				"Pass the variable as a parameter into the goroutine closure.",
			))
		}

		if strings.Contains(content, "go func") && reMapConcurrent.MatchString(content) && !strings.Contains(content, "sync.") && !strings.Contains(content, "sync.Map") {
			candidates = append(candidates, ProposeFinding(
				PersonaConcurrencyAuditor,
				patch.NewPath,
				1,
				25,
				"Unsynchronized Concurrent Map Access Race Condition",
				models.SeverityCritical,
				0.94,
				"Go standard maps are not safe for concurrent usage across goroutines. Concurrent read/write causes fatal runtime panic.",
				"Use sync.RWMutex or sync.Map to protect map read/writes.",
			))
		}

		// 3. SQL Optimizer Checks
		if reSqlInLoop.MatchString(content) {
			candidates = append(candidates, ProposeFinding(
				PersonaSQLOptimizer,
				patch.NewPath,
				1,
				30,
				"N+1 Database Query Anti-Pattern in Loop",
				models.SeverityHigh,
				0.91,
				"Database query executed iteratively inside a loop. This degrades database throughput exponentially. Use batch queries with WHERE IN (...) or SQL JOINs.",
				"Batch query using WHERE IN (...) or JOIN instead of iterative execution.",
			))
		}

		if reSelectStarJoin.MatchString(content) {
			candidates = append(candidates, ProposeFinding(
				PersonaSQLOptimizer,
				patch.NewPath,
				1,
				10,
				"Inefficient SELECT * in Multi-Table SQL Join",
				models.SeverityMedium,
				0.88,
				"Using `SELECT *` across joined tables retrieves unused columns, bloats network transport, and disables covering index scans.",
				"Explicitly project required columns: SELECT t1.id, t1.name, t2.status FROM ...",
			))
		}

		if reOrderByRand.MatchString(content) {
			candidates = append(candidates, ProposeFinding(
				PersonaSQLOptimizer,
				patch.NewPath,
				1,
				5,
				"Catastrophic Full-Table Sort with ORDER BY RAND()",
				models.SeverityHigh,
				0.93,
				"`ORDER BY RAND()` triggers full table scans and temporary disk table sorting on large datasets.",
				"Sample rows using indexed id ranges or offset tables.",
			))
		}
	}

	return candidates
}

func extractAddedContent(patch *diff.FilePatch) string {
	var b strings.Builder
	for _, hunk := range patch.Hunks {
		for _, line := range hunk.Lines {
			if line.Type == diff.LineAddition {
				b.WriteString(line.Content + "\n")
			}
		}
	}
	return b.String()
}
