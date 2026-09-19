package translations

import (
	"strings"
	"testing"
)

func TestGetTranslations(t *testing.T) {
	enDict := GetTranslations("en-US")
	if enDict == nil {
		t.Fatalf("expected non-nil en-US dictionary")
	}

	if !strings.Contains(enDict.ReviewComment.TalkToDrixy, "Drixy") {
		t.Fatalf("expected Drixy in reviewComment.TalkToDrixy, got %q", enDict.ReviewComment.TalkToDrixy)
	}

	ptDict := GetTranslations("pt-BR")
	if ptDict == nil {
		t.Fatalf("expected non-nil pt-BR dictionary")
	}
	if ptDict.PullRequestSummaryMarkdown.Title == "" {
		t.Fatalf("expected translated title in pt-BR")
	}

	// Fallback to en-US for unknown language
	unknownDict := GetTranslations("xx-YY")
	if unknownDict == nil {
		t.Fatalf("expected fallback dictionary for unknown locale")
	}
}

func TestInterpolate(t *testing.T) {
	tmpl := "Review failed for reason: {{errorMessage}}. Check {{dashboardUrl}}"
	params := map[string]string{
		"errorMessage": "Rate limit exceeded",
		"dashboardUrl": "https://app.scandrix.dev/reports",
	}

	res := Interpolate(tmpl, params)
	expected := "Review failed for reason: Rate limit exceeded. Check https://app.scandrix.dev/reports"
	if res != expected {
		t.Fatalf("expected %q, got %q", expected, res)
	}
}
