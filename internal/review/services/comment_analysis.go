// Package services provides production-grade infrastructure implementations for ScanDrix code review.
package services

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
)

// CommentIntent classifies developer responses to ScanDrix suggestions.
type CommentIntent string

const (
	IntentAgreement           CommentIntent = "AGREEMENT"
	IntentDisagreement        CommentIntent = "DISAGREEMENT"
	IntentQuestion            CommentIntent = "QUESTION"
	IntentAlreadyFixed        CommentIntent = "ALREADY_FIXED"
	IntentFalsePositiveReport CommentIntent = "FALSE_POSITIVE_REPORT"
	IntentIrrelevant          CommentIntent = "IRRELEVANT"
)

// CommentSentiment describes developer tone.
type CommentSentiment string

const (
	SentimentPositive CommentSentiment = "POSITIVE"
	SentimentNeutral  CommentSentiment = "NEUTRAL"
	SentimentNegative CommentSentiment = "NEGATIVE"
)

// DeveloperCommentAnalysis records the classified intent and actionable followup for a PR reply.
type DeveloperCommentAnalysis struct {
	CommentID         string           `json:"comment_id"`
	PullNumber        int              `json:"pull_number"`
	Author            string           `json:"author"`
	Body              string           `json:"body"`
	TargetSuggestionID string          `json:"target_suggestion_id,omitempty"`
	Intent            CommentIntent    `json:"intent"`
	Sentiment         CommentSentiment `json:"sentiment"`
	Confidence        float64          `json:"confidence"`
	AutomatedReply    string           `json:"automated_reply,omitempty"`
	ShouldResolve     bool             `json:"should_resolve"`
	ShouldMuteRule    bool             `json:"should_mute_rule"`
	ExtractedRuleDesc string           `json:"extracted_rule_desc,omitempty"`
	CreatedAt         time.Time        `json:"created_at"`
}

// RuleCandidate represents a custom team rule synthesized from developer discussions.
type RuleCandidate struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Severity    string   `json:"severity"`
	FileGlobs   []string `json:"file_globs"`
	SampleFix   string   `json:"sample_fix,omitempty"`
}

// CommentAnalysisEngine processes developer replies to ScanDrix comments.
type CommentAnalysisEngine struct {
	mu           sync.RWMutex
	intentRegex  map[CommentIntent][]*regexp.Regexp
	ruleKeywords []*regexp.Regexp
}

// NewCommentAnalysisEngine initializes the analysis engine.
func NewCommentAnalysisEngine() *CommentAnalysisEngine {
	eng := &CommentAnalysisEngine{
		intentRegex:  make(map[CommentIntent][]*regexp.Regexp),
		ruleKeywords: make([]*regexp.Regexp, 0),
	}
	eng.initRegexes()
	return eng
}

