// Package orchestrator coordinates specialized review agents and synthesizes multi-perspective findings.
package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

const (
	// PRShardDiffBudgetChars caps total diff characters sent to PR-scope shard to prevent context exhaustion (150k chars).
	PRShardDiffBudgetChars = 150_000

	// DefaultRuleRefMaxChars caps the total reference content inlined per rule to prevent prompt blowup.
	DefaultRuleRefMaxChars = 6_000

	// DefaultShardConcurrency is the max parallel shard LLM calls.
	DefaultShardConcurrency = 4
)

// RawShardViolation models the unverified LLM output from a single shard evaluation.
type RawShardViolation struct {
	RuleID             any    `json:"ruleId"`
	AltRuleID          any    `json:"rule_id,omitempty"`
	RelevantLinesStart *int   `json:"relevantLinesStart,omitempty"`
	AltLinesStart      *int   `json:"relevant_lines_start,omitempty"`
	RelevantLinesEnd   *int   `json:"relevantLinesEnd,omitempty"`
	AltLinesEnd        *int   `json:"relevant_lines_end,omitempty"`
	Language           string `json:"language,omitempty"`
	ExistingCode       string `json:"existingCode,omitempty"`
	AltExistingCode    string `json:"existing_code,omitempty"`
	ImprovedCode       string `json:"improvedCode,omitempty"`
	AltImprovedCode    string `json:"improved_code,omitempty"`
	SuggestionContent  string `json:"suggestionContent"`
	AltContent         string `json:"suggestion_content,omitempty"`
	OneSentenceSummary string `json:"oneSentenceSummary,omitempty"`
	AltSummary         string `json:"one_sentence_summary,omitempty"`
}

// ShardResponse represents the JSON output format expected from the LLM.
type ShardResponse struct {
	Violations []RawShardViolation `json:"violations"`
}

// ShardSystemPrompt instructs the model to judge a single file against batched rules.
const ShardSystemPrompt = `You check a set of team rules against the diff of a SINGLE file. Report EVERY added line that violates ANY of the listed rules — one entry per (rule, violating line).

Rules of engagement:
- Only flag lines ADDED in this diff (each line is prefixed with its file line number then '+'). Unchanged context lines are NEVER flagged.
- One entry PER violating line PER rule; do not collapse repeats. Downstream dedup folds repeats into one comment.
- Identify the violated rule by its number — the [n] shown before each rule. Put that number in "ruleId". Never invent a number; if a real issue matches no listed rule, DROP it.
- If nothing violates, return an empty list.`

// ShardPRSystemPrompt instructs the model to evaluate whole-PR rules against the full PR diff.
const ShardPRSystemPrompt = `You evaluate PULL-REQUEST-level team rules against a PR: its title, description, the list of changed files, and the FULL DIFF of every changed file. Judge the PR as a whole — cross-file conditions (e.g. "one migration = one logical change", "index added to a table that already existed before this PR") are exactly what these rules are about, so reason across the whole diff. Identify each violated rule by its number — the [n] shown before each rule — and put that number in "ruleId"; never invent one. Return only real violations.`

