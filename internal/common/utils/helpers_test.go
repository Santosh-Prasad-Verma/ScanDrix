package utils

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestExtractRepoNames(t *testing.T) {
	urls := []string{
		"https://github.com/scandrix/backend",
		"https://gitlab.com/group/subgroup/frontend/",
		"local-repo",
	}
	names := ExtractRepoNames(urls)
	expected := []string{"backend", "frontend", "local-repo"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d names, got %d", len(expected), len(names))
	}
	for i, exp := range expected {
		if names[i] != exp {
			t.Errorf("expected %s, got %s", exp, names[i])
		}
	}

	if len(ExtractRepoNames(nil)) != 0 {
		t.Errorf("expected empty slice for nil")
	}
}

func TestExtractRepoName(t *testing.T) {
	if ExtractRepoName("scandrix/backend") != "backend" {
		t.Errorf("failed simple repo extract")
	}
	if ExtractRepoName("gitlab.com/org/sub/service/") != "service" {
		t.Errorf("failed nested repo extract")
	}
	if ExtractRepoName("") != "" {
		t.Errorf("failed empty extract")
	}
}

func TestExtractOwnerAndRepo(t *testing.T) {
	res := ExtractOwnerAndRepo("scandrix/backend")
	if res == nil || res.Owner != "scandrix" || res.Repo != "backend" {
		t.Fatalf("failed owner repo parse: %+v", res)
	}

	if ExtractOwnerAndRepo("invalid") != nil {
		t.Errorf("expected nil for single token")
	}
	if ExtractOwnerAndRepo("") != nil {
		t.Errorf("expected nil for empty string")
	}
}

func TestExtractRepoData(t *testing.T) {
	repos := []RepositoryData{
		{
			ID:               "1",
			Name:             "backend",
			OrganizationName: "scandrix",
			DefaultBranch:    "main",
		},
	}
	found := ExtractRepoData(repos, "backend", "github")
	if found == nil || found.Platform != "github" || found.FullName != "scandrix/backend" {
		t.Fatalf("failed extract repo data: %+v", found)
	}
	if ExtractRepoData(repos, "frontend", "") != nil {
		t.Errorf("expected nil for missing repo")
	}
}

func TestHoursDiff(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 1, 1, 15, 30, 0, 0, time.UTC)
	diff := HoursDiff(t1, t2)
	if diff != 3.5 {
		t.Errorf("expected 3.5 hours, got %f", diff)
	}
}

func TestDateRanges(t *testing.T) {
	now := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
	wk := GetWeekDate(now)
	if wk.StartDate == "" || wk.EndDate == "" {
		t.Errorf("invalid week date range: %+v", wk)
	}

	prev := GetPreviousWeekRange(now)
	if prev.StartDate == "" || prev.EndDate == "" {
		t.Errorf("invalid previous week range: %+v", prev)
	}

	h24 := GetLast24HoursRange(now)
	if h24.StartDate == "" || h24.EndDate == "" {
		t.Errorf("invalid 24h range: %+v", h24)
	}
}

func TestFilterByColumn(t *testing.T) {
	items := []WorkItem{
		{ID: "1", ColumnID: "col-todo", Title: "Task 1"},
		{ID: "2", ColumnID: "col-wip", Title: "Task 2"},
		{ID: "3", ColumnID: "col-done", Title: "Task 3"},
	}
	cols := []ColumnItem{
		{ID: "col-todo", Column: "todo"},
		{ID: "col-wip", Column: "wip"},
		{ID: "col-done", Column: "done"},
	}

	res := FilterByColumn(items, cols, []string{"wip", "done"})
	if len(res) != 2 {
		t.Fatalf("expected 2 filtered items, got %d", len(res))
	}
	if res[0].ID != "2" || res[1].ID != "3" {
		t.Errorf("unexpected filtered items: %+v", res)
	}
}

