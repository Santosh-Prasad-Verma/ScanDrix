// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package stages

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/codeanalysis"
	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/internal/review/priority"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/pkg/models"
)

const (
	DefaultMinBatchSize       = 10
	DefaultMaxBatchSize       = 30
	DefaultMaxTokensPerBatch  = 64000
	DefaultMaxFindingsPerFile = 5
	DefaultMaxFindingsPerPR   = 30
)

// FileAnalysisBatch represents a partition of changed files processed together.
type FileAnalysisBatch struct {
	BatchIndex int
	Files      []pipeline.FileChangeInfo
	TotalLines int
	RiskScore  float64
}

// FileProcessingResult holds findings and telemetry from analyzing a single file.
type FileProcessingResult struct {
	Filename         string
	Findings         []models.CodeFinding
	ValidSuggestions []domain.CodeSuggestion
	DiscardedCount   int
	ProcessingTime   time.Duration
	RiskTier         priority.CoverageTier
	Error            error
}

// ProcessFilesReviewStage orchestrates file-level code analysis, risk-based batching,
// snippet enrichment, and safeguard deduplication across PR changes.
type ProcessFilesReviewStage struct {
	minBatchSize       int
	maxBatchSize       int
	maxTokensPerBatch  int
	maxFindingsPerFile int
	maxFindingsPerPR   int
	riskClassifier     *priority.SecurityRiskClassifier
	identifierRegex    *regexp.Regexp
}

// ProcessFilesReviewOption configures ProcessFilesReviewStage.
type ProcessFilesReviewOption func(*ProcessFilesReviewStage)

// WithBatchBounds sets minimum and maximum file batch sizes.
func WithBatchBounds(minBatch, maxBatch int) ProcessFilesReviewOption {
	return func(s *ProcessFilesReviewStage) {
		if minBatch > 0 {
			s.minBatchSize = minBatch
		}
		if maxBatch >= s.minBatchSize {
			s.maxBatchSize = maxBatch
		}
	}
}

// WithFindingsCaps sets maximum allowed findings per file and per PR.
func WithFindingsCaps(perFile, perPR int) ProcessFilesReviewOption {
	return func(s *ProcessFilesReviewStage) {
		if perFile > 0 {
			s.maxFindingsPerFile = perFile
		}
		if perPR > 0 {
			s.maxFindingsPerPR = perPR
		}
	}
}

// NewProcessFilesReviewStage constructs the file processing review stage.
func NewProcessFilesReviewStage(opts ...ProcessFilesReviewOption) *ProcessFilesReviewStage {
	stage := &ProcessFilesReviewStage{
		minBatchSize:       DefaultMinBatchSize,
		maxBatchSize:       DefaultMaxBatchSize,
		maxTokensPerBatch:  DefaultMaxTokensPerBatch,
		maxFindingsPerFile: DefaultMaxFindingsPerFile,
		maxFindingsPerPR:   DefaultMaxFindingsPerPR,
		riskClassifier:     priority.NewSecurityRiskClassifier(),
		identifierRegex:    regexp.MustCompile(`\b[a-zA-Z_][a-zA-Z0-9_]{2,}\b`),
	}

	for _, opt := range opts {
		opt(stage)
	}

	return stage
}

// Name returns the pipeline stage identifier.
func (s *ProcessFilesReviewStage) Name() string {
	return "ProcessFilesReview"
}

