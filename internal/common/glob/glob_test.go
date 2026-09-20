package glob

import (
	"testing"
)

func TestIsFileMatchingGlob(t *testing.T) {
	ClearMatcherCache()

	patterns := []string{"src/**/*.ts", "*.go", ".github/workflows/*.yml"}

	if !IsFileMatchingGlob("src/components/button.ts", patterns) {
		t.Fatalf("expected src/components/button.ts to match")
	}

	if !IsFileMatchingGlob("/src/nested/deep/file.ts", patterns) {
		t.Fatalf("expected /src/nested/deep/file.ts to match")
	}

	if !IsFileMatchingGlob("main.go", patterns) {
		t.Fatalf("expected main.go to match")
	}

	if IsFileMatchingGlob("src/components/button.js", patterns) {
		t.Fatalf("did not expect .js file to match")
	}

	// Case sensitivity test
	if IsFileMatchingGlob("MAIN.GO", []string{"*.go"}) {
		t.Fatalf("did not expect MAIN.GO to match case-sensitive *.go")
	}

	if !IsFileMatchingGlobCaseInsensitive("MAIN.GO", []string{"*.go"}) {
		t.Fatalf("expected MAIN.GO to match case-insensitive *.go")
	}

	csSize, ciSize := GetMatcherCacheSize()
	if csSize == 0 || ciSize == 0 {
		t.Fatalf("expected caches to have entries, got cs=%d ci=%d", csSize, ciSize)
	}

	ClearMatcherCache()
	csSize, ciSize = GetMatcherCacheSize()
	if csSize != 0 || ciSize != 0 {
		t.Fatalf("expected cleared caches, got cs=%d ci=%d", csSize, ciSize)
	}
}
