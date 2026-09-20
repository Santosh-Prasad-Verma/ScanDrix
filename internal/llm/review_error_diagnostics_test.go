// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package llm_test

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

func TestRedactSecrets(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		contains string
		forbid   *regexp.Regexp
	}{
		{
			name:     "OpenAI / OpenRouter",
			input:    "key sk-or-v1-abcdef0123456789abcdef expired",
			contains: "[redacted]",
			forbid:   regexp.MustCompile(`sk-[A-Za-z0-9]|abcdef0123456789`),
		},
		{
			name:     "Anthropic",
			input:    "using sk-ant-api03-AAAAbbbbCCCCddddEEEE now",
			contains: "[redacted]",
			forbid:   regexp.MustCompile(`sk-ant-`),
		},
		{
			name:     "Google AIza",
			input:    "AIzaSyD-1234567890abcdefghijklmno is invalid",
			contains: "[redacted]",
			forbid:   regexp.MustCompile(`AIzaSy`),
		},
		{
			name:     "AWS Access Key",
			input:    "AKIAIOSFODNN7EXAMPLE denied",
			contains: "[redacted]",
			forbid:   regexp.MustCompile(`AKIAIOSFODNN7EXAMPLE`),
		},
		{
			name:     "GitHub Token",
			input:    "ghp_abcdefghijklmnopqrstuvwxyz0123456789",
			contains: "[redacted]",
			forbid:   regexp.MustCompile(`ghp_`),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := llm.RedactSecrets(tc.input)
			if !strings.Contains(out, tc.contains) {
				t.Fatalf("expected output to contain %q, got: %s", tc.contains, out)
			}
			if tc.forbid != nil && tc.forbid.MatchString(out) {
				t.Fatalf("redacted output still matched forbidden pattern %v: %s", tc.forbid, out)
			}
		})
	}

	t.Run("removes echoed authorization header", func(t *testing.T) {
		out := llm.RedactSecrets("Authorization: Bearer eyJhbGciOi.J9.abc")
		if strings.Contains(out, "eyJhbGciOi") {
			t.Fatalf("authorization token leaked in redacted output: %s", out)
		}
		if !strings.Contains(out, "[redacted]") {
			t.Fatalf("expected [redacted] marker in output: %s", out)
		}
	})

	t.Run("keeps field name and drops only the value", func(t *testing.T) {
		input := `{"api_key":"sk-live-9f8e7d6c5b4a3","model":"x"}`
		out := llm.RedactSecrets(input)
		if !strings.Contains(out, "api_key") {
			t.Fatalf("field name was removed: %s", out)
		}
		if !strings.Contains(out, "[redacted]") {
			t.Fatalf("expected [redacted] marker: %s", out)
		}
		if strings.Contains(out, "9f8e7d6c5b4a3") {
			t.Fatalf("secret key value leaked: %s", out)
		}
		if !strings.Contains(out, `"model":"x"`) {
			t.Fatalf("non-secret fields were corrupted: %s", out)
		}
	})

	t.Run("leaves an ordinary sentence untouched", func(t *testing.T) {
		said := "Rate limit exceeded for free models. Try again in 60s."
		out := llm.RedactSecrets(said)
		if out != said {
			t.Fatalf("ordinary sentence was modified: %s", out)
		}
	})
}

