package engine

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/pkg/models"
)

// CLIRunner coordinates code review execution across remote API services, local AI gateways, and AST rule engines.
type CLIRunner struct {
	evaluator  *rules.Evaluator
	httpClient *http.Client
	aiGateway  *llm.Gateway
}

// NewCLIRunner initializes the runner with the default OWASP rule engine, local AI gateway (if keys present in env), and HTTP transport.
func NewCLIRunner() *CLIRunner {
	ev, _ := rules.NewEvaluator(rules.DefaultCatalog())
	gw := InitLocalGateway()
	return &CLIRunner{
		evaluator:  ev,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		aiGateway:  gw,
	}
}

// InitLocalGateway initializes an LLM Gateway using environment variables and loaded .env files.
func InitLocalGateway() *llm.Gateway {
	// Attempt loading .env from workspace, ScanDrix folder, parent dirs, and user config
	loadEnvFiles := []string{
		"ScanDrix/.env",
		".env",
		"../ScanDrix/.env",
		"../.env",
		"../../.env",
	}
	if home, err := os.UserHomeDir(); err == nil {
		loadEnvFiles = append(loadEnvFiles,
			filepath.Join(home, ".config", "scandrix", ".env"),
			filepath.Join(home, ".scandrix", ".env"),
		)
	}
	for _, f := range loadEnvFiles {
		if envMap, err := godotenv.Read(f); err == nil {
			for k, v := range envMap {
				vTrim := strings.TrimSpace(v)
				if vTrim != "" && (os.Getenv(k) == "" || os.Getenv(k) == "\"\"" || os.Getenv(k) == "''") {
					_ = os.Setenv(k, vTrim)
				}
			}
		}
	}

	anthropicKey := getEnvAny("ANTHROPIC_API_KEY", "API_ANTHROPIC_API_KEY")
	openAIKey := getEnvAny("OPENAI_API_KEY", "API_OPEN_AI_API_KEY")
	geminiKey := getEnvAny("GEMINI_API_KEY", "API_GOOGLE_AI_API_KEY")
	openrouterKey := getEnvAny("OPENROUTER_API_KEY", "API_OPEN_ROUTER_API_KEY")
	deepseekKey := getEnvAny("DEEPSEEK_API_KEY", "API_DEEPSEEK_API_KEY")
	openAIBaseURL := getEnvAny("OPENAI_BASE_URL", "API_OPENAI_FORCE_BASE_URL")
	vllmEndpoint := os.Getenv("VLLM_ENDPOINT")
	localEndpoint := os.Getenv("LOCAL_LLM_ENDPOINT")

	// If no AI keys or endpoints are configured, return nil
	if anthropicKey == "" && openAIKey == "" && geminiKey == "" && openrouterKey == "" && deepseekKey == "" && localEndpoint == "" {
		return nil
	}

	opts := make([]llm.GatewayOption, 0)
	if openrouterKey != "" {
		opts = append(opts, llm.WithOpenRouter(openrouterKey))
	}
	if deepseekKey != "" {
		opts = append(opts, llm.WithDeepSeek(deepseekKey))
	}
	if openAIBaseURL != "" {
		opts = append(opts, llm.WithOpenAIBaseURL(openAIBaseURL))
	}
	if vllmEndpoint != "" {
		opts = append(opts, llm.WithVLLM(vllmEndpoint))
	}

	defaultModel := os.Getenv("AI_MODEL_DEFAULT")
	fallbackModel := os.Getenv("AI_MODEL_FALLBACK")
	if defaultModel != "" || fallbackModel != "" {
		modelsList := []string{}
		if defaultModel != "" {
			modelsList = append(modelsList, defaultModel)
		}
		if fallbackModel != "" {
			modelsList = append(modelsList, fallbackModel)
		}
		opts = append(opts, llm.WithOpenRouterModels(modelsList...))
		opts = append(opts, llm.WithOpenAIModels(modelsList...))
	}

	return llm.NewGateway(anthropicKey, openAIKey, geminiKey, localEndpoint, opts...)
}

