package utils

import (
	"testing"
)

func TestEncodePathSegment(t *testing.T) {
	// encodes slash as %2F
	res, err := EncodePathSegment("app/models")
	if err != nil || res != "app%2Fmodels" {
		t.Fatalf("expected app%%2Fmodels, got %s, err: %v", res, err)
	}

	// encodes percent as %25 before encoding slash
	res, err = EncodePathSegment("a%b")
	if err != nil || res != "a%25b" {
		t.Fatalf("expected a%%25b, got %s, err: %v", res, err)
	}

	res, err = EncodePathSegment("a%/b")
	if err != nil || res != "a%25%2Fb" {
		t.Fatalf("expected a%%25%%2Fb, got %s, err: %v", res, err)
	}

	// strips leading and trailing slashes
	res, err = EncodePathSegment("/src/api/")
	if err != nil || res != "src%2Fapi" {
		t.Fatalf("expected src%%2Fapi, got %s, err: %v", res, err)
	}

	// trims surrounding whitespace
	res, err = EncodePathSegment("  src/api  ")
	if err != nil || res != "src%2Fapi" {
		t.Fatalf("expected src%%2Fapi, got %s, err: %v", res, err)
	}

	// rejects empty string
	if _, err = EncodePathSegment(""); err == nil {
		t.Fatalf("expected error for empty path")
	}

	// rejects whitespace only
	if _, err = EncodePathSegment("   "); err == nil {
		t.Fatalf("expected error for whitespace path")
	}

	// rejects root
	if _, err = EncodePathSegment("/"); err == nil {
		t.Fatalf("expected error for root path")
	}
	if _, err = EncodePathSegment("//"); err == nil {
		t.Fatalf("expected error for double-slash path")
	}
}

func TestDecodePathSegment(t *testing.T) {
	// decodes %2F as slash
	res := DecodePathSegment("app%2Fmodels")
	if res != "app/models" {
		t.Fatalf("expected app/models, got %s", res)
	}

	// decodes %25 as percent
	res = DecodePathSegment("a%25b")
	if res != "a%b" {
		t.Fatalf("expected a%%b, got %s", res)
	}

	// does not re-interpret decoded percent
	res = DecodePathSegment("a%252Fb")
	if res != "a%2Fb" {
		t.Fatalf("expected a%%2Fb, got %s", res)
	}

	// case insensitive hex decoding
	res = DecodePathSegment("app%2fmodels")
	if res != "app/models" {
		t.Fatalf("expected app/models from lowercase hex, got %s", res)
	}
}

func TestBuildGroupFolderName(t *testing.T) {
	paths := []string{"src/api", "app/models", "lib/utils"}
	folder, err := BuildGroupFolderName(paths)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Sorted lexicographically: app/models -> lib/utils -> src/api
	expected := "app%2Fmodels&lib%2Futils&src%2Fapi"
	if folder != expected {
		t.Fatalf("expected %s, got %s", expected, folder)
	}

	// Rejects duplicates
	dupPaths := []string{"src/api", "src/api", "app/models"}
	_, err = BuildGroupFolderName(dupPaths)
	if err == nil {
		t.Fatalf("expected error for duplicate paths in group")
	}

	// Rejects empty slice
	if _, err := BuildGroupFolderName(nil); err == nil {
		t.Fatalf("expected error for nil slice")
	}
	if _, err := BuildGroupFolderName([]string{}); err == nil {
		t.Fatalf("expected error for empty slice")
	}
}

func TestParseGroupFolderName(t *testing.T) {
	folder := "app%2Fmodels&src%2Fapi"
	paths, err := ParseGroupFolderName(folder)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(paths) != 2 || paths[0] != "app/models" || paths[1] != "src/api" {
		t.Fatalf("unexpected parsed paths: %+v", paths)
	}

	// Round-trip verification
	original := []string{"services/auth", "services/billing", "shared/types"}
	encoded, err := BuildGroupFolderName(original)
	if err != nil {
		t.Fatalf("build error: %v", err)
	}
	decoded, err := ParseGroupFolderName(encoded)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if len(decoded) != len(original) {
		t.Fatalf("length mismatch: %d vs %d", len(decoded), len(original))
	}
	for i := range decoded {
		if decoded[i] != original[i] {
			t.Fatalf("mismatch at index %d: %s vs %s", i, decoded[i], original[i])
		}
	}
}

func TestValidateGroupPaths(t *testing.T) {
	valid := []string{"app/models", "src/api"}
	if err := ValidateGroupPaths(valid); err != nil {
		t.Fatalf("expected valid paths, got error: %v", err)
	}

	invalidEmpty := []string{"app/models", ""}
	if err := ValidateGroupPaths(invalidEmpty); err == nil {
		t.Fatalf("expected error for empty element in group paths")
	}

	invalidWhitespace := []string{"app/models", "   "}
	if err := ValidateGroupPaths(invalidWhitespace); err == nil {
		t.Fatalf("expected error for whitespace element in group paths")
	}
}
