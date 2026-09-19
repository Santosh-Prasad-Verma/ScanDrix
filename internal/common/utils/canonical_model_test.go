package utils_test

import (
	"testing"

	"github.com/scandrix/backend/internal/common/utils"
)

func TestCanonicalModelID(t *testing.T) {
	t.Run("strips a leading provider: prefix", func(t *testing.T) {
		if res := utils.CanonicalModelID("anthropic:claude-opus-5"); res != "claude-opus-5" {
			t.Fatalf("expected claude-opus-5, got %s", res)
		}
		if res := utils.CanonicalModelID("google_gemini:gemini-2.5-pro"); res != "gemini-2.5-pro" {
			t.Fatalf("expected gemini-2.5-pro, got %s", res)
		}
	})

	t.Run("strips a Bedrock :<version> suffix instead of returning the version", func(t *testing.T) {
		if res := utils.CanonicalModelID("us.anthropic.claude-3-5-haiku-20241022-v1:0"); res != "us.anthropic.claude-3-5-haiku-20241022-v1" {
			t.Fatalf("expected us.anthropic.claude-3-5-haiku-20241022-v1, got %s", res)
		}
	})

	t.Run("canonicalizes the versioned and unversioned Bedrock id to the SAME value", func(t *testing.T) {
		v1 := utils.CanonicalModelID("us.anthropic.claude-3-5-haiku-20241022-v1:0")
		v2 := utils.CanonicalModelID("us.anthropic.claude-3-5-haiku-20241022-v1")
		if v1 != v2 {
			t.Fatalf("expected identical canonical ids, got %s vs %s", v1, v2)
		}
	})

	t.Run("keeps DISTINCT Bedrock models distinct", func(t *testing.T) {
		haiku := utils.CanonicalModelID("us.anthropic.claude-3-5-haiku-20241022-v1:0")
		sonnet := utils.CanonicalModelID("us.anthropic.claude-3-5-sonnet-20241022-v2:0")
		if haiku == sonnet {
			t.Fatalf("expected distinct models not to collide: %s vs %s", haiku, sonnet)
		}
	})

	t.Run("preserves a provider/ slash prefix", func(t *testing.T) {
		if res := utils.CanonicalModelID("vertex_ai/gemini-2.5-pro"); res != "vertex_ai/gemini-2.5-pro" {
			t.Fatalf("expected vertex_ai/gemini-2.5-pro, got %s", res)
		}
	})

	t.Run("leaves a plain id untouched and handles empty", func(t *testing.T) {
		if res := utils.CanonicalModelID("gpt-4o-2024-08-06"); res != "gpt-4o-2024-08-06" {
			t.Fatalf("expected gpt-4o-2024-08-06, got %s", res)
		}
		if res := utils.CanonicalModelID(""); res != "" {
			t.Fatalf("expected empty string, got %s", res)
		}
		if res := utils.CanonicalModelID("   "); res != "" {
			t.Fatalf("expected empty string for whitespace, got %s", res)
		}
	})
}