// Execute orchestrates the full file processing lifecycle.
func (s *ProcessFilesReviewStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	start := time.Now()

	if pCtx.SkipReview {
		pCtx.AddMetric(s.Name(), time.Since(start), true, nil, 0)
		return nil
	}

	// 1. Filter and prepare reviewable files
	reviewableFiles := s.filterAndPrepareFiles(pCtx.ChangedFiles, pCtx.IgnoredFiles)
	if len(reviewableFiles) == 0 {
		pCtx.AddMetric(s.Name(), time.Since(start), true, nil, 0)
		return nil
	}

	// 2. Initialize Coverage Ledger for interval tracking
	ledger := priority.NewCoverageLedger()
	for _, f := range reviewableFiles {
		ledger.RegisterTarget(f.Filename, f.Patch, priority.TierWarm)
	}

	// 3. Partition files into risk-weighted optimized batches
	batches := s.createOptimizedBatches(reviewableFiles)

	// 4. Execute batches sequentially with bounded concurrency per batch
	var allResults []FileProcessingResult
	for _, batch := range batches {
		select {
		case <-ctx.Done():
			pCtx.AddError(s.Name(), "BatchExecution", ctx.Err(), "critical", nil)
			return ctx.Err()
		default:
		}

		batchResults := s.processSingleBatch(ctx, pCtx, batch, ledger)
		allResults = append(allResults, batchResults...)
	}

	// 5. Aggregate, prioritize by severity, and apply PR-level safeguards
	s.consolidateResults(pCtx, allResults)

	pCtx.AddMetric(s.Name(), time.Since(start), true, nil, len(reviewableFiles))
	return nil
}

// filterAndPrepareFiles excludes ignored files, vendored directories, and binaries.
func (s *ProcessFilesReviewStage) filterAndPrepareFiles(files []pipeline.FileChangeInfo, ignored []string) []pipeline.FileChangeInfo {
	ignoredMap := make(map[string]struct{}, len(ignored))
	for _, ig := range ignored {
		ignoredMap[ig] = struct{}{}
	}

	var valid []pipeline.FileChangeInfo
	for _, f := range files {
		if _, exists := ignoredMap[f.Filename]; exists {
			continue
		}
		if f.Status == "removed" {
			continue
		}
		if isBinaryOrGenerated(f.Filename) {
			continue
		}
		valid = append(valid, f)
	}

	return valid
}

// createOptimizedBatches groups files by risk score and token bounds.
func (s *ProcessFilesReviewStage) createOptimizedBatches(files []pipeline.FileChangeInfo) []FileAnalysisBatch {
	if len(files) == 0 {
		return nil
	}

	// Sort files by line modifications descending (process largest/riskiest first)
	sorted := make([]pipeline.FileChangeInfo, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool {
		return (sorted[i].Additions + sorted[i].Deletions) > (sorted[j].Additions + sorted[j].Deletions)
	})

	var batches []FileAnalysisBatch
	currentBatch := FileAnalysisBatch{BatchIndex: 0}
	currentTokens := 0

	for _, file := range sorted {
		estimatedTokens := (file.Additions + file.Deletions) * 15 // Rough token estimate
		if estimatedTokens < 200 {
			estimatedTokens = 200
		}

		shouldSplit := len(currentBatch.Files) >= s.maxBatchSize ||
			(currentTokens+estimatedTokens > s.maxTokensPerBatch && len(currentBatch.Files) >= s.minBatchSize)

		if shouldSplit {
			batches = append(batches, currentBatch)
			currentBatch = FileAnalysisBatch{
				BatchIndex: len(batches),
			}
			currentTokens = 0
		}

		currentBatch.Files = append(currentBatch.Files, file)
		currentBatch.TotalLines += file.Additions + file.Deletions
		currentTokens += estimatedTokens
	}

	if len(currentBatch.Files) > 0 {
		batches = append(batches, currentBatch)
	}

	return batches
}

// processSingleBatch executes file analysis with worker concurrency.
func (s *ProcessFilesReviewStage) processSingleBatch(
	ctx context.Context,
	pCtx *pipeline.PipelineContext,
	batch FileAnalysisBatch,
	ledger *priority.CoverageLedger,
) []FileProcessingResult {
	results := make([]FileProcessingResult, len(batch.Files))
	var wg sync.WaitGroup

	maxConcurrency := 4
	sem := make(chan struct{}, maxConcurrency)

	for i, file := range batch.Files {
		wg.Add(1)
		go func(idx int, f pipeline.FileChangeInfo) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[idx] = FileProcessingResult{
					Filename: f.Filename,
					Error:    ctx.Err(),
				}
				return
			}

			start := time.Now()
			res := s.analyzeIndividualFile(ctx, pCtx, f, ledger)
			res.ProcessingTime = time.Since(start)
			results[idx] = res
		}(i, file)
	}

	wg.Wait()
	return results
}