func getEnvAny(keys ...string) string {
	for _, k := range keys {
		if val := os.Getenv(k); strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
	}
	return ""
}

// RunReview processes the provided unified diff and returns comprehensive review findings.
func (r *CLIRunner) RunReview(ctx context.Context, rawDiff string, opts CLIOptions) (*CLIResult, error) {
	startTime := time.Now()

	result := &CLIResult{
		Status:   "passed",
		Findings: make([]models.CodeFinding, 0),
	}

	if strings.TrimSpace(rawDiff) == "" {
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// 1. Check if remote API execution should be attempted
	shouldTryRemote := !opts.Offline && !opts.DryRun && opts.APIBaseURL != "" && (opts.APIKey != "" || opts.AccessToken != "")
	if shouldTryRemote {
		if remoteRes, err := r.runRemoteReview(ctx, rawDiff, opts); err == nil && remoteRes != nil {
			remoteRes.Duration = time.Since(startTime)
			r.evaluateBlockingThreshold(remoteRes, opts)
			return remoteRes, nil
		}
		// Remote failure falls through to high-fidelity local engine
	}

	// 2. Local Review Evaluation Pipeline
	patches, err := diff.ParseUnifiedDiff(strings.NewReader(rawDiff))
	if err != nil {
		return nil, fmt.Errorf("failed parsing diff: %w", err)
	}

	// Filter out files matching .scandrixignore or standard ignore patterns
	ignoreEngine := NewIgnoreEngine(opts.TargetDirectory)
	patches = ignoreEngine.FilterPatches(patches)
	result.FilesReviewed = len(patches)

	if len(patches) == 0 {
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// Evaluate deterministic rules against diff hunks
	sessionID, wsID := resolveWorkspaceID(opts)
	findings := r.evaluator.EvaluatePatches(sessionID, wsID, patches)

	// 3. Local AI Synthesis (if AI Gateway is available and not in rules-only/fast mode or prompt-only)
	if r.aiGateway != nil && !opts.RulesOnly && opts.Format != FormatPrompt {
		reviewReq := llm.ReviewRequest{
			WorkspaceID:   wsID,
			RepoNamespace: "local-workspace",
			PullTitle:     "ScanDrix CLI AI Review",
			DiffContent:   rawDiff,
		}
		aiResp, err := r.aiGateway.AnalyzeDiff(ctx, reviewReq)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ScanDrix AI Engine] AI synthesis error: %v\n", err)
		} else if aiResp != nil {
			for _, af := range aiResp.Findings {
				sev := models.SeverityMedium
				switch strings.ToUpper(af.Severity) {
				case "CRITICAL":
					sev = models.SeverityCritical
				case "HIGH":
					sev = models.SeverityHigh
				case "LOW":
					sev = models.SeverityLow
				case "INFO":
					sev = models.SeverityInfo
				}

				// Check deduplication with rule findings
				isDup := false
				for _, rf := range findings {
					if rf.FilePath == af.FilePath && rf.StartLine == af.StartLine && strings.EqualFold(rf.Title, af.Title) {
						isDup = true
						break
					}
				}

				if !isDup {
					findings = append(findings, models.CodeFinding{
						ReviewID:      sessionID,
						WorkspaceID:   wsID,
						FilePath:      af.FilePath,
						StartLine:     af.StartLine,
						EndLine:       af.EndLine,
						Severity:      sev,
						Category:      af.Category,
						Title:         af.Title,
						Description:   af.Description,
						Remediation:   af.Remediation,
						SuggestedDiff: af.SuggestedDiff,
					})
				}
			}
		}
	}

	// Filter findings if Focus is specified
	if opts.Focus != "" {
		focusedFindings := make([]models.CodeFinding, 0)
		focusLower := strings.ToLower(opts.Focus)
		for _, f := range findings {
			if strings.Contains(strings.ToLower(f.Title), focusLower) ||
				strings.Contains(strings.ToLower(f.Description), focusLower) ||
				strings.Contains(strings.ToLower(f.Category), focusLower) {
				focusedFindings = append(focusedFindings, f)
			}
		}
		findings = focusedFindings
	}

	result.Findings = findings
	result.TotalFindings = len(findings)

	// 4. Aggregate severity statistics
	for _, f := range findings {
		switch f.Severity {
		case models.SeverityCritical:
			result.CriticalCount++
		case models.SeverityHigh:
			result.HighCount++
		case models.SeverityMedium:
			result.MediumCount++
		case models.SeverityLow:
			result.LowCount++
		}
	}

	// 5. Apply automatic remediations if --fix is requested
	if opts.Fix && len(findings) > 0 {
		applied := r.applyRemediations(opts.TargetDirectory, findings)
		result.FixesApplied = applied
	}

	// 6. Evaluate blocking threshold and exit codes
	r.evaluateBlockingThreshold(result, opts)
	result.Duration = time.Since(startTime)

	return result, nil
}

