// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package rulesengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// PastReviewComment represents a developer review comment from historical PR discussions.
type PastReviewComment struct {
	ID          string    `json:"id"`
	PRNumber    int       `json:"pr_number"`
	Author      string    `json:"author"`
	FilePath    string    `json:"file_path"`
	LineNumber  int       `json:"line_number"`
	Body        string    `json:"body"`
	CodeSnippet string    `json:"code_snippet,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// RuleCandidateCluster groups related historical comments addressing the same underlying pattern.
type RuleCandidateCluster struct {
	ClusterKey      string              `json:"cluster_key"`
	Topic           string              `json:"topic"`
	Comments        []PastReviewComment `json:"comments"`
	SamplePaths     []string            `json:"sample_paths"`
	SuggestedTitle  string              `json:"suggested_title"`
	SuggestedLevel  RFC2119Level        `json:"suggested_level"`
	SuggestedScope  DrixyRuleScope      `json:"suggested_scope"`
	InferredSeverity models.FindingSeverity `json:"inferred_severity"`
}

// RuleSynthesisRequest configures automated rule mining from PR history.
type RuleSynthesisRequest struct {
	OrgID            string               `json:"org_id"`
	RepoID           string               `json:"repo_id"`
	MinClusterSize   int                  `json:"min_cluster_size"`
	HistoricalComments []PastReviewComment `json:"historical_comments"`
}

// SynthesizedRuleDefinition represents an AI-mined custom review rule.
type SynthesizedRuleDefinition struct {
	ID               uuid.UUID              `json:"id"`
	OrgID            string                 `json:"org_id"`
	RepoID           string                 `json:"repo_id"`
	Name             string                 `json:"name"`
	Description      string                 `json:"description"`
	Scope            DrixyRuleScope         `json:"scope"`
	Severity         models.FindingSeverity `json:"severity"`
	NormativeLevel   RFC2119Level           `json:"normative_level"`
	PathGlobs        []string               `json:"path_globs"`
	DetectorPattern  string                 `json:"detector_pattern"`
	NegativePattern  string                 `json:"negative_pattern,omitempty"`
	Remediation      string                 `json:"remediation"`
	CorrectExample   string                 `json:"correct_example"`
	IncorrectExample string                 `json:"incorrect_example"`
	SupportCount     int                    `json:"support_count"`
	ConfidenceScore  float64                `json:"confidence_score"` // 0.0 to 1.0
	CreatedAt        time.Time              `json:"created_at"`
}

// TopicDefinition pairs canonical topic names, severities, and trigger keywords.
type TopicDefinition struct {
	Topic    string
	Severity models.FindingSeverity
	Keywords []string
}

// DrixyRuleGenerator automatically synthesizes custom review rules from recurring team code review advice.
type DrixyRuleGenerator struct {
	orderedTopics []TopicDefinition
}

// NewDrixyRuleGenerator constructs a rule generator.
func NewDrixyRuleGenerator() *DrixyRuleGenerator {
	return &DrixyRuleGenerator{
		orderedTopics: []TopicDefinition{
			{
				Topic:    "sql_injection",
				Severity: models.SeverityCritical,
				Keywords: []string{"sql injection", "raw query", "sqli"},
			},
			{
				Topic:    "hardcoded_secrets",
				Severity: models.SeverityCritical,
				Keywords: []string{"hardcoded", "token leak", "secret key", "api key", "secret", "bearer"},
			},
			{
				Topic:    "concurrency_race",
				Severity: models.SeverityHigh,
				Keywords: []string{"data race", "mutex", "goroutine leak"},
			},
			{
				Topic:    "null_pointer",
				Severity: models.SeverityHigh,
				Keywords: []string{"nil pointer", "null deref"},
			},
			{
				Topic:    "memory_leak",
				Severity: models.SeverityHigh,
				Keywords: []string{"memory leak"},
			},
			{
				Topic:    "context_timeout",
				Severity: models.SeverityHigh,
				Keywords: []string{"context timeout", "timeout", "http.get", "http.post"},
			},
			{
				Topic:    "error_handling",
				Severity: models.SeverityHigh,
				Keywords: []string{"handle err", "unhandled error", "error"},
			},
			{
				Topic:    "database_performance",
				Severity: models.SeverityMedium,
				Keywords: []string{"n+1", "unindexed"},
			},
			{
				Topic:    "production_logging",
				Severity: models.SeverityMedium,
				Keywords: []string{"fmt.print", "console.log", "production logging"},
			},
			{
				Topic:    "deprecation",
				Severity: models.SeverityMedium,
				Keywords: []string{"deprecated"},
			},
			{
				Topic:    "code_style",
				Severity: models.SeverityLow,
				Keywords: []string{"magic number", "naming", "formatting"},
			},
		},
	}
}

// GenerateRules mines historical comments and clusters repeated advice into actionable Drixy rules.
func (g *DrixyRuleGenerator) GenerateRules(
	ctx context.Context,
	req RuleSynthesisRequest,
) ([]SynthesizedRuleDefinition, error) {
	if len(req.HistoricalComments) == 0 {
		return nil, nil
	}

	minCluster := req.MinClusterSize
	if minCluster <= 0 {
		minCluster = 2
	}

	// 1. Cluster historical comments by semantic topic and keyword signatures
	clusters := g.clusterHistoricalComments(req.HistoricalComments)

	// 2. Synthesize concrete rule definitions from clusters meeting support threshold
	var rules []SynthesizedRuleDefinition
	for _, cl := range clusters {
		if len(cl.Comments) < minCluster {
			continue
		}

		rule := g.synthesizeRuleFromCluster(req.OrgID, req.RepoID, cl)
		rules = append(rules, rule)
	}

	// Sort rules by support count descending, then confidence descending
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].SupportCount == rules[j].SupportCount {
			return rules[i].ConfidenceScore > rules[j].ConfidenceScore
		}
		return rules[i].SupportCount > rules[j].SupportCount
	})

	return rules, nil
}

func (g *DrixyRuleGenerator) clusterHistoricalComments(comments []PastReviewComment) []*RuleCandidateCluster {
	clusterMap := make(map[string]*RuleCandidateCluster)

	for _, c := range comments {
		lowerBody := strings.ToLower(c.Body)
		matchedTopic := "general_code_quality"
		severity := models.SeverityMedium

		// Deterministic ordered keyword-based topic classification
	searchLoop:
		for _, td := range g.orderedTopics {
			for _, kw := range td.Keywords {
				if strings.Contains(lowerBody, kw) {
					matchedTopic = td.Topic
					severity = td.Severity
					break searchLoop
				}
			}
		}

		// Fallback: If comment references specific code construct
		if matchedTopic == "general_code_quality" {
			if strings.Contains(lowerBody, "error") || strings.Contains(lowerBody, "handle err") {
				matchedTopic = "error_handling"
				severity = models.SeverityHigh
			} else if strings.Contains(lowerBody, "timeout") || strings.Contains(lowerBody, "context") {
				matchedTopic = "context_timeout"
				severity = models.SeverityHigh
			} else if strings.Contains(lowerBody, "log") || strings.Contains(lowerBody, "print") {
				matchedTopic = "production_logging"
				severity = models.SeverityMedium
			}
		}

		ext := strings.ToLower(filepath.Ext(c.FilePath))
		clusterKey := fmt.Sprintf("%s:%s", ext, matchedTopic)

		cl, exists := clusterMap[clusterKey]
		if !exists {
			cl = &RuleCandidateCluster{
				ClusterKey:       clusterKey,
				Topic:            matchedTopic,
				Comments:         make([]PastReviewComment, 0),
				SamplePaths:      make([]string, 0),
				SuggestedLevel:   NormativeMustNot,
				SuggestedScope:   ScopeFile,
				InferredSeverity: severity,
			}
			clusterMap[clusterKey] = cl
		}

		cl.Comments = append(cl.Comments, c)
		if len(cl.SamplePaths) < 5 {
			cl.SamplePaths = append(cl.SamplePaths, c.FilePath)
		}
	}

	clusters := make([]*RuleCandidateCluster, 0, len(clusterMap))
	for _, cl := range clusterMap {
		clusters = append(clusters, cl)
	}

	return clusters
}

func (g *DrixyRuleGenerator) synthesizeRuleFromCluster(
	orgID, repoID string,
	cl *RuleCandidateCluster,
) SynthesizedRuleDefinition {
	// Synthesize Title and Description
	topicWords := strings.Split(cl.Topic, "_")
	for i, w := range topicWords {
		if len(w) > 0 {
			topicWords[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	humanTopic := strings.Join(topicWords, " ")
	title := fmt.Sprintf("Enforce Standard Patterns for %s", humanTopic)
	description := fmt.Sprintf("Automated team convention derived from %d historical peer review comments regarding %s.",
		len(cl.Comments), humanTopic)

	// Infer Path Globs from sample paths
	pathGlobs := inferPathGlobs(cl.SamplePaths)

	// Synthesize Detector Pattern and Negative Pattern based on topic
	detectorPattern, negativePattern, remediation := synthesizeDetectorsForTopic(cl.Topic)

	// Synthesize Examples
	correctEx, incorrectEx := synthesizeExamplesForTopic(cl.Topic)

	// Confidence score is a function of cluster support and distinct reviewers
	distinctAuthors := make(map[string]struct{})
	for _, c := range cl.Comments {
		distinctAuthors[c.Author] = struct{}{}
	}

	confidence := 0.60 + (float64(len(cl.Comments))*0.05) + (float64(len(distinctAuthors))*0.05)
	if confidence > 0.98 {
		confidence = 0.98
	}

	return SynthesizedRuleDefinition{
		ID:               uuid.New(),
		OrgID:            orgID,
		RepoID:           repoID,
		Name:             title,
		Description:      description,
		Scope:            cl.SuggestedScope,
		Severity:         cl.InferredSeverity,
		NormativeLevel:   cl.SuggestedLevel,
		PathGlobs:        pathGlobs,
		DetectorPattern:  detectorPattern,
		NegativePattern:  negativePattern,
		Remediation:      remediation,
		CorrectExample:   correctEx,
		IncorrectExample: incorrectEx,
		SupportCount:     len(cl.Comments),
		ConfidenceScore:  confidence,
		CreatedAt:        time.Now().UTC(),
	}
}

func inferPathGlobs(paths []string) []string {
	if len(paths) == 0 {
		return []string{"**/*"}
	}

	extMap := make(map[string]struct{})
	dirMap := make(map[string]struct{})

	for _, p := range paths {
		ext := filepath.Ext(p)
		if ext != "" {
			extMap[ext] = struct{}{}
		}
		dir := filepath.Dir(p)
		if dir != "." && dir != "/" {
			dirMap[dir] = struct{}{}
		}
	}

	var globs []string
	for ext := range extMap {
		globs = append(globs, fmt.Sprintf("**/*%s", ext))
	}

	if len(globs) == 0 {
		globs = append(globs, "**/*")
	}

	sort.Strings(globs)
	return globs
}

func synthesizeDetectorsForTopic(topic string) (detector string, negative string, remediation string) {
	switch topic {
	case "sql_injection", "raw_query":
		detector = `(?i)(fmt\.sprintf|strings\.builder|\+\s*["'].*select|insert|update|delete)`
		negative = `(?i)(\$1|\?|db\.execcontext\(|db\.querycontext\()`
		remediation = "Use parameterized SQL queries with bind variables ($1, ?)"
	case "hardcoded_secrets", "hardcoded", "token_leak", "secret":
		detector = `(?i)(bearer\s+[a-z0-9_\-\.]{20,}|(?:api[_-]?key|secret[_-]?key)\s*[:=]\s*["'][a-zA-Z0-9_\-]{16,}["'])`
		negative = `(?i)(os\.getenv|config\.|secretmanager|process\.env)`
		remediation = "Store API tokens in environment variables or cloud secret manager"
	case "concurrency_race", "data_race", "mutex":
		detector = `(?i)(go\s+func\(.*\)\s*\{[^}]*\b[a-zA-Z_]\w*\s*=[^}]*\})`
		negative = `(?i)(\.lock\(|\.unlock\(|atomic\.)`
		remediation = "Protect shared state mutations with sync.Mutex or atomic operations"
	case "error_handling":
		detector = `(?i)(err\s*:=\s*[^;]+;\s*err\s*==\s*nil|_\s*=\s*[a-zA-Z_]\w*\(.*\))`
		negative = `(?i)(if\s+err\s*!=\s*nil)`
		remediation = "Explicitly handle and propagate non-nil errors"
	case "context_timeout":
		detector = `(?i)(http\.get\(|http\.post\(|exec\.command\()`
		negative = `(?i)(newrequestwithcontext|commandcontext)`
		remediation = "Use context-bounded calls (http.NewRequestWithContext, exec.CommandContext)"
	case "production_logging":
		detector = `(?i)(fmt\.print(ln|f)?\(|console\.log\()`
		negative = `(?i)(slog\.|logger\.|zap\.|logrus\.)`
		remediation = "Use structured leveled logging (slog, zap) instead of stdout print statements"
	default:
		detector = `(?i)\bTODO:\s*(fix|hack|bug)\b`
		negative = ""
		remediation = "Resolve technical debt or track with formal issue ticket"
	}

	return detector, negative, remediation
}

func synthesizeExamplesForTopic(topic string) (correct string, incorrect string) {
	switch topic {
	case "sql_injection", "raw_query":
		correct = "db.QueryContext(ctx, \"SELECT * FROM users WHERE id = $1\", userID)"
		incorrect = "db.Query(fmt.Sprintf(\"SELECT * FROM users WHERE id = '%s'\", userID))"
	case "hardcoded_secrets", "hardcoded", "token_leak", "secret":
		correct = "apiKey := os.Getenv(\"STRIPE_API_KEY\")"
		incorrect = "apiKey := \"sk_live_51Abcdef1234567890abcdef12345\""
	case "context_timeout":
		correct = "req, err := http.NewRequestWithContext(ctx, \"GET\", url, nil)"
		incorrect = "resp, err := http.Get(url)"
	case "error_handling":
		correct = "val, err := doOp()\nif err != nil {\n    return nil, fmt.Errorf(\"failed op: %w\", err)\n}"
		incorrect = "val, _ := doOp()\nreturn val, nil"
	default:
		correct = "// Approved implementation conforming to team standards"
		incorrect = "// Deprecated or anti-pattern implementation"
	}

	return correct, incorrect
}

// ComputeRuleFingerprint returns a deterministic sha256 hash across the rule's detector and scope.
func ComputeRuleFingerprint(r *SynthesizedRuleDefinition) string {
	h := sha256.New()
	h.Write([]byte(r.Name))
	h.Write([]byte(r.Scope))
	h.Write([]byte(r.DetectorPattern))
	for _, g := range r.PathGlobs {
		h.Write([]byte(g))
	}
	return hex.EncodeToString(h.Sum(nil))
}