// analyzeIndividualFile evaluates an individual file against rules, safeguards, and diff boundaries.
func (s *ProcessFilesReviewStage) analyzeIndividualFile(
	ctx context.Context,
	pCtx *pipeline.PipelineContext,
	file pipeline.FileChangeInfo,
	ledger *priority.CoverageLedger,
) FileProcessingResult {
	result := FileProcessingResult{
		Filename: file.Filename,
	}

	// 1. Calculate risk tier
	diffContent := file.Patch
	riskProfile := s.riskClassifier.ClassifyFile(file.Filename, "", diffContent)
	result.RiskTier = riskProfile.RecommendedTier

	// 2. Extract identifiers for knowledge correlation
	identifiers := s.ExtractDiffIdentifiers(file.Patch)

	// 3. Match relevant enriched hunks or trace decisions
	matchedHunks := s.findMatchingHunks(pCtx.EnrichedHunks, file.Filename)
	_ = identifiers
	_ = matchedHunks

	// 4. Generate candidate findings from active rules matching file path
	candidateFindings := s.evaluateActiveRulesOnPatch(file, pCtx.ActiveRules, pCtx.ReviewID)

	// 5. Apply Diff Boundary and Safeguard Filtering
	validDiffLines := file.ValidDiffLines
	if len(validDiffLines) == 0 {
		validDiffLines = extractValidLineIntervalsFromPatch(file.Patch)
	}

	for _, finding := range candidateFindings {
		// Verify finding line falls inside modified diff hunks
		if !isLineInIntervals(finding.StartLine, validDiffLines) {
			result.DiscardedCount++
			continue
		}

		result.Findings = append(result.Findings, finding)

		// Create corresponding valid suggestion
		sug := domain.CodeSuggestion{
			ID:                 uuid.New(),
			PullRequestID:      fmt.Sprintf("%d", pCtx.PullNumber),
			PullNumber:         pCtx.PullNumber,
			RelevantFile:       file.Filename,
			FilePath:           file.Filename,
			StartLine:          finding.StartLine,
			EndLine:            finding.EndLine,
			RelevantLinesStart: finding.StartLine,
			RelevantLinesEnd:   finding.EndLine,
			RuleID:             finding.Title,
			Severity:           domain.ReviewSeverity(finding.Severity),
			Category:           domain.CategoryRules,
			OneSentenceSummary: finding.Title,
			Description:        finding.Description,
			Confidence:         0.90,
			PriorityStatus:     domain.PriorityStatusPrioritized,
			DeliveryStatus:     domain.DeliveryStatusQueued,
		}
		result.ValidSuggestions = append(result.ValidSuggestions, sug)

		// Record coverage in ledger
		ledger.RecordObservation(file.Filename, finding.StartLine, finding.EndLine, "ProcessFilesReview", 1, "rule-evaluator")
	}

	return result
}

// ExtractDiffIdentifiers tokenizes changed code to identify target functions and symbols.
func (s *ProcessFilesReviewStage) ExtractDiffIdentifiers(patch string) []string {
	if patch == "" {
		return nil
	}

	matches := s.identifierRegex.FindAllString(patch, -1)
	if len(matches) == 0 {
		return nil
	}

	unique := make(map[string]struct{})
	var idents []string
	for _, m := range matches {
		lower := strings.ToLower(m)
		if isCommonCodeKeyword(lower) {
			continue
		}
		if _, seen := unique[lower]; !seen {
			unique[lower] = struct{}{}
			idents = append(idents, m)
		}
	}

	return idents
}

