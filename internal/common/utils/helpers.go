package utils

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strings"
	"time"
)

// OwnerAndRepo holds separated repository owner and name.
type OwnerAndRepo struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
}

// RepositoryData represents normalized repository metadata.
type RepositoryData struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	FullName         string `json:"fullName"`
	Language         string `json:"language,omitempty"`
	DefaultBranch    string `json:"defaultBranch"`
	Platform         string `json:"platform"`
	OrganizationName string `json:"organizationName"`
}

// ExtractRepoNames extracts repository names from a slice of URLs or strings.
func ExtractRepoNames(urls []string) []string {
	if len(urls) == 0 {
		return []string{}
	}
	results := make([]string, 0, len(urls))
	for _, raw := range urls {
		trimmed := strings.TrimRight(raw, "/")
		if trimmed == "" {
			continue
		}
		parts := strings.Split(trimmed, "/")
		results = append(results, parts[len(parts)-1])
	}
	return results
}

// ExtractRepoName extracts the repository name from a slash-separated repository path.
func ExtractRepoName(repoPath string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(repoPath), "/")
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, "/")
	return strings.TrimSpace(parts[len(parts)-1])
}

// ExtractOwnerAndRepo parses a full repository name (e.g. "scandrix/backend") into owner and repo.
func ExtractOwnerAndRepo(repoFullName string) *OwnerAndRepo {
	trimmed := strings.TrimSpace(repoFullName)
	if trimmed == "" {
		return nil
	}
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return nil
	}
	return &OwnerAndRepo{
		Owner: parts[0],
		Repo:  parts[1],
	}
}

// ExtractRepoData looks up and extracts normalized repository metadata from a list.
func ExtractRepoData(repositories []RepositoryData, repoName string, platform string) *RepositoryData {
	for _, repo := range repositories {
		if repo.Name == repoName {
			res := repo
			if platform != "" {
				res.Platform = platform
			}
			if res.FullName == "" && res.OrganizationName != "" {
				res.FullName = fmt.Sprintf("%s/%s", res.OrganizationName, res.Name)
			}
			return &res
		}
	}
	return nil
}

// HoursDiff calculates the difference in hours between two timestamps.
func HoursDiff(initialDate, finalDate time.Time) float64 {
	return finalDate.Sub(initialDate).Hours()
}

// DateRange holds formatted start and end strings for reporting intervals.
type DateRange struct {
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
}

// GetWeekDate generates the start and end date of the current week (Sunday to Sunday).
func GetWeekDate(now time.Time) DateRange {
	weekday := int(now.Weekday())
	daysSinceSunday := weekday
	if daysSinceSunday == 0 {
		daysSinceSunday = 7
	}

	previousSunday := time.Date(now.Year(), now.Month(), now.Day()-daysSinceSunday, 0, 0, 0, 0, now.Location())
	yesterday := time.Date(now.Year(), now.Month(), now.Day()-1, 23, 59, 59, 0, now.Location())

	return DateRange{
		StartDate: previousSunday.Format("2006-01-02 15:04"),
		EndDate:   yesterday.Format("2006-01-02 15:04"),
	}
}

// GetPreviousWeekRange returns the date range for the previous 7 days.
func GetPreviousWeekRange(now time.Time) DateRange {
	start := now.AddDate(0, 0, -7)
	return DateRange{
		StartDate: start.Format("2006-01-02 15:04"),
		EndDate:   now.Format("2006-01-02 15:04"),
	}
}

// GetLast24HoursRange returns the date range for the past 24 hours.
func GetLast24HoursRange(now time.Time) DateRange {
	start := now.Add(-24 * time.Hour)
	return DateRange{
		StartDate: start.Format("2006-01-02 15:04"),
		EndDate:   now.Format("2006-01-02 15:04"),
	}
}

// ColumnItem describes a board or kanban column mapping.
type ColumnItem struct {
	ID     string `json:"id"`
	Column string `json:"column"`
}

// WorkItem describes a work item with an assigned column ID.
type WorkItem struct {
	ID       string `json:"id"`
	ColumnID string `json:"columnId"`
	Title    string `json:"title"`
}

// FilterByColumn filters raw work items based on matching column classifications (e.g. todo, wip, done).
func FilterByColumn(items []WorkItem, columns []ColumnItem, allowedColumns []string) []WorkItem {
	colMap := make(map[string]string, len(columns))
	for _, col := range columns {
		colMap[col.ID] = col.Column
	}

	allowed := make(map[string]struct{}, len(allowedColumns))
	for _, a := range allowedColumns {
		allowed[a] = struct{}{}
	}

	var filtered []WorkItem
	for _, item := range items {
		cType, ok := colMap[item.ColumnID]
		if ok {
			if _, match := allowed[cType]; match {
				filtered = append(filtered, item)
			}
		}
	}
	return filtered
}

// ParseJSON parses a JSON string into a generic map, returning an empty map on syntax errors.
func ParseJSON(value string) map[string]any {
	result := make(map[string]any)
	if strings.TrimSpace(value) == "" {
		return result
	}
	_ = json.Unmarshal([]byte(value), &result)
	return result
}