// RunScan recursively audits files in the target directory without requiring a git diff.
func (r *CLIRunner) RunScan(ctx context.Context, targetDir string, opts CLIOptions) (*CLIResult, error) {
	startTime := time.Now()

	if targetDir == "" {
		targetDir = "."
	}

	result := &CLIResult{
		Status:   "passed",
		Findings: make([]models.CodeFinding, 0),
	}

	ignoreEngine := NewIgnoreEngine(targetDir)
	validExts := map[string]bool{
		".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true,
		".py": true, ".rs": true, ".java": true, ".php": true, ".cs": true,
		".rb": true, ".c": true, ".cpp": true, ".h": true, ".yaml": true,
		".yml": true, ".json": true, ".sql": true, ".env": true, ".sh": true,
	}

	sessionID, wsID := resolveWorkspaceID(opts)
	var patches []*diff.FilePatch
	filesScanned := 0
	totalLinesScanned := 0

	fi, statErr := os.Stat(targetDir)
	if statErr == nil && !fi.IsDir() {
		// Single file scanning
		content, readErr := os.ReadFile(targetDir)
		if readErr == nil && len(content) > 0 {
			lines := strings.Split(string(content), "\n")
			filesScanned = 1
			totalLinesScanned = len(lines)
			if opts.OnProgress != nil {
				opts.OnProgress(targetDir, 1, len(lines))
			}
			var diffLines []diff.DiffLine
			for idx, line := range lines {
				diffLines = append(diffLines, diff.DiffLine{
					Type:      diff.LineAddition,
					Content:   line,
					NewLineNo: idx + 1,
				})
			}
			patch := &diff.FilePatch{
				OldPath: targetDir,
				NewPath: targetDir,
				Hunks: []diff.Hunk{
					{
						NewStart: 1,
						NewLines: len(lines),
						Lines:    diffLines,
					},
				},
			}
			patches = append(patches, patch)
		}
	} else {
		err := filepath.Walk(targetDir, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return nil
			}

			// Skip directories like .git, node_modules, dist, vendor, benchmarks, fixtures
			if info.IsDir() {
				base := info.Name()
				if strings.HasPrefix(base, ".") && base != "." && base != ".." {
					return filepath.SkipDir
				}
				if base == "node_modules" || base == "dist" || base == "vendor" || base == "bin" ||
					base == "coverage" || base == "build" || base == ".next" || base == ".turbo" ||
					base == "out" || base == ".cache" || base == "benchmark" || base == "fixtures" ||
					base == "seed" || base == "golden" || base == "target" || base == "venv" ||
					base == ".venv" || base == ".idea" || base == ".vscode" {
					return filepath.SkipDir
				}
				return nil
			}

			relPath, relErr := filepath.Rel(targetDir, path)
			if relErr != nil {
				relPath = path
			}

			if ignoreEngine.ShouldIgnore(relPath) {
				return nil
			}

			// Skip unit test files, declaration files, minified bundles, and lockfiles
			if strings.HasSuffix(relPath, ".spec.ts") || strings.HasSuffix(relPath, ".test.ts") ||
				strings.HasSuffix(relPath, ".spec.js") || strings.HasSuffix(relPath, ".test.js") ||
				strings.HasSuffix(relPath, "_test.go") || strings.HasSuffix(relPath, ".d.ts") ||
				strings.HasSuffix(relPath, ".min.js") || strings.HasSuffix(relPath, ".map") ||
				strings.HasSuffix(relPath, "pnpm-lock.yaml") || strings.HasSuffix(relPath, "yarn.lock") ||
				strings.HasSuffix(relPath, "package-lock.json") {
				return nil
			}

			ext := strings.ToLower(filepath.Ext(path))
			if !validExts[ext] && !strings.HasPrefix(info.Name(), ".env") {
				return nil
			}

			// Read file lines
			content, readErr := os.ReadFile(path)
			if readErr != nil || len(content) == 0 {
				return nil
			}

			// Skip large JSON dataset files (>64KB) or files >500KB
			if (ext == ".json" && len(content) > 64*1024) || len(content) > 512*1024 || bytes.ContainsRune(content, 0) {
				return nil
			}

			lines := strings.Split(string(content), "\n")
			filesScanned++
			totalLinesScanned += len(lines)
			if opts.OnProgress != nil {
				opts.OnProgress(relPath, filesScanned, len(lines))
			}

			var diffLines []diff.DiffLine
			for idx, line := range lines {
				diffLines = append(diffLines, diff.DiffLine{
					Type:      diff.LineAddition,
					Content:   line,
					NewLineNo: idx + 1,
				})
			}

			patch := &diff.FilePatch{
				OldPath: relPath,
				NewPath: relPath,
				Hunks: []diff.Hunk{
					{
						NewStart: 1,
						NewLines: len(lines),
						Lines:    diffLines,
					},
				},
			}
			patches = append(patches, patch)
			return nil
		})

		if err != nil {
			return nil, fmt.Errorf("scan failed during filesystem walk: %w", err)
		}
	}

	result.FilesReviewed = filesScanned
	result.TotalLines = totalLinesScanned

	if len(patches) == 0 {
		result.Duration = time.Since(startTime)
		return result, nil
	}

	if opts.OnStatus != nil {
		opts.OnStatus(fmt.Sprintf("Evaluating security AST & semantic rules across %d files...", len(patches)))
	}

	// Evaluate rules against all scanned file patches
	findings := r.evaluator.EvaluatePatches(sessionID, wsID, patches)

	// Deep AI semantic synthesis across scanned files (if AI Gateway is available and not in rules-only/fast/offline mode)
	if r.aiGateway != nil && !opts.RulesOnly && !opts.Fast && !opts.Offline {
		if opts.OnStatus != nil {
			opts.OnStatus("Synthesizing multi-file security posture with AI engine...")
		}

		// Build combined diff summary of critical security-sensitive files (auth, api, controllers, db, routes)
		var combinedDiff strings.Builder
		diffSize := 0
		const maxScanDiffBytes = 32 * 1024 // 32KB optimal chunk for responsive multi-file AI synthesis

		filesWithFindings := make(map[string]bool)
		for _, f := range findings {
			filesWithFindings[f.FilePath] = true
		}

		for _, p := range patches {
			pathLower := strings.ToLower(p.NewPath)
			isSensitive := filesWithFindings[p.NewPath] ||
				strings.Contains(pathLower, "auth") ||
				strings.Contains(pathLower, "login") ||
				strings.Contains(pathLower, "token") ||
				strings.Contains(pathLower, "secret") ||
				strings.Contains(pathLower, "api") ||
				strings.Contains(pathLower, "controller") ||
				strings.Contains(pathLower, "db") ||
				strings.Contains(pathLower, "repo") ||
				strings.Contains(pathLower, "store") ||
				strings.Contains(pathLower, "database") ||
				strings.Contains(pathLower, "exec") ||
				strings.Contains(pathLower, "cmd") ||
				strings.Contains(pathLower, "handler") ||
				strings.Contains(pathLower, "service") ||
				strings.Contains(pathLower, "route")

			if isSensitive || len(findings) == 0 {
				var patchBuf strings.Builder
				patchBuf.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n", p.OldPath, p.NewPath, p.OldPath, p.NewPath))
				for _, h := range p.Hunks {
					patchBuf.WriteString(fmt.Sprintf("@@ -1,0 +%d,%d @@\n", h.NewStart, h.NewLines))
					for _, l := range h.Lines {
						patchBuf.WriteString("+")
						patchBuf.WriteString(l.Content)
						patchBuf.WriteString("\n")
						if diffSize+patchBuf.Len() > maxScanDiffBytes {
							break
						}
					}
				}
				patchText := patchBuf.String()
				if diffSize+len(patchText) <= maxScanDiffBytes {
					combinedDiff.WriteString(patchText)
					combinedDiff.WriteString("\n")
					diffSize += len(patchText)
					if opts.OnStatus != nil {
						opts.OnStatus(fmt.Sprintf("Analyzed %s", p.NewPath))
					}
				}
			}
		}

		if combinedDiff.Len() > 0 {
			reviewReq := llm.ReviewRequest{
				WorkspaceID:   wsID,
				RepoNamespace: "local-scan",
				PullTitle:     "ScanDrix Repository Deep Scan",
				DiffContent:   combinedDiff.String(),
			}
			aiCtx, aiCancel := context.WithTimeout(ctx, 90*time.Second)
			aiResp, err := r.aiGateway.AnalyzeDiff(aiCtx, reviewReq)
			aiCancel()
			if err != nil {
				if opts.OnStatus != nil {
					opts.OnStatus(fmt.Sprintf("AI engine skipped (%v) - relying on local AST rule engine", err))
				}
			} else if aiResp != nil {
				if opts.OnStatus != nil {
					opts.OnStatus(fmt.Sprintf("AI engine synthesized %d deep architectural findings", len(aiResp.Findings)))
				}
				var aiFindings []models.CodeFinding
				fallbackPath := ""
				if len(patches) > 0 {
					fallbackPath = patches[0].NewPath
				}

				for _, af := range aiResp.Findings {
					sev := models.SeverityMedium
					switch strings.ToUpper(af.Severity) {
					case "CRITICAL":
						sev = models.SeverityCritical
					case "HIGH":
						sev = models.SeverityHigh
					case "LOW":
						sev = models.SeverityLow
					case "INFO":
						sev = models.SeverityInfo
					}

					filePath := af.FilePath
					if filePath == "" {
						filePath = fallbackPath
					}
					startLine := af.StartLine
					if startLine <= 0 {
						startLine = 1
					}
					endLine := af.EndLine
					if endLine < startLine {
						endLine = startLine
					}

					category := "AI_ARCHITECTURAL"
					if af.Category != "" {
						category = "AI / " + strings.ToUpper(af.Category)
					}

					isDup := false
					for _, rf := range findings {
						if rf.FilePath == filePath && rf.StartLine == startLine && strings.EqualFold(rf.Title, af.Title) {
							isDup = true
							break
						}
					}

					if !isDup {
						aiFindings = append(aiFindings, models.CodeFinding{
							ReviewID:      sessionID,
							WorkspaceID:   wsID,
							FilePath:      filePath,
							StartLine:     startLine,
							EndLine:       endLine,
							Severity:      sev,
							Category:      category,
							Title:         af.Title,
							Description:   af.Description,
							Remediation:   af.Remediation,
							SuggestedDiff: af.SuggestedDiff,
						})
					}
				}

				// Place AI architectural findings at the very front so they are prominently featured
				if len(aiFindings) > 0 {
					findings = append(aiFindings, findings...)
				}
			}
		}
	}

	// Filter findings if Focus is specified
	if opts.Focus != "" {
		focusedFindings := make([]models.CodeFinding, 0)
		focusLower := strings.ToLower(opts.Focus)
		for _, f := range findings {
			if strings.Contains(strings.ToLower(f.Title), focusLower) ||
				strings.Contains(strings.ToLower(f.Description), focusLower) ||
				strings.Contains(strings.ToLower(f.Category), focusLower) {
				focusedFindings = append(focusedFindings, f)
			}
		}
		findings = focusedFindings
	}

	// Sort findings: Critical > High > Medium > Low > Info
	// Within the same severity: AI architectural findings first
	sort.SliceStable(findings, func(i, j int) bool {
		sevWeight := map[models.FindingSeverity]int{
			models.SeverityCritical: 4,
			models.SeverityHigh:     3,
			models.SeverityMedium:   2,
			models.SeverityLow:      1,
			models.SeverityInfo:     0,
		}
		wi := sevWeight[findings[i].Severity]
		wj := sevWeight[findings[j].Severity]
		if wi != wj {
			return wi > wj
		}
		iIsAI := strings.Contains(findings[i].Category, "AI") || strings.Contains(strings.ToUpper(findings[i].Category), "SECURITY") || strings.Contains(strings.ToUpper(findings[i].Title), "AI")
		jIsAI := strings.Contains(findings[j].Category, "AI") || strings.Contains(strings.ToUpper(findings[j].Category), "SECURITY") || strings.Contains(strings.ToUpper(findings[j].Title), "AI")
		if iIsAI != jIsAI {
			return iIsAI
		}
		return findings[i].FilePath < findings[j].FilePath
	})

	result.Findings = findings
	result.TotalFindings = len(findings)

	// Aggregate severity statistics
	for _, f := range findings {
		switch f.Severity {
		case models.SeverityCritical:
			result.CriticalCount++
		case models.SeverityHigh:
			result.HighCount++
		case models.SeverityMedium:
			result.MediumCount++
		case models.SeverityLow:
			result.LowCount++
		}
	}

	// Apply automatic remediations if --fix is requested
	if opts.Fix && len(findings) > 0 {
		applied := r.applyRemediations(targetDir, findings)
		result.FixesApplied = applied
	}

	r.evaluateBlockingThreshold(result, opts)
	result.Duration = time.Since(startTime)

	return result, nil
}