// evaluateActiveRulesOnPatch applies regex and token rules against added lines in patch.
func (s *ProcessFilesReviewStage) evaluateActiveRulesOnPatch(
	file pipeline.FileChangeInfo,
	activeRules []rules.RuleSpec,
	reviewID uuid.UUID,
) []models.CodeFinding {
	if len(activeRules) == 0 || file.Patch == "" {
		return nil
	}

	var findings []models.CodeFinding
	lines := strings.Split(file.Patch, "\n")
	currentRightLine := 0

	for _, line := range lines {
		if strings.HasPrefix(line, "@@") {
			currentRightLine = parseHunkStartLine(line)
			continue
		}

		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			addedContent := strings.TrimPrefix(line, "+")

			for _, r := range activeRules {
				// Match path pattern if specified
				if r.PathPattern != "" && !matchesGlob(file.Filename, r.PathPattern) {
					continue
				}

				if r.RegexRule != "" {
					re, err := regexp.Compile(r.RegexRule)
					if err == nil && re.MatchString(addedContent) {
						findings = append(findings, models.CodeFinding{
							ID:          uuid.New(),
							ReviewID:    reviewID,
							FilePath:    file.Filename,
							StartLine:   currentRightLine,
							EndLine:     currentRightLine,
							Severity:    r.Severity,
							Title:       r.Name,
							Description: r.Description,
							Remediation: r.Remediation,
							Category:    r.Category,
							Fingerprint: fmt.Sprintf("%s:%d:%s", file.Filename, currentRightLine, r.Name),
						})
					}
				}
			}
			currentRightLine++
		} else if !strings.HasPrefix(line, "-") {
			currentRightLine++
		}
	}

	return findings
}

// consolidateResults enforces per-file and PR-wide finding limits prioritizing highest severity.
func (s *ProcessFilesReviewStage) consolidateResults(pCtx *pipeline.PipelineContext, results []FileProcessingResult) {
	var allFindings []models.CodeFinding
	var allSuggestions []domain.CodeSuggestion

	for _, res := range results {
		if res.Error != nil {
			pCtx.AddError(s.Name(), "FileError", res.Error, "partial", map[string]interface{}{
				"filename": res.Filename,
			})
			continue
		}

		// Sort findings by severity descending before per-file capping
		fileFindings := res.Findings
		sort.Slice(fileFindings, func(i, j int) bool {
			return severityWeight(fileFindings[i].Severity) > severityWeight(fileFindings[j].Severity)
		})

		if len(fileFindings) > s.maxFindingsPerFile {
			fileFindings = fileFindings[:s.maxFindingsPerFile]
		}
		allFindings = append(allFindings, fileFindings...)

		fileSug := res.ValidSuggestions
		sort.Slice(fileSug, func(i, j int) bool {
			return severityWeight(models.FindingSeverity(fileSug[i].Severity)) > severityWeight(models.FindingSeverity(fileSug[j].Severity))
		})
		if len(fileSug) > s.maxFindingsPerFile {
			fileSug = fileSug[:s.maxFindingsPerFile]
		}
		allSuggestions = append(allSuggestions, fileSug...)
	}

	// PR-level capping
	sort.Slice(allFindings, func(i, j int) bool {
		return severityWeight(allFindings[i].Severity) > severityWeight(allFindings[j].Severity)
	})
	if len(allFindings) > s.maxFindingsPerPR {
		allFindings = allFindings[:s.maxFindingsPerPR]
	}

	sort.Slice(allSuggestions, func(i, j int) bool {
		return severityWeight(models.FindingSeverity(allSuggestions[i].Severity)) > severityWeight(models.FindingSeverity(allSuggestions[j].Severity))
	})
	if len(allSuggestions) > s.maxFindingsPerPR {
		allSuggestions = allSuggestions[:s.maxFindingsPerPR]
	}

	pCtx.AllFindings = append(pCtx.AllFindings, allFindings...)
	pCtx.ValidSuggestions = append(pCtx.ValidSuggestions, allSuggestions...)
}

