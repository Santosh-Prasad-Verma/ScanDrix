package syntax_test

import (
	"testing"

	"github.com/scandrix/backend/internal/sandbox/syntax"
)

func TestSandboxSyntaxValidator(t *testing.T) {
	v := syntax.NewSandboxSyntaxValidator()

	// 1. Valid Go Snippet
	validGo := `
func Add(a, b int) int {
	return a + b
}
`
	rGo := v.ValidateSuggestion("math/calc.go", validGo)
	if !rGo.IsValid {
		t.Errorf("expected valid Go, got error: %s", rGo.ErrorMessage)
	}

	// 2. Broken Go Snippet (unclosed brace)
	brokenGo := `
func Broken(x int) {
	if x > 0 {
		println(x)
`
	rBrokenGo := v.ValidateSuggestion("broken.go", brokenGo)
	if rBrokenGo.IsValid {
		t.Errorf("expected invalid Go snippet to fail syntax check")
	}

	// 3. Valid JSON
	validJSON := `{"status":"ok","code":200,"items":["a","b"]}`
	rJSON := v.ValidateSuggestion("config.json", validJSON)
	if !rJSON.IsValid {
		t.Errorf("expected valid JSON, got error: %s", rJSON.ErrorMessage)
	}

	// 4. Broken JSON
	brokenJSON := `{"status":"ok", "items": [1, 2, }`
	rBrokenJSON := v.ValidateSuggestion("bad.json", brokenJSON)
	if rBrokenJSON.IsValid {
		t.Errorf("expected broken JSON to fail syntax check")
	}

	// 5. Generic Braces Balancing (JS/Python)
	validJS := `const handler = (event) => { console.log([1, 2, 3]); };`
	rJS := v.ValidateSuggestion("index.ts", validJS)
	if !rJS.IsValid {
		t.Errorf("expected valid JS, got error: %s", rJS.ErrorMessage)
	}

	brokenJS := `function test() { if (true) { console.log('missing brace'); }`
	rBrokenJS := v.ValidateSuggestion("app.js", brokenJS)
	if rBrokenJS.IsValid {
		t.Errorf("expected unbalanced JS to fail syntax check")
	}
}