func TestParseJSON(t *testing.T) {
	m := ParseJSON(`{"key": "value", "num": 42}`)
	if m["key"] != "value" {
		t.Errorf("failed parse json valid")
	}
	bad := ParseJSON(`invalid-json`)
	if len(bad) != 0 {
		t.Errorf("expected empty map for invalid json")
	}
}

func TestShouldProcessNotBugItems(t *testing.T) {
	if !ShouldProcessNotBugItems("error", []string{"defect"}) {
		t.Errorf("expected true for error")
	}
	if !ShouldProcessNotBugItems("defect", []string{"defect"}) {
		t.Errorf("expected true for defect")
	}
	if ShouldProcessNotBugItems("feature", []string{"defect"}) {
		t.Errorf("expected false for feature")
	}
}

func TestSanitizeString(t *testing.T) {
	s := `hello "world" \ test`
	cleaned := SanitizeString(s)
	if cleaned != "hello world  test" {
		t.Errorf("failed sanitization: %s", cleaned)
	}
}

func TestRandomStringAndOrgName(t *testing.T) {
	str1 := RandomString(16)
	str2 := RandomString(16)
	if len(str1) != 16 || len(str2) != 16 {
		t.Fatalf("expected length 16")
	}
	if str1 == str2 {
		t.Errorf("expected random strings to differ")
	}

	org := GenerateRandomOrgName("Acme Corp!")
	if !strings.HasPrefix(org, "AcmeCorp-") {
		t.Errorf("unexpected org name format: %s", org)
	}
	if len(org) > 50 {
		t.Errorf("org name too long: %s", org)
	}
}

func TestRetryWithBackoff(t *testing.T) {
	ctx := context.Background()
	attempts := 0
	err := RetryWithBackoff(ctx, func() error {
		attempts++
		if attempts < 3 {
			return errors.New("transient error")
		}
		return nil
	}, 3, 5*time.Millisecond)

	if err != nil {
		t.Fatalf("expected retry success: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}

	// Test exhausting retries
	attempts = 0
	err = RetryWithBackoff(ctx, func() error {
		attempts++
		return errors.New("permanent failure")
	}, 2, 5*time.Millisecond)

	if err == nil {
		t.Fatalf("expected error after exhausting retries")
	}
}

func TestParseHunksGit(t *testing.T) {
	patch := `@@ -1,3 +1,4 @@
 line1
-line2
+line2_mod
+line2_add
 line3`

	res := ParseHunksGit(patch)
	if !strings.Contains(res.NewHunk, "line2_mod") || !strings.Contains(res.NewHunk, "line2_add") {
		t.Errorf("missing new hunk additions: %s", res.NewHunk)
	}
	if strings.Contains(res.NewHunk, "-line2") {
		t.Errorf("new hunk contains removed line")
	}
	if !strings.Contains(res.OldHunk, "-line2") {
		t.Errorf("old hunk missing deleted line: %s", res.OldHunk)
	}
}

func TestCleanHumanMessage(t *testing.T) {
	raw := `Contextual information: 2026-09-19 12:00
###Data Analyst Tool Response
Some tool output
### Files
Review message body
Code Base data: extra stuff`

	cleaned := CleanHumanMessage(raw)
	if strings.Contains(cleaned, "Contextual information") || strings.Contains(cleaned, "Data Analyst") {
		t.Errorf("failed cleaning human message: %s", cleaned)
	}
	if !strings.Contains(cleaned, "Review message body") {
		t.Errorf("lost message body: %s", cleaned)
	}
}

func TestExtractOrganizationID(t *testing.T) {
	m1 := map[string]any{
		"organizationAndTeamData": map[string]any{
			"organizationId": "org-1",
		},
	}
	if ExtractOrganizationID(m1) != "org-1" {
		t.Errorf("failed nested org id extract")
	}

	m2 := map[string]any{"organizationId": "org-2"}
	if ExtractOrganizationID(m2) != "org-2" {
		t.Errorf("failed top level org id extract")
	}
}