func (s *ProcessFilesReviewStage) findMatchingHunks(hunks []codeanalysis.EnrichedHunk, filename string) []codeanalysis.EnrichedHunk {
	var matched []codeanalysis.EnrichedHunk
	for _, h := range hunks {
		if h.FilePath == filename {
			matched = append(matched, h)
		}
	}
	return matched
}

func extractHunkIntervals(patch string) [][2]int {
	if patch == "" {
		return nil
	}
	var intervals [][2]int
	lines := strings.Split(patch, "\n")
	for _, l := range lines {
		if strings.HasPrefix(l, "@@") {
			start, count := parseHunkCoordinates(l)
			if count > 0 {
				intervals = append(intervals, [2]int{start, start + count - 1})
			}
		}
	}
	return intervals
}

func extractValidLineIntervalsFromPatch(patch string) [][2]int {
	return extractHunkIntervals(patch)
}

func parseHunkCoordinates(hunkHeader string) (int, int) {
	plusIdx := strings.Index(hunkHeader, "+")
	if plusIdx == -1 {
		return 1, 1
	}
	sub := hunkHeader[plusIdx+1:]
	spaceIdx := strings.Index(sub, " ")
	if spaceIdx != -1 {
		sub = sub[:spaceIdx]
	}

	var start, count int
	if strings.Contains(sub, ",") {
		_, _ = fmt.Sscanf(sub, "%d,%d", &start, &count)
	} else {
		_, _ = fmt.Sscanf(sub, "%d", &start)
		count = 1
	}

	if start <= 0 {
		start = 1
	}
	if count <= 0 {
		count = 1
	}
	return start, count
}

func parseHunkStartLine(hunkHeader string) int {
	start, _ := parseHunkCoordinates(hunkHeader)
	return start
}

func isLineInIntervals(line int, intervals [][2]int) bool {
	if len(intervals) == 0 {
		return true
	}
	for _, iv := range intervals {
		if line >= iv[0] && line <= iv[1] {
			return true
		}
	}
	return false
}

func matchesGlob(path string, glob string) bool {
	clean := filepath.ToSlash(path)
	gClean := filepath.ToSlash(glob)
	if gClean == "**/*" || gClean == "*" {
		return true
	}
	if matched, _ := filepath.Match(gClean, clean); matched {
		return true
	}
	if strings.HasPrefix(gClean, "**/*") {
		ext := strings.TrimPrefix(gClean, "**/*")
		if strings.HasSuffix(clean, ext) {
			return true
		}
	}
	return false
}

func severityWeight(sev models.FindingSeverity) int {
	switch sev {
	case models.SeverityCritical:
		return 4
	case models.SeverityHigh:
		return 3
	case models.SeverityMedium:
		return 2
	case models.SeverityLow:
		return 1
	default:
		return 0
	}
}

func isBinaryOrGenerated(path string) bool {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".jpg") ||
		strings.HasSuffix(lower, ".svg") || strings.HasSuffix(lower, ".pdf") ||
		strings.HasSuffix(lower, ".woff") || strings.HasSuffix(lower, ".woff2") ||
		strings.HasSuffix(lower, ".lock") || strings.HasSuffix(lower, ".sum") ||
		strings.HasSuffix(lower, ".min.js") || strings.HasSuffix(lower, ".min.css") {
		return true
	}
	return false
}

func isCommonCodeKeyword(kw string) bool {
	switch kw {
	case "func", "return", "var", "const", "type", "struct", "interface",
		"if", "else", "for", "range", "switch", "case", "import", "package",
		"true", "false", "nil", "int", "string", "bool", "err", "error",
		"let", "function", "class", "def", "public", "private":
		return true
	default:
		return false
	}
}