func TestExtractProviderMessage(t *testing.T) {
	t.Run("prefers provider sentence over SDK terse message", func(t *testing.T) {
		respBody, _ := json.Marshal(map[string]any{
			"error": map[string]any{
				"message": "No allowed providers are available for the selected model.",
			},
		})
		errObj := map[string]any{
			"message":      "Not Found",
			"responseBody": string(respBody),
		}
		msg := llm.ExtractProviderMessage(errObj)
		if msg != "No allowed providers are available for the selected model." {
			t.Fatalf("unexpected provider message: %q", msg)
		}
	})

	t.Run("reads an already-parsed body", func(t *testing.T) {
		errObj := map[string]any{
			"data": map[string]any{
				"error": map[string]any{
					"message": "Rate limit exceeded",
				},
			},
		}
		msg := llm.ExtractProviderMessage(errObj)
		if msg != "Rate limit exceeded" {
			t.Fatalf("unexpected provider message: %q", msg)
		}
	})

	t.Run("falls back to a plain-text body", func(t *testing.T) {
		errObj := map[string]any{
			"responseBody": " upstream unavailable ",
		}
		msg := llm.ExtractProviderMessage(errObj)
		if msg != "upstream unavailable" {
			t.Fatalf("unexpected provider message: %q", msg)
		}
	})

	t.Run("says nothing when provider said nothing beyond terse status", func(t *testing.T) {
		if msg := llm.ExtractProviderMessage(map[string]any{"message": "Not Found"}); msg != "" {
			t.Fatalf("expected empty message, got %q", msg)
		}
		if msg := llm.ExtractProviderMessage(nil); msg != "" {
			t.Fatalf("expected empty message for nil, got %q", msg)
		}
		if msg := llm.ExtractProviderMessage("a string"); msg != "" {
			t.Fatalf("expected empty message for raw string, got %q", msg)
		}
	})

	t.Run("redacts before returning", func(t *testing.T) {
		respBody, _ := json.Marshal(map[string]any{
			"error": map[string]any{
				"message": "Invalid key sk-or-v1-abcdef0123456789abcdef",
			},
		})
		msg := llm.ExtractProviderMessage(map[string]any{"responseBody": string(respBody)})
		if !strings.Contains(msg, "[redacted]") {
			t.Fatalf("expected [redacted] marker, got %q", msg)
		}
		if strings.Contains(msg, "abcdef0123456789") {
			t.Fatalf("leaked secret token in: %q", msg)
		}
	})

	t.Run("caps long body at 400 runes with ellipsis", func(t *testing.T) {
		respBody, _ := json.Marshal(map[string]any{
			"error": map[string]any{
				"message": strings.Repeat("x", 5000),
			},
		})
		msg := llm.ExtractProviderMessage(map[string]any{"responseBody": string(respBody)})
		runes := []rune(msg)
		if len(runes) > 401 {
			t.Fatalf("message exceeded cap: len %d", len(runes))
		}
		if !strings.HasSuffix(msg, "…") {
			t.Fatalf("message missing ellipsis suffix: %q", msg)
		}
	})

	t.Run("collapses newlines", func(t *testing.T) {
		respBody, _ := json.Marshal(map[string]any{
			"error": map[string]any{
				"message": "line one\n\n line two",
			},
		})
		msg := llm.ExtractProviderMessage(map[string]any{"responseBody": string(respBody)})
		if msg != "line one line two" {
			t.Fatalf("newlines not properly collapsed: %q", msg)
		}
	})

	t.Run("prose gating on raw response body", func(t *testing.T) {
		rawOf := func(body string) string {
			return llm.ExtractProviderMessage(map[string]any{"responseBody": body})
		}

		// 1. Short plain text explanation passes
		if raw := rawOf("Upstream provider is temporarily unavailable."); raw != "Upstream provider is temporarily unavailable." {
			t.Fatalf("expected plain explanation to pass, got %q", raw)
		}

		// 2. Drops body that echoes code or prompt
		echoed := "Request rejected. Input was: function computeInternalPricing(customer) { return customer.tier * 1.7; } and the rest of the file followed"
		if raw := rawOf(echoed); raw != "" {
			t.Fatalf("expected echoed code payload to be dropped, got %q", raw)
		}

		// 3. Drops truncated unparsed JSON
		if raw := rawOf(`{"error": "truncated at the byte limit`); raw != "" {
			t.Fatalf("expected unparsed JSON to be dropped, got %q", raw)
		}

		// 4. Drops HTML dumps
		if raw := rawOf("<html><body>502 Bad Gateway</body></html>"); raw != "" {
			t.Fatalf("expected HTML dump to be dropped, got %q", raw)
		}

		// 5. Drops multi-line stack traces
		if raw := rawOf("Error: failed\n at run (a.ts:1)\n at main (b.ts:2)"); raw != "" {
			t.Fatalf("expected stack trace to be dropped, got %q", raw)
		}

		// 6. Drops oversized text payloads (> 200 chars)
		if raw := rawOf(strings.Repeat("a ", 200)); raw != "" {
			t.Fatalf("expected oversized raw body to be dropped, got %q", raw)
		}

		// 7. Structured messages up to 300 chars still pass because provider labelled it as message
		resp300, _ := json.Marshal(map[string]any{
			"error": map[string]any{
				"message": strings.Repeat("x", 300),
			},
		})
		msg300 := llm.ExtractProviderMessage(map[string]any{"responseBody": string(resp300)})
		if len(msg300) != 300 {
			t.Fatalf("expected structured 300-char message to be retained, got len %d", len(msg300))
		}
	})
}