func (e *CommentAnalysisEngine) initRegexes() {
	// Agreement patterns
	e.intentRegex[IntentAgreement] = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(good catch|thanks|fixed|will fix|applied|done|updated|makes sense|agreed|merged)\b`),
		regexp.MustCompile(`(?i)\b(nice catch|thank you|good point|committed)\b`),
	}

	// Disagreement patterns
	e.intentRegex[IntentDisagreement] = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(intentional|by design|won't fix|wontfix|not needed|disagree|works as intended|leave it)\b`),
		regexp.MustCompile(`(?i)\b(don't change|keep as is|not an issue|ignore this)\b`),
	}

	// Question patterns
	e.intentRegex[IntentQuestion] = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(why|how should|can you explain|what if|how do i|why is this|could you clarify)\b`),
		regexp.MustCompile(`\?`),
	}

	// Already fixed patterns
	e.intentRegex[IntentAlreadyFixed] = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(already fixed|fixed in commit|addressed in|see commit|fixed above)\b`),
	}

	// False positive patterns
	e.intentRegex[IntentFalsePositiveReport] = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(false positive|not a bug|hallucination|incorrect|doesn't apply here|wrong suggestion)\b`),
	}

	// Rule extraction keywords
	e.ruleKeywords = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\balways\s+(use|add|check|wrap|call|prefer|write|require|enforce)\b`),
		regexp.MustCompile(`(?i)\bnever\s+(use|allow|expose|commit|ignore)\b`),
		regexp.MustCompile(`(?i)\bour standard is to\b`),
		regexp.MustCompile(`(?i)\bin (?:our codebase|this project|our repo)\s+we\b`),
	}
}

// ClassifyDeveloperComment analyzes developer text to categorize their response.
func (e *CommentAnalysisEngine) ClassifyDeveloperComment(
	commentID string,
	pullNumber int,
	author string,
	body string,
	targetSuggestion *domain.CodeSuggestion,
) DeveloperCommentAnalysis {
	cleanBody := strings.TrimSpace(body)
	cleanLower := strings.ToLower(cleanBody)

	intent := IntentIrrelevant
	confidence := 0.6
	sentiment := SentimentNeutral

	// Check False Positive first (highest precedence for review feedback)
	for _, re := range e.intentRegex[IntentFalsePositiveReport] {
		if re.MatchString(cleanLower) {
			intent = IntentFalsePositiveReport
			confidence = 0.95
			sentiment = SentimentNegative
			break
		}
	}

	// Check Already Fixed
	if intent == IntentIrrelevant {
		for _, re := range e.intentRegex[IntentAlreadyFixed] {
			if re.MatchString(cleanLower) {
				intent = IntentAlreadyFixed
				confidence = 0.9
				sentiment = SentimentPositive
				break
			}
		}
	}

	// Check Agreement
	if intent == IntentIrrelevant {
		for _, re := range e.intentRegex[IntentAgreement] {
			if re.MatchString(cleanLower) {
				intent = IntentAgreement
				confidence = 0.85
				sentiment = SentimentPositive
				break
			}
		}
	}

	// Check Disagreement
	if intent == IntentIrrelevant {
		for _, re := range e.intentRegex[IntentDisagreement] {
			if re.MatchString(cleanLower) {
				intent = IntentDisagreement
				confidence = 0.85
				sentiment = SentimentNegative
				break
			}
		}
	}

	// Check Question
	if intent == IntentIrrelevant {
		for _, re := range e.intentRegex[IntentQuestion] {
			if re.MatchString(cleanLower) {
				intent = IntentQuestion
				confidence = 0.8
				sentiment = SentimentNeutral
				break
			}
		}
	}

	sugID := ""
	if targetSuggestion != nil {
		sugID = targetSuggestion.ID.String()
	}

	analysis := DeveloperCommentAnalysis{
		CommentID:          commentID,
		PullNumber:         pullNumber,
		Author:             author,
		Body:               cleanBody,
		TargetSuggestionID: sugID,
		Intent:             intent,
		Sentiment:          sentiment,
		Confidence:         confidence,
		CreatedAt:          time.Now().UTC(),
	}

	// Generate automated follow-up responses and actions
	switch intent {
	case IntentAgreement, IntentAlreadyFixed:
		analysis.ShouldResolve = true
		analysis.AutomatedReply = fmt.Sprintf("Great! Resolving this suggestion for @%s.", author)
	case IntentDisagreement:
		analysis.ShouldResolve = true
		analysis.AutomatedReply = fmt.Sprintf("Acknowledged. Closing suggestion as per @%s's review preference.", author)
	case IntentFalsePositiveReport:
		analysis.ShouldResolve = true
		analysis.ShouldMuteRule = true
		analysis.AutomatedReply = fmt.Sprintf("Thank you for the feedback, @%s. ScanDrix has logged this report to suppress similar false alarms in this repository.", author)
	case IntentQuestion:
		if targetSuggestion != nil {
			analysis.AutomatedReply = fmt.Sprintf(
				"Hey @%s, ScanDrix flagged this because `%s`. The recommended resolution is:\n```suggestion\n%s\n```\nLet us know if you have further questions!",
				author,
				targetSuggestion.GetExplanation(),
				targetSuggestion.GetSuggestedReplacement(),
			)
		}
	}

	// Check if developer comment expresses a team coding guideline
	for _, re := range e.ruleKeywords {
		if loc := re.FindStringIndex(cleanBody); loc != nil {
			analysis.ExtractedRuleDesc = cleanBody
			break
		}
	}

	return analysis
}

// ExtractRuleCandidates parses developer discussions for implicit organizational conventions.
func (e *CommentAnalysisEngine) ExtractRuleCandidates(comments []DeveloperCommentAnalysis) []RuleCandidate {
	var rules []RuleCandidate

	for _, c := range comments {
		if c.ExtractedRuleDesc == "" {
			continue
		}

		desc := strings.TrimSpace(c.ExtractedRuleDesc)
		title := truncateString(desc, 50)

		rules = append(rules, RuleCandidate{
			ID:          uuid.New().String(),
			Title:       "Team Convention: " + title,
			Description: desc,
			Severity:    "MAJOR",
			FileGlobs:   []string{"**/*"},
		})
	}

	return rules
}

// BotFilter manages bot account detection and automated message filtering.
type BotFilter struct {
	knownBotNames  map[string]bool
	bodySignatures []*regexp.Regexp
}

// NewBotFilter creates a bot filter initialized with common SCM bots and automated CI engines.
func NewBotFilter() *BotFilter {
	bots := []string{
		"dependabot", "dependabot[bot]", "renovate", "renovate[bot]",
		"github-actions", "github-actions[bot]", "gitlab-bot", "codecov",
		"codecov[bot]", "sonarcloud", "sonarcloud[bot]", "snyk-bot",
		"greenkeeper[bot]", "mergify[bot]", "scandrix[bot]",
		"drixy[bot]", "vercel[bot]", "netlify[bot]",
	}

	known := make(map[string]bool)
	for _, b := range bots {
		known[strings.ToLower(b)] = true
	}

	signatures := []*regexp.Regexp{
		regexp.MustCompile(`(?i)<!--\s*(?:scandrix|drixy|dependabot|renovate|codecov)\b`),
		regexp.MustCompile(`(?i)\bgenerated by\s+(?:github actions|gitlab ci|codecov|sonar)\b`),
		regexp.MustCompile(`(?i)\bcoverage (?:increased|decreased|remains unchanged)\b`),
		regexp.MustCompile(`(?i)\bno conflicts found\b`),
		regexp.MustCompile(`(?i)\bthis pull request was created automatically\b`),
	}

	return &BotFilter{
		knownBotNames:  known,
		bodySignatures: signatures,
	}
}

// IsBotComment checks if the author or comment content matches an automated bot.
func (b *BotFilter) IsBotComment(author, body string) bool {
	authorLower := strings.ToLower(strings.TrimSpace(author))
	if b.knownBotNames[authorLower] || strings.HasSuffix(authorLower, "[bot]") {
		return true
	}

	for _, sig := range b.bodySignatures {
		if sig.MatchString(body) {
			return true
		}
	}

	return false
}

// ReviewerFilter manages author exclusion and inclusion lists.
type ReviewerFilter struct {
	denylist  map[string]bool
	allowlist map[string]bool
}

// NewReviewerFilter creates an author filter.
func NewReviewerFilter(denylist, allowlist []string) *ReviewerFilter {
	deny := make(map[string]bool)
	for _, d := range denylist {
		deny[strings.ToLower(strings.TrimSpace(d))] = true
	}

	allow := make(map[string]bool)
	for _, a := range allowlist {
		allow[strings.ToLower(strings.TrimSpace(a))] = true
	}

	return &ReviewerFilter{
		denylist:  deny,
		allowlist: allow,
	}
}

// IsExcluded evaluates whether a given author should be skipped from comment analysis.
func (rf *ReviewerFilter) IsExcluded(author string) bool {
	authorLower := strings.ToLower(strings.TrimSpace(author))

	// If allowlist is populated, author must be in allowlist
	if len(rf.allowlist) > 0 {
		return !rf.allowlist[authorLower]
	}

	// Otherwise check denylist
	return rf.denylist[authorLower]
}

// CommentCategorizationParams defines input arguments for batch comment processing.
type CommentCategorizationParams struct {
	Comments          []SCMReviewComment
	Denylist          []string
	Allowlist         []string
	TargetSuggestions map[string]*domain.CodeSuggestion
}

// CategorizeComments processes a batch of pull request comments, filtering bots and extracting rules.
func (e *CommentAnalysisEngine) CategorizeComments(params CommentCategorizationParams) ([]DeveloperCommentAnalysis, []RuleCandidate) {
	botFilter := NewBotFilter()
	reviewerFilter := NewReviewerFilter(params.Denylist, params.Allowlist)

	var analyses []DeveloperCommentAnalysis

	for _, c := range params.Comments {
		// Filter bot comments
		author := c.Anchor.OriginalCommit // or metadata author
		if author == "" {
			author = "developer"
		}

		if botFilter.IsBotComment(author, c.Body) {
			continue
		}

		// Filter excluded reviewers
		if reviewerFilter.IsExcluded(author) {
			continue
		}

		var targetSug *domain.CodeSuggestion
		if c.SuggestionID != "" && params.TargetSuggestions != nil {
			targetSug = params.TargetSuggestions[c.SuggestionID]
		}

		analysis := e.ClassifyDeveloperComment(c.ID, 0, author, c.Body, targetSug)
		analyses = append(analyses, analysis)
	}

	ruleCandidates := e.ExtractRuleCandidates(analyses)
	synthesizedRules := e.SynthesizeCustomDrixyRules(ruleCandidates)

	return analyses, synthesizedRules
}

// SynthesizeCustomDrixyRules groups similar rule conventions into canonical Drixy rules.
func (e *CommentAnalysisEngine) SynthesizeCustomDrixyRules(candidates []RuleCandidate) []RuleCandidate {
	if len(candidates) <= 1 {
		return candidates
	}

	seen := make(map[string]bool)
	var unique []RuleCandidate

	for _, c := range candidates {
		normalized := strings.ToLower(strings.TrimSpace(c.Description))
		if !seen[normalized] {
			seen[normalized] = true
			unique = append(unique, c)
		}
	}

	return unique
}

