package taskcontext

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	issueKeyRegex    = regexp.MustCompile(`\b([A-Z][A-Z0-9]+-\d+)\b`)
	issueNumberRegex = regexp.MustCompile(`(?i)(?:#|\bissue\s*#?\s*|\bissues\s*#?\s*)(\d+)\b`)
	urlRegex         = regexp.MustCompile(`(?i)https?://[^\s)]+`)
	ariRegex         = regexp.MustCompile(`(?i)\bari:[^\s,]+`)
	strictKeyRegex   = regexp.MustCompile(`^[A-Z][A-Z0-9]+-\d+$`)
)

func normalizeLikelyURL(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimLeft(trimmed, "(\"'`<")
	trimmed = strings.TrimRight(trimmed, ")]'\",.;:!?")
	return trimmed
}

// ExtractIssueNumbers finds all numeric issue references like #123, issue 45.
func ExtractIssueNumbers(text string) []int {
	seen := make(map[int]struct{})
	var result []int

	matches := issueNumberRegex.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		if len(m) > 1 {
			if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
				if _, exists := seen[n]; !exists {
					seen[n] = struct{}{}
					result = append(result, n)
				}
			}
		}
	}
	return result
}

// ExtractIssueKeys finds Jira/Linear-style keys like PROJ-123.
func ExtractIssueKeys(text string) []string {
	seen := make(map[string]struct{})
	var result []string

	matches := issueKeyRegex.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		if len(m) > 1 {
			key := strings.ToUpper(m[1])
			if _, exists := seen[key]; !exists {
				seen[key] = struct{}{}
				result = append(result, key)
			}
		}
	}
	return result
}

// ExtractLinks extracts clean HTTP/HTTPS links from text.
func ExtractLinks(text string) []string {
	var links []string
	matches := urlRegex.FindAllString(text, -1)
	for _, m := range matches {
		normalized := normalizeLikelyURL(m)
		if normalized != "" && IsLikelyURL(normalized) {
			links = append(links, normalized)
		}
	}
	return UniqueNonEmpty(links)
}

// ExtractAris extracts Atlassian Resource Identifiers (ari:...) from text.
func ExtractAris(text string) []string {
	seen := make(map[string]struct{})
	var result []string

	matches := ariRegex.FindAllString(text, -1)
	for _, m := range matches {
		if _, exists := seen[m]; !exists {
			seen[m] = struct{}{}
			result = append(result, m)
		}
	}
	return result
}

// IsLikelyIssueKey checks whether a string is a standard project key.
func IsLikelyIssueKey(value string) bool {
	return strictKeyRegex.MatchString(value)
}

// IsLikelyURL checks whether a string begins with http:// or https://.
func IsLikelyURL(value string) bool {
	lower := strings.ToLower(value)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// IsLikelyTaskReferenceUrl checks if a URL points to a known task tracker or issue path.
func IsLikelyTaskReferenceUrl(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}

	host := strings.ToLower(parsed.Hostname())
	path := strings.ToLower(parsed.Path)
	query := strings.ToLower(parsed.RawQuery)

	knownTaskHosts := []string{
		"atlassian.net",
		"linear.app",
		"notion.so",
		"notion.site",
		"clickup.com",
		"trello.com",
		"asana.com",
		"dev.azure.com",
		"youtrack",
		"shortcut.com",
		"monday.com",
	}

	for _, h := range knownTaskHosts {
		if strings.Contains(host, h) {
			return true
		}
	}

	return strings.Contains(path, "/browse/") ||
		strings.Contains(path, "/issue/") ||
		strings.Contains(path, "/issues/") ||
		strings.Contains(path, "/ticket/") ||
		strings.Contains(path, "/tickets/") ||
		strings.Contains(path, "/task/") ||
		strings.Contains(path, "/tasks/") ||
		strings.Contains(path, "/work-items/") ||
		strings.HasPrefix(path, "/t/") ||
		strings.Contains(query, "selectedissue=")
}