func TestBuildReviewErrorMessage(t *testing.T) {
	t.Run("leads with friendly sentence, facts line, and provider message", func(t *testing.T) {
		out := llm.BuildReviewErrorMessage(llm.ReviewErrorDiagnostics{
			FriendlyMessage: "The provider is rate limiting this key.",
			Provider:        "open_router",
			Model:           "moonshotai/kimi-k2:free",
			HTTPStatus:      429,
			ProviderMessage: "Rate limit exceeded for free models.",
		})
		expected := "The provider is rate limiting this key.\n\n" +
			"open_router · moonshotai/kimi-k2:free · HTTP 429\n\n" +
			"Provider said: Rate limit exceeded for free models."
		if out != expected {
			t.Fatalf("expected:\n%s\n\ngot:\n%s", expected, out)
		}
	})

	t.Run("names the model that actually ran", func(t *testing.T) {
		out := llm.BuildReviewErrorMessage(llm.ReviewErrorDiagnostics{
			FriendlyMessage: "Failed.",
			Model:           "openai/gpt-oss-120b:free",
		})
		if !strings.Contains(out, "openai/gpt-oss-120b:free") {
			t.Fatalf("missing model in diagnostics output: %s", out)
		}
	})

	t.Run("omits what it does not know instead of printing empty", func(t *testing.T) {
		out := llm.BuildReviewErrorMessage(llm.ReviewErrorDiagnostics{
			FriendlyMessage: "Something broke.",
		})
		if out != "Something broke." {
			t.Fatalf("expected plain friendly message, got: %q", out)
		}
	})

	t.Run("does not quote provider twice if friendly message already includes it", func(t *testing.T) {
		said := "No allowed providers are available."
		out := llm.BuildReviewErrorMessage(llm.ReviewErrorDiagnostics{
			FriendlyMessage: "Nowhere to route: " + said,
			ProviderMessage: said,
		})
		if strings.Contains(out, "Provider said:") {
			t.Fatalf("expected provider quote not to be duplicated: %s", out)
		}
	})

	t.Run("reports failing agent when attributed", func(t *testing.T) {
		out := llm.BuildReviewErrorMessage(llm.ReviewErrorDiagnostics{
			FriendlyMessage: "Rule verification failure.",
			AgentName:       "custom-rules",
		})
		if !strings.Contains(out, "custom-rules") {
			t.Fatalf("missing agent name in output: %s", out)
		}
	})

	t.Run("omits falsy HTTP status 0 from facts line", func(t *testing.T) {
		out := llm.BuildReviewErrorMessage(llm.ReviewErrorDiagnostics{
			FriendlyMessage: "Network connection lost.",
			HTTPStatus:      0,
		})
		if out != "Network connection lost." {
			t.Fatalf("expected HTTP 0 to be omitted, got: %q", out)
		}
	})

	t.Run("automatically redacts secrets in FriendlyMessage and ProviderMessage", func(t *testing.T) {
		out := llm.BuildReviewErrorMessage(llm.ReviewErrorDiagnostics{
			FriendlyMessage: "Failed with key sk-ant-api03-abcdef1234567890",
			Provider:        "anthropic",
			Model:           "claude-3-5-sonnet",
			HTTPStatus:      401,
			ProviderMessage: "Invalid key: Bearer eyJhbGciOi.secret.token",
		})
		if strings.Contains(out, "sk-ant-") || strings.Contains(out, "eyJhbGciOi") {
			t.Fatalf("secrets leaked in BuildReviewErrorMessage: %s", out)
		}
		if !strings.Contains(out, "[redacted]") {
			t.Fatalf("expected [redacted] marker in output: %s", out)
		}
	})
}