// ShardedJudgeExecutor abstracts the LLM invocation for a single shard.
type ShardedJudgeExecutor interface {
	ExecuteShard(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// ShardedJudgeInput defines the input parameters for the sharded judge sweep.
type ShardedJudgeInput struct {
	ChangedFiles  []ChangedFile
	Rules         []DrixyRule
	Executor      ShardedJudgeExecutor
	PRTitle       string
	PRBody        string
	LanguageLabel string
	Concurrency   int
}

// ShardedJudgeResult returns the aggregated findings and execution counters.
type ShardedJudgeResult struct {
	Findings      []AgentFinding
	ShardsRun     int
	ShardsErrored int
}

// DeterministicShardedJudge executes structural, deterministic file × rule matrix evaluations.
type DeterministicShardedJudge struct {
	executor ShardedJudgeExecutor
}

// NewDeterministicShardedJudge constructs a new sharded judge.
func NewDeterministicShardedJudge(executor ShardedJudgeExecutor) *DeterministicShardedJudge {
	return &DeterministicShardedJudge{
		executor: executor,
	}
}

// RuleAppliesToFile tests whether a filename matches any of the rule's path globs.
// Supports comma-separated glob patterns (e.g. "src/**/*.go, pkg/*.go, internal/auth/*").
func RuleAppliesToFile(filePath string, pattern string) bool {
	if pattern == "" {
		return true
	}

	cleanFile := filepath.ToSlash(filepath.Clean(filePath))
	cleanFile = strings.TrimPrefix(cleanFile, "./")
	cleanFile = strings.TrimPrefix(cleanFile, "/")

	subPatterns := strings.Split(pattern, ",")
	for _, sub := range subPatterns {
		sub = strings.TrimSpace(filepath.ToSlash(sub))
		sub = strings.TrimPrefix(sub, "./")
		sub = strings.TrimPrefix(sub, "/")
		if sub == "" {
			continue
		}

		if matchPathGlob(sub, cleanFile) {
			return true
		}
	}

	return false
}

// matchPathGlob matches a path against a glob pattern supporting '**', '*', and '?'.
func matchPathGlob(pattern, path string) bool {
	pattern = filepath.ToSlash(pattern)
	path = filepath.ToSlash(path)

	// Direct match
	if pattern == path {
		return true
	}

	// If no double-star wildcard, use standard filepath.Match
	if !strings.Contains(pattern, "**") {
		if matched, err := filepath.Match(pattern, path); err == nil && matched {
			return true
		}
		// Also match against basename if pattern contains no directory slash
		if !strings.Contains(pattern, "/") {
			if matched, err := filepath.Match(pattern, filepath.Base(path)); err == nil && matched {
				return true
			}
		}
		return false
	}

	// Double-star glob conversion to regex
	var sb strings.Builder
	sb.WriteString("^")
	i := 0
	n := len(pattern)
	for i < n {
		if strings.HasPrefix(pattern[i:], "/**/") {
			sb.WriteString("(?:/|/.+/)")
			i += 4
		} else if strings.HasPrefix(pattern[i:], "/**") && (i+3 == n || pattern[i+3] == '/') {
			sb.WriteString("(?:/.*)?")
			i += 3
		} else if strings.HasPrefix(pattern[i:], "**/") {
			sb.WriteString("(?:.*/)?")
			i += 3
		} else if strings.HasPrefix(pattern[i:], "**") {
			sb.WriteString(".*")
			i += 2
		} else if pattern[i] == '*' {
			sb.WriteString("[^/]*")
			i++
		} else if pattern[i] == '?' {
			sb.WriteString("[^/]")
			i++
		} else {
			ch := pattern[i]
			if strings.ContainsRune(`.+()|{}^$[]\`, rune(ch)) {
				sb.WriteByte('\\')
			}
			sb.WriteByte(ch)
			i++
		}
	}
	sb.WriteString("$")

	re, err := regexp.Compile(sb.String())
	if err != nil {
		return false
	}
	return re.MatchString(path)
}

// FileMatchesRule checks if file satisfies rule's path globs.
func (j *DeterministicShardedJudge) FileMatchesRule(filename string, rule DrixyRule) bool {
	if len(rule.PathGlobs) == 0 {
		return true
	}
	for _, glob := range rule.PathGlobs {
		if RuleAppliesToFile(filename, glob) {
			return true
		}
	}
	return false
}

// FileRuleShard groups a changed file with the subset of rules matching its path.
type FileRuleShard struct {
	File  ChangedFile
	Rules []DrixyRule
}

// BuildShards partitions rules into file-level vs PR-level, and builds per-file shards.
func (j *DeterministicShardedJudge) BuildShards(files []ChangedFile, rules []DrixyRule) ([]FileRuleShard, []DrixyRule) {
	var fileRules []DrixyRule
	var prRules []DrixyRule
	for _, r := range rules {
		if !r.IsActive {
			continue
		}
		if strings.EqualFold(r.Scope, "pull_request") || strings.EqualFold(r.Scope, "pr") {
			prRules = append(prRules, r)
		} else {
			fileRules = append(fileRules, r)
		}
	}

	var shards []FileRuleShard
	for _, f := range files {
		if f.IsBinary || f.IsVendored {
			continue
		}
		var matched []DrixyRule
		for _, r := range fileRules {
			if j.FileMatchesRule(f.Filename, r) {
				matched = append(matched, r)
			}
		}
		if len(matched) > 0 {
			shards = append(shards, FileRuleShard{
				File:  f,
				Rules: matched,
			})
		}
	}
	return shards, prRules
}

// FormatRuleBlock formats a numbered list of rules for inclusion in the user prompt.
func FormatRuleBlock(rules []DrixyRule) string {
	var sb strings.Builder
	for i, r := range rules {
		sb.WriteString(fmt.Sprintf("[%d] %s\n", i+1, r.Name))
		if r.Description != "" {
			sb.WriteString(fmt.Sprintf("  description: %s\n", r.Description))
		}
		if r.Prompt != "" {
			sb.WriteString(fmt.Sprintf("  requirement: %s\n", r.Prompt))
		}
		if len(r.Examples) > 0 {
			sb.WriteString("  examples:\n")
			for _, ex := range r.Examples {
				label := "incorrect"
				if ex.IsCorrect {
					label = "correct"
				}
				sb.WriteString(fmt.Sprintf("    - %s: %s\n", label, ex.Snippet))
			}
		}
	}
	return sb.String()
}

// FormatLanguageInstruction returns language localization directives if specified.
func FormatLanguageInstruction(languageLabel string) string {
	if languageLabel == "" {
		return ""
	}
	return fmt.Sprintf("Respond in %s: write \"suggestionContent\" and \"oneSentenceSummary\" in %s, not English. This is mandatory.\n\n", languageLabel, languageLabel)
}

// BuildFileShardUserPrompt formats the prompt for a single file shard.
func BuildFileShardUserPrompt(file ChangedFile, rules []DrixyRule, languageLabel string) string {
	diff := file.Patch
	if diff == "" {
		diff = file.Content
	}

	langInst := FormatLanguageInstruction(languageLabel)

	return fmt.Sprintf(`<Rules>
%s</Rules>

<File path="%s">
Each diff line is prefixed with its file line number; '+' marks a line ADDED by this PR.
`+"```diff\n%s\n```"+`
</File>

%sReturn ONLY JSON (ruleId is the rule's [n] number):
{"violations":[{"ruleId":<n>,"relevantLinesStart":<line>,"relevantLinesEnd":<line>,"existingCode":"<offending code>","suggestionContent":"WHAT/WHY/HOW","oneSentenceSummary":"<short>"}]}`,
		FormatRuleBlock(rules), file.Filename, diff, langInst,
	)
}

// BuildPRShardUserPrompt formats the whole-PR user prompt up to PRShardDiffBudgetChars.
func BuildPRShardUserPrompt(files []ChangedFile, rules []DrixyRule, prTitle, prBody, languageLabel string) string {
	used := 0
	var diffs strings.Builder
	for _, f := range files {
		diff := f.Patch
		if diff == "" {
			diff = f.Content
		}
		if diff == "" {
			diffs.WriteString(fmt.Sprintf("## file: '%s' (no diff available)\n", f.Filename))
			continue
		}
		if used+len(diff) > PRShardDiffBudgetChars {
			diffs.WriteString(fmt.Sprintf("## file: '%s' (diff omitted — PR diff budget exceeded)\n", f.Filename))
			continue
		}
		used += len(diff)
		diffs.WriteString(fmt.Sprintf("## file: '%s'\n%s\n\n", f.Filename, diff))
	}

	var fileList strings.Builder
	for _, f := range files {
		fileList.WriteString(fmt.Sprintf("- %s (+%d, -%d)\n", f.Filename, f.Additions, f.Deletions))
	}

	desc := prBody
	if len(desc) > 1000 {
		desc = desc[:1000] + "..."
	}
	if desc == "" {
		desc = "(empty)"
	}

	langInst := FormatLanguageInstruction(languageLabel)

	return fmt.Sprintf(`<Rules>
%s</Rules>

<PR title=%q>
Description: %s
Changed files (%d):
%s
Full diff of every changed file (each line prefixed with its file line number; '+' marks a line ADDED by this PR):
`+"```diff\n%s\n```"+`
</PR>

%sReturn ONLY JSON (ruleId is the rule's [n] number):
{"violations":[{"ruleId":<n>,"suggestionContent":"WHAT/WHY","oneSentenceSummary":"<short>"}]}`,
		FormatRuleBlock(rules), prTitle, desc, len(files), fileList.String(), diffs.String(), langInst,
	)
}

// InlineRuleReferences fetches repository files referenced by rules (sourcePath) and appends their content.
func InlineRuleReferences(
	rules []DrixyRule,
	readFn func(path string, start, end int) (string, error),
	maxRefChars int,
) []DrixyRule {
	if readFn == nil {
		return rules
	}
	if maxRefChars <= 0 {
		maxRefChars = DefaultRuleRefMaxChars
	}

	out := make([]DrixyRule, len(rules))
	for i, r := range rules {
		out[i] = r
		if r.SourcePath == "" {
			continue
		}
		content, err := readFn(r.SourcePath, 1, 10000)
		if err == nil && len(strings.TrimSpace(content)) > 0 {
			if len(content) > maxRefChars {
				content = content[:maxRefChars]
			}
			anchor := ""
			if r.SourceAnchor != "" {
				anchor = fmt.Sprintf(" (section: %s)", r.SourceAnchor)
			}
			out[i].Prompt = fmt.Sprintf(
				"%s\n\n[Authoritative convention referenced by this rule — from `%s`%s]:\n%s",
				r.Prompt, r.SourcePath, anchor, content,
			)
		}
	}
	return out
}

// InlineLoadedReferences appends external references resolved from Context OS into rules.
func InlineLoadedReferences(
	rules []DrixyRule,
	refMap map[string][]LoadedRuleReference,
	maxRefChars int,
) []DrixyRule {
	if len(refMap) == 0 {
		return rules
	}
	if maxRefChars <= 0 {
		maxRefChars = DefaultRuleRefMaxChars
	}

	out := make([]DrixyRule, len(rules))
	for i, r := range rules {
		out[i] = r
		refs, ok := refMap[r.ID.String()]
		if !ok || len(refs) == 0 {
			continue
		}

		appended := 0
		var sb strings.Builder
		sb.WriteString(r.Prompt)

		for _, ref := range refs {
			content := strings.TrimSpace(ref.Content)
			if content == "" {
				continue
			}
			rem := maxRefChars - appended
			if rem <= 0 {
				break
			}
			if len(content) > rem {
				content = content[:rem]
			}
			appended += len(content)
			path := ref.FilePath
			if path == "" {
				path = "referenced convention"
			}
			sb.WriteString(fmt.Sprintf("\n\n[Authoritative convention referenced by this rule — from `%s`]:\n%s", path, content))
		}

		if appended > 0 {
			out[i].Prompt = sb.String()
		}
	}
	return out
}

// ResolveRuleID maps a raw rule ID (1-based index or string) back to a DrixyRule in the shard.
func ResolveRuleID(rawID any, orderedRules []DrixyRule) *DrixyRule {
	if rawID == nil {
		if len(orderedRules) == 1 {
			return &orderedRules[0]
		}
		return nil
	}

	switch val := rawID.(type) {
	case float64:
		idx := int(val) - 1
		if idx >= 0 && idx < len(orderedRules) {
			return &orderedRules[idx]
		}
	case int:
		idx := val - 1
		if idx >= 0 && idx < len(orderedRules) {
			return &orderedRules[idx]
		}
	case string:
		clean := strings.TrimSpace(val)
		if n, err := strconv.Atoi(clean); err == nil {
			idx := n - 1
			if idx >= 0 && idx < len(orderedRules) {
				return &orderedRules[idx]
			}
		}

		// Check UUID match
		if u, err := uuid.Parse(clean); err == nil {
			for _, r := range orderedRules {
				if r.ID == u {
					return &r
				}
			}
		}

		// Check rule name match
		for _, r := range orderedRules {
			if strings.EqualFold(r.Name, clean) {
				return &r
			}
		}
	}

	if len(orderedRules) == 1 {
		return &orderedRules[0]
	}

	return nil
}

// EvaluateAll implements ReviewAgent evaluation by executing the sharded sweep.
func (j *DeterministicShardedJudge) EvaluateAll(ctx context.Context, input ReviewAgentInput) ([]AgentFinding, error) {
	res, err := j.JudgeSharded(ctx, ShardedJudgeInput{
		ChangedFiles:  input.ChangedFiles,
		Rules:         input.DrixyRules,
		Executor:      j.executor,
		PRTitle:       input.Title,
		PRBody:        input.Description,
		LanguageLabel: "English",
		Concurrency:   DefaultShardConcurrency,
	})
	if err != nil {
		return nil, err
	}
	return res.Findings, nil
}

// JudgeSharded runs the full deterministic file-shard + PR-shard matrix sweep.
func (j *DeterministicShardedJudge) JudgeSharded(ctx context.Context, input ShardedJudgeInput) (*ShardedJudgeResult, error) {
	executor := input.Executor
	if executor == nil {
		executor = j.executor
	}
	if executor == nil {
		return nil, fmt.Errorf("sharded judge executor is not configured")
	}

	concurrency := input.Concurrency
	if concurrency <= 0 {
		concurrency = DefaultShardConcurrency
	}

	// 1. Partition rules into file-level vs PR-level
	var fileRules []DrixyRule
	var prRules []DrixyRule
	for _, r := range input.Rules {
		if !r.IsActive {
			continue
		}
		if strings.EqualFold(r.Scope, "pull_request") || strings.EqualFold(r.Scope, "pr") {
			prRules = append(prRules, r)
		} else {
			fileRules = append(fileRules, r)
		}
	}

	// 2. Build file shards
	type fileShardTask struct {
		file  ChangedFile
		rules []DrixyRule
	}

	var tasks []fileShardTask
	for _, f := range input.ChangedFiles {
		if f.IsBinary || f.IsVendored {
			continue
		}
		var matched []DrixyRule
		for _, r := range fileRules {
			if j.FileMatchesRule(f.Filename, r) {
				matched = append(matched, r)
			}
		}
		if len(matched) > 0 {
			tasks = append(tasks, fileShardTask{
				file:  f,
				rules: matched,
			})
		}
	}

	var shardsRun atomic.Int64
	var shardsErrored atomic.Int64
	var findingsMu sync.Mutex
	var allFindings []AgentFinding

	// Semaphore for concurrency control
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	// Run file shards
	for _, task := range tasks {
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(t fileShardTask) {
			defer wg.Done()
			defer func() { <-sem }()

			shardsRun.Add(1)
			userPrompt := BuildFileShardUserPrompt(t.file, t.rules, input.LanguageLabel)

			rawResponse, err := executor.ExecuteShard(ctx, ShardSystemPrompt, userPrompt)
			if err != nil {
				shardsErrored.Add(1)
				return
			}

			findings, err := parseShardResponse(rawResponse, t.file.Filename, t.rules)
			if err != nil {
				shardsErrored.Add(1)
				return
			}

			findingsMu.Lock()
			allFindings = append(allFindings, findings...)
			findingsMu.Unlock()
		}(task)
	}

	// Run PR-level shard if PR rules exist
	if len(prRules) > 0 && ctx.Err() == nil {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			shardsRun.Add(1)
			userPrompt := BuildPRShardUserPrompt(input.ChangedFiles, prRules, input.PRTitle, input.PRBody, input.LanguageLabel)

			rawResponse, err := executor.ExecuteShard(ctx, ShardPRSystemPrompt, userPrompt)
			if err != nil {
				shardsErrored.Add(1)
				return
			}

			findings, err := parseShardResponse(rawResponse, "PULL_REQUEST", prRules)
			if err != nil {
				shardsErrored.Add(1)
				return
			}

			findingsMu.Lock()
			allFindings = append(allFindings, findings...)
			findingsMu.Unlock()
		}()
	}

	wg.Wait()

	return &ShardedJudgeResult{
		Findings:      allFindings,
		ShardsRun:     int(shardsRun.Load()),
		ShardsErrored: int(shardsErrored.Load()),
	}, nil
}

func parseShardResponse(raw string, defaultFile string, rules []DrixyRule) ([]AgentFinding, error) {
	clean := strings.TrimSpace(raw)
	if strings.HasPrefix(clean, "```json") {
		clean = strings.TrimPrefix(clean, "```json")
		if idx := strings.LastIndex(clean, "```"); idx != -1 {
			clean = clean[:idx]
		}
		clean = strings.TrimSpace(clean)
	} else if strings.HasPrefix(clean, "```") {
		clean = strings.TrimPrefix(clean, "```")
		if idx := strings.LastIndex(clean, "```"); idx != -1 {
			clean = clean[:idx]
		}
		clean = strings.TrimSpace(clean)
	}

	var resp ShardResponse
	if err := json.Unmarshal([]byte(clean), &resp); err != nil {
		// Try unmarshaling direct array
		var direct []RawShardViolation
		if err2 := json.Unmarshal([]byte(clean), &direct); err2 == nil {
			resp.Violations = direct
		} else {
			return nil, fmt.Errorf("failed to parse shard JSON: %w (raw: %s)", err, clean)
		}
	}

	var findings []AgentFinding
	for _, v := range resp.Violations {
		rID := v.RuleID
		if rID == nil {
			rID = v.AltRuleID
		}
		matchedRule := ResolveRuleID(rID, rules)
		if matchedRule == nil {
			continue
		}

		startLine := 1
		endLine := 1
		startPtr := v.RelevantLinesStart
		if startPtr == nil {
			startPtr = v.AltLinesStart
		}
		if startPtr != nil && *startPtr > 0 {
			startLine = *startPtr
			endLine = startLine
		}

		endPtr := v.RelevantLinesEnd
		if endPtr == nil {
			endPtr = v.AltLinesEnd
		}
		if endPtr != nil && *endPtr >= startLine {
			endLine = *endPtr
		}

		content := v.SuggestionContent
		if content == "" {
			content = v.AltContent
		}

		improved := v.ImprovedCode
		if improved == "" {
			improved = v.AltImprovedCode
		}

		existing := v.ExistingCode
		if existing == "" {
			existing = v.AltExistingCode
		}

		summary := v.OneSentenceSummary
		if summary == "" {
			summary = v.AltSummary
		}

		ruleUUID := matchedRule.ID
		findings = append(findings, AgentFinding{
			ID:                 uuid.New(),
			RuleID:             &ruleUUID,
			AgentName:          "drixy_rules",
			FilePath:           defaultFile,
			StartLine:          startLine,
			EndLine:            endLine,
			Severity:           matchedRule.Severity,
			Confidence:         "HIGH",
			Category:           "custom_rule",
			Title:              fmt.Sprintf("Rule Violation: %s", matchedRule.Name),
			Description:        content,
			Remediation:        improved,
			ExistingCode:       existing,
			ImprovedCode:       improved,
			OneSentenceSummary: summary,
			Blocking:           matchedRule.Severity == models.SeverityCritical || matchedRule.Severity == models.SeverityHigh,
			Fingerprint:        generateFindingFingerprint(defaultFile, startLine, matchedRule.Name),
		})
	}

	return findings, nil
}

func generateFindingFingerprint(file string, line int, title string) string {
	h := fmt.Sprintf("%s:%d:%s", file, line, title)
	return fmt.Sprintf("%x", h)
}