// ShouldProcessNotBugItems returns true if the work item type should be skipped as a known bug/error type.
func ShouldProcessNotBugItems(workItemType string, bugTypes []string) bool {
	lower := strings.ToLower(strings.TrimSpace(workItemType))
	if lower == "error" {
		return true
	}
	for _, bt := range bugTypes {
		if strings.ToLower(strings.TrimSpace(bt)) == lower {
			return true
		}
	}
	return false
}

// SanitizeString strips double quotes and backslashes from a string.
func SanitizeString(str string) string {
	if str == "" {
		return str
	}
	r := strings.NewReplacer("\"", "", "\\", "")
	return r.Replace(str)
}

const charsetAlphanumeric = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// RandomString generates a cryptographically secure random alphanumeric string of the specified length.
func RandomString(length int) string {
	if length <= 0 {
		return ""
	}
	b := make([]byte, length)
	charsetLen := big.NewInt(int64(len(charsetAlphanumeric)))
	for i := 0; i < length; i++ {
		num, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			b[i] = charsetAlphanumeric[i%len(charsetAlphanumeric)]
		} else {
			b[i] = charsetAlphanumeric[num.Int64()]
		}
	}
	return string(b)
}

// GenerateRandomOrgName creates a slugified organization name with random suffix.
func GenerateRandomOrgName(name string) string {
	cleanName := regexp.MustCompile(`[^\w-]`).ReplaceAllString(name, "")
	orgName := fmt.Sprintf("%s-%s", cleanName, RandomString(16))
	if len(orgName) > 50 {
		orgName = orgName[:50]
	}
	return orgName
}

// RetryWithBackoff executes an operation with exponential backoff on retriable errors.
func RetryWithBackoff(ctx context.Context, fn func() error, retries int, initialDelay time.Duration) error {
	var lastErr error
	delay := initialDelay

	for attempt := 0; attempt <= retries; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err

		if attempt < retries {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
			delay = time.Duration(float64(delay) * math.Pow(2, 1))
		}
	}
	return fmt.Errorf("operation failed after %d retries: %w", retries, lastErr)
}

// HunkSplit represents separated new and old hunk strings from a unified patch.
type HunkSplit struct {
	NewHunk string `json:"newHunk"`
	OldHunk string `json:"oldHunk"`
}

// ParseHunksGit splits a unified diff patch into new and old hunk lines.
func ParseHunksGit(patch string) HunkSplit {
	lines := strings.Split(patch, "\n")
	var newHunk strings.Builder
	var oldHunk strings.Builder

	for _, line := range lines {
		if strings.HasPrefix(line, "@@") {
			newHunk.WriteString(line + "\n")
			oldHunk.WriteString(line + "\n")
		} else if strings.HasPrefix(line, "+") {
			newHunk.WriteString(line + "\n")
		} else if strings.HasPrefix(line, "-") {
			oldHunk.WriteString(line + "\n")
		} else {
			newHunk.WriteString(line + "\n")
			oldHunk.WriteString(line + "\n")
		}
	}

	return HunkSplit{
		NewHunk: newHunk.String(),
		OldHunk: oldHunk.String(),
	}
}

var (
	contextualInfoRe = regexp.MustCompile(`Contextual information.*?:.*?\d{4}-\d{2}-\d{2} \d{2}:\d{2}`)
	dataAnalystRe    = regexp.MustCompile(`(?s)###Data Analyst Tool Response.*?### Files`)
	codebaseDataRe   = regexp.MustCompile(`(?s)Code Base data:.*$`)
	pullRequestsRe   = regexp.MustCompile(`(?s)Pull Requests:.*$`)
	multiSpacesRe    = regexp.MustCompile(`\s+`)
)

// CleanHumanMessage strips synthetic prompt artifacts, system tool responses, and boilerplate from review messages.
func CleanHumanMessage(content string) string {
	if content == "" {
		return ""
	}
	s := strings.ReplaceAll(content, "\\n", " ")
	s = contextualInfoRe.ReplaceAllString(s, "")
	s = dataAnalystRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "Error executing ConversationCodeBaseTool. Please try again.", "")
	s = codebaseDataRe.ReplaceAllString(s, "")
	s = pullRequestsRe.ReplaceAllString(s, "")
	s = multiSpacesRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// Sleep pauses execution for the given milliseconds or until context cancellation.
func Sleep(ctx context.Context, ms int) error {
	select {
	case <-time.After(time.Duration(ms) * time.Millisecond):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ExtractOrganizationID extracts the organization UUID from map or struct inputs.
func ExtractOrganizationID(data map[string]any) string {
	if data == nil {
		return ""
	}
	if orgData, ok := data["organizationAndTeamData"].(map[string]any); ok {
		if id, ok := orgData["organizationId"].(string); ok {
			return id
		}
	}
	if id, ok := data["organizationId"].(string); ok {
		return id
	}
	return ""
}