func TestAttemptSlotStamping(t *testing.T) {
	t.Run("names the fallback when the fallback is what failed in failover", func(t *testing.T) {
		engine := llm.NewEngine()
		primarySlot := byok.NormalizedModel{
			Provider: byok.ProviderOpenAI,
			Model:    "gpt-4o",
		}
		fallbackSlot := &byok.NormalizedModel{
			Provider: byok.ProviderOpenRouter,
			Model:    "anthropic/claude-3.5-sonnet",
		}

		_, err := engine.RunWithFailover(
			context.Background(),
			primarySlot,
			fallbackSlot,
			func(s byok.NormalizedModel) (*kernel.ExecutionResult, error) {
				return nil, errors.New("auth failed: invalid key")
			},
		)

		if err == nil {
			t.Fatal("expected error, got nil")
		}

		model, provider := llm.ReadAttemptedSlot(err)
		if model != "anthropic/claude-3.5-sonnet" {
			t.Fatalf("expected fallback model 'anthropic/claude-3.5-sonnet', got: %q", model)
		}
		if provider != string(byok.ProviderOpenRouter) {
			t.Fatalf("expected fallback provider %q, got: %q", byok.ProviderOpenRouter, provider)
		}
	})

	t.Run("names the primary when there was no fallback to try", func(t *testing.T) {
		engine := llm.NewEngine()
		primarySlot := byok.NormalizedModel{
			Provider: byok.ProviderAnthropic,
			Model:    "claude-3-5-sonnet-20241022",
		}

		_, err := engine.RunWithFailover(
			context.Background(),
			primarySlot,
			nil,
			func(s byok.NormalizedModel) (*kernel.ExecutionResult, error) {
				return nil, errors.New("rate limit reached")
			},
		)

		if err == nil {
			t.Fatal("expected error, got nil")
		}

		model, provider := llm.ReadAttemptedSlot(err)
		if model != "claude-3-5-sonnet-20241022" {
			t.Fatalf("expected primary model, got: %q", model)
		}
		if provider != "anthropic" {
			t.Fatalf("expected primary provider, got: %q", provider)
		}
	})

	t.Run("says nothing for an error that never went through failover", func(t *testing.T) {
		model, provider := llm.ReadAttemptedSlot(errors.New("unrelated error"))
		if model != "" || provider != "" {
			t.Fatalf("expected empty slot, got model=%q provider=%q", model, provider)
		}
		model, provider = llm.ReadAttemptedSlot(nil)
		if model != "" || provider != "" {
			t.Fatalf("expected empty slot for nil, got model=%q provider=%q", model, provider)
		}
	})

	t.Run("ResolvedModel and ResolvedProvider helper resolution", func(t *testing.T) {
		primarySlot := &byok.NormalizedModel{
			Provider: byok.ProviderOpenAI,
			Model:    "primary-model",
		}

		// When error has no stamped slot, resolves to slot default
		if m := llm.ResolvedModel(primarySlot, errors.New("generic")); m != "primary-model" {
			t.Fatalf("expected slot default model, got %q", m)
		}
		if p := llm.ResolvedProvider(primarySlot, errors.New("generic")); p != "openai" {
			t.Fatalf("expected slot default provider, got %q", p)
		}

		// When error has stamped slot (e.g. fallback), resolves to stamped slot
		stampedErr := llm.AttachAttemptedSlot(errors.New("fail"), "fallback-model", "gemini")
		if m := llm.ResolvedModel(primarySlot, stampedErr); m != "fallback-model" {
			t.Fatalf("expected stamped fallback model, got %q", m)
		}
		if p := llm.ResolvedProvider(primarySlot, stampedErr); p != "gemini" {
			t.Fatalf("expected stamped fallback provider, got %q", p)
		}
	})
}
