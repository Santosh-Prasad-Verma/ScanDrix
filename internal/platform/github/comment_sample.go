package github

import (
	"regexp"
	"sort"
	"strconv"
	"time"
)

var (
	prNumberURLRegex = regexp.MustCompile(`/(?:pulls|issues)/(\d+)(?:$|[/?#])`)
	prHTMLURLRegex   = regexp.MustCompile(`/pull/\d+`)
)

// RawReviewComment models a pull request review comment with diff placement.
type RawReviewComment struct {
	ID             int64     `json:"id"`
	PullRequestURL string    `json:"pull_request_url"`
	Path           string    `json:"path"`
	Body           string    `json:"body"`
	User           string    `json:"user"`
	CreatedAt      time.Time `json:"created_at"`
	Raw            any       `json:"raw,omitempty"`
}

// RawIssueComment models a discussion comment left on an issue or PR conversation tab.
type RawIssueComment struct {
	ID        int64     `json:"id"`
	IssueURL  string    `json:"issue_url"`
	HTMLURL   string    `json:"html_url"`
	Body      string    `json:"body"`
	User      string    `json:"user"`
	CreatedAt time.Time `json:"created_at"`
	Raw       any       `json:"raw,omitempty"`
}

// SampledPRFile identifies a modified file involved in a review comment.
type SampledPRFile struct {
	Filename string `json:"filename"`
}

// SampledPRReference encapsulates the pull request number.
type SampledPRReference struct {
	PullNumber int `json:"pull_number"`
}

// SampledPullRequestComments aggregates review and conversation comments per PR.
type SampledPullRequestComments struct {
	PR              SampledPRReference `json:"pr"`
	GeneralComments []RawIssueComment  `json:"generalComments"`
	ReviewComments  []RawReviewComment `json:"reviewComments"`
	Files           []SampledPRFile    `json:"files"`
}

// PullRequestNumberFromURL extracts the PR/issue integer identifier from an API URL.
func PullRequestNumberFromURL(url string) (int, bool) {
	if url == "" {
		return 0, false
	}
	matches := prNumberURLRegex.FindStringSubmatch(url)
	if len(matches) < 2 {
		return 0, false
	}
	num, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, false
	}
	return num, true
}

// IsPullRequestIssueComment verifies whether an issue comment belongs to a PR conversation
// (indicated by /pull/<n> in its html_url) rather than an unrelated standalone issue.
func IsPullRequestIssueComment(htmlURL string) bool {
	if htmlURL == "" {
		return false
	}
	return prHTMLURLRegex.MatchString(htmlURL)
}

// GroupCommentsByPullRequest organizes flat collections of review comments
// and PR conversation issue comments into cohesive buckets grouped by pull request.
func GroupCommentsByPullRequest(
	reviewComments []RawReviewComment,
	issueComments []RawIssueComment,
) []SampledPullRequestComments {
	byPR := make(map[int]*SampledPullRequestComments)

	getBucket := func(prNumber int) *SampledPullRequestComments {
		if entry, exists := byPR[prNumber]; exists {
			return entry
		}
		entry := &SampledPullRequestComments{
			PR:              SampledPRReference{PullNumber: prNumber},
			GeneralComments: make([]RawIssueComment, 0),
			ReviewComments:  make([]RawReviewComment, 0),
			Files:           make([]SampledPRFile, 0),
		}
		byPR[prNumber] = entry
		return entry
	}

	for _, rc := range reviewComments {
		prNum, ok := PullRequestNumberFromURL(rc.PullRequestURL)
		if !ok {
			continue
		}
		bucket := getBucket(prNum)
		bucket.ReviewComments = append(bucket.ReviewComments, rc)
		if rc.Path != "" {
			bucket.Files = append(bucket.Files, SampledPRFile{Filename: rc.Path})
		}
	}

	for _, ic := range issueComments {
		if !IsPullRequestIssueComment(ic.HTMLURL) {
			continue
		}
		prNum, ok := PullRequestNumberFromURL(ic.IssueURL)
		if !ok {
			continue
		}
		bucket := getBucket(prNum)
		bucket.GeneralComments = append(bucket.GeneralComments, ic)
	}

	prNumbers := make([]int, 0, len(byPR))
	for prNum := range byPR {
		prNumbers = append(prNumbers, prNum)
	}
	sort.Ints(prNumbers)

	result := make([]SampledPullRequestComments, 0, len(prNumbers))
	for _, prNum := range prNumbers {
		result = append(result, *byPR[prNum])
	}
	return result
}
