package callgraph

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

type memoryFileProvider struct {
	files map[string]string
}

func (m *memoryFileProvider) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if content, ok := m.files[path]; ok {
		return []byte(content), nil
	}
	return nil, fmt.Errorf("file not found: %s", path)
}

func (m *memoryFileProvider) ListFiles(ctx context.Context) ([]string, error) {
	keys := make([]string, 0, len(m.files))
	for k := range m.files {
		keys = append(keys, k)
	}
	return keys, nil
}

func TestImpactAnalyzer_GoBreakingSignatureAndCallSites(t *testing.T) {
	analyzer := NewImpactAnalyzer()

	// Simulate diff where CalculateDiscount signature changes by adding a parameter
	diff := `
diff --git a/billing/calculator.go b/billing/calculator.go
--- a/billing/calculator.go
+++ b/billing/calculator.go
@@ -10,3 +10,3 @@
-func CalculateDiscount(price float64) float64 {
+func CalculateDiscount(price float64, tier string) float64 {
`

	files := &memoryFileProvider{
		files: map[string]string{
			"billing/calculator.go": `package billing
func CalculateDiscount(price float64, tier string) float64 {
	return price * 0.9
}
`,
			"checkout/service.go": `package checkout
import "billing"
func ProcessOrder() {
	discount := billing.CalculateDiscount(100.0)
	println(discount)
}
`,
			"invoice/generator.go": `package invoice
import "billing"
func GenerateInvoice() {
	d := billing.CalculateDiscount(50.0)
}
`,
		},
	}

	report, err := analyzer.AnalyzeDiffWithFiles(context.Background(), "repo-123", diff, files)
	assert.NoError(t, err)
	assert.NotNil(t, report)

	// Verify symbol change was detected as breaking
	assert.Len(t, report.ModifiedSymbols, 1)
	sym := report.ModifiedSymbols[0]
	assert.Equal(t, "CalculateDiscount", sym.SymbolName)
	assert.Equal(t, "billing/calculator.go", sym.FilePath)
	assert.True(t, sym.IsBreaking)

	// Verify call sites in unchanged files checkout/service.go and invoice/generator.go
	assert.Len(t, report.CallSites, 2)
	assert.Contains(t, report.AffectedFiles, "checkout/service.go")
	assert.Contains(t, report.AffectedFiles, "invoice/generator.go")
	// The modified file itself must not be in AffectedFiles
	assert.NotContains(t, report.AffectedFiles, "billing/calculator.go")
}

func TestImpactAnalyzer_TypeScriptAndPythonChanges(t *testing.T) {
	analyzer := NewImpactAnalyzer()

	diff := `
diff --git a/src/auth.ts b/src/auth.ts
--- a/src/auth.ts
+++ b/src/auth.ts
@@ -5,2 +5,2 @@
-export function verifyToken(token: string): boolean {
+export function verifyToken(token: string, secret: string): boolean {
diff --git a/services/worker.py b/services/worker.py
--- a/services/worker.py
+++ b/services/worker.py
@@ -12,2 +12,2 @@
-def dispatch_task(task_id: str) -> None:
+def dispatch_task(task_id: str, priority: int = 1) -> None:
`

	report, err := analyzer.AnalyzeDiff(context.Background(), "repo-456", diff)
	assert.NoError(t, err)
	assert.NotNil(t, report)

	assert.Len(t, report.ModifiedSymbols, 2)

	tsSym := report.ModifiedSymbols[0]
	assert.Equal(t, "verifyToken", tsSym.SymbolName)
	assert.True(t, tsSym.IsBreaking)

	pySym := report.ModifiedSymbols[1]
	assert.Equal(t, "dispatch_task", pySym.SymbolName)
	assert.True(t, pySym.IsBreaking)
}

func TestImpactAnalyzer_DeletedSymbolIsBreaking(t *testing.T) {
	analyzer := NewImpactAnalyzer()

	diff := `
diff --git a/pkg/legacy.go b/pkg/legacy.go
--- a/pkg/legacy.go
+++ b/pkg/legacy.go
@@ -20,3 +20,0 @@
-func DeprecatedHelper(input string) string {
-	return input
-}
`

	report, err := analyzer.AnalyzeDiff(context.Background(), "repo-789", diff)
	assert.NoError(t, err)
	assert.NotNil(t, report)

	assert.Len(t, report.ModifiedSymbols, 1)
	assert.Equal(t, "DeprecatedHelper", report.ModifiedSymbols[0].SymbolName)
	assert.True(t, report.ModifiedSymbols[0].IsBreaking)
	assert.Empty(t, report.ModifiedSymbols[0].NewSignature)
}