func (r *CLIRunner) evaluateBlockingThreshold(result *CLIResult, opts CLIOptions) {
	threshold := opts.SeverityThreshold
	if threshold == "" {
		threshold = models.SeverityHigh
	}

	isBlocking := false
	switch threshold {
	case models.SeverityCritical:
		if result.CriticalCount > 0 {
			isBlocking = true
		}
	case models.SeverityHigh:
		if result.CriticalCount > 0 || result.HighCount > 0 {
			isBlocking = true
		}
	case models.SeverityMedium:
		if result.CriticalCount > 0 || result.HighCount > 0 || result.MediumCount > 0 {
			isBlocking = true
		}
	case models.SeverityLow, models.SeverityInfo:
		if result.TotalFindings > 0 {
			isBlocking = true
		}
	}

	result.IsBlocking = isBlocking
	if isBlocking && !opts.DryRun {
		result.Status = "failed"
		result.ExitCode = 1
	} else {
		result.Status = "passed"
		result.ExitCode = 0
	}
}

func (r *CLIRunner) runRemoteReview(ctx context.Context, rawDiff string, opts CLIOptions) (*CLIResult, error) {
	endpoint := strings.TrimRight(opts.APIBaseURL, "/") + "/api/v1/reviews"
	payload := map[string]any{
		"raw_diff": rawDiff,
		"title":    "ScanDrix CLI Automated Review",
		"options": map[string]any{
			"fast":       opts.Fast,
			"heavy":      opts.Heavy,
			"rules_only": opts.RulesOnly,
			"focus":      opts.Focus,
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ScanDrix-CLI/v1.2.0")

	if opts.APIKey != "" {
		req.Header.Set("X-Team-Key", opts.APIKey)
		req.Header.Set("X-API-Key", opts.APIKey)
		req.Header.Set("X-Workspace-Key", opts.APIKey)
		if !strings.HasPrefix(opts.APIKey, "Bearer ") {
			req.Header.Set("Authorization", "Bearer "+opts.APIKey)
		}
	} else if opts.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+opts.AccessToken)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("remote API returned status %d", resp.StatusCode)
	}

	var remoteResp struct {
		ReviewID string               `json:"review_id"`
		Status   string               `json:"status"`
		Findings []models.CodeFinding `json:"findings,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&remoteResp); err != nil {
		return nil, err
	}

	if remoteResp.ReviewID != "" && len(remoteResp.Findings) == 0 && (resp.StatusCode == http.StatusAccepted || remoteResp.Status == "PROCESSING") {
		pollEndpoint := fmt.Sprintf("%s/api/v1/reviews/%s/findings", strings.TrimRight(opts.APIBaseURL, "/"), remoteResp.ReviewID)
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		timeout := time.After(45 * time.Second)

	pollLoop:
		for {
			select {
			case <-ctx.Done():
				break pollLoop
			case <-timeout:
				break pollLoop
			case <-ticker.C:
				pReq, err := http.NewRequestWithContext(ctx, http.MethodGet, pollEndpoint, nil)
				if err != nil {
					continue
				}
				pReq.Header.Set("User-Agent", "ScanDrix-CLI/v1.2.0")
				if opts.APIKey != "" {
					pReq.Header.Set("X-Team-Key", opts.APIKey)
					pReq.Header.Set("X-API-Key", opts.APIKey)
					pReq.Header.Set("X-Workspace-Key", opts.APIKey)
					if !strings.HasPrefix(opts.APIKey, "Bearer ") {
						pReq.Header.Set("Authorization", "Bearer "+opts.APIKey)
					}
				} else if opts.AccessToken != "" {
					pReq.Header.Set("Authorization", "Bearer "+opts.AccessToken)
				}
				pResp, err := r.httpClient.Do(pReq)
				if err != nil {
					continue
				}
				if pResp.StatusCode == http.StatusOK {
					var findings []models.CodeFinding
					if err := json.NewDecoder(pResp.Body).Decode(&findings); err == nil {
						pResp.Body.Close()
						if len(findings) > 0 {
							remoteResp.Findings = findings
							remoteResp.Status = "COMPLETED"
							break pollLoop
						}
					} else {
						pResp.Body.Close()
					}
				} else {
					pResp.Body.Close()
				}
			}
		}
	}

	res := &CLIResult{
		Status:        remoteResp.Status,
		Findings:      remoteResp.Findings,
		TotalFindings: len(remoteResp.Findings),
	}
	for _, f := range remoteResp.Findings {
		switch f.Severity {
		case models.SeverityCritical:
			res.CriticalCount++
		case models.SeverityHigh:
			res.HighCount++
		case models.SeverityMedium:
			res.MediumCount++
		case models.SeverityLow:
			res.LowCount++
		}
	}
	return res, nil
}

func (r *CLIRunner) applyRemediations(targetDir string, findings []models.CodeFinding) int {
	if targetDir == "" {
		targetDir = "."
	}
	appliedCount := 0

	// Group findings by file and sort in descending line order (bottom-to-top)
	// so earlier line indices remain valid as lines are replaced.
	byFile := make(map[string][]models.CodeFinding)
	for _, f := range findings {
		if f.FilePath == "" || strings.TrimSpace(f.SuggestedDiff) == "" {
			continue
		}
		byFile[f.FilePath] = append(byFile[f.FilePath], f)
	}

	for _, fileFindings := range byFile {
		sort.Slice(fileFindings, func(i, j int) bool {
			return fileFindings[i].StartLine > fileFindings[j].StartLine
		})
		for _, f := range fileFindings {
			if _, err := ApplyFindingFix(targetDir, f); err == nil {
				appliedCount++
			}
		}
	}
	return appliedCount
}

func resolveWorkspaceID(opts CLIOptions) (sessionID, wsID uuid.UUID) {
	sessionID = uuid.New()
	wsID = uuid.New()
	if opts.AccessToken != "" {
		parts := strings.Split(opts.AccessToken, ".")
		if len(parts) >= 2 {
			if payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1]); err == nil {
				var claims struct {
					WS string `json:"ws"`
				}
				if json.Unmarshal(payloadBytes, &claims) == nil && claims.WS != "" {
					if parsed, err := uuid.Parse(claims.WS); err == nil && parsed != uuid.Nil {
						wsID = parsed
					}
				}
			}
		}
	}
	return sessionID, wsID
}
