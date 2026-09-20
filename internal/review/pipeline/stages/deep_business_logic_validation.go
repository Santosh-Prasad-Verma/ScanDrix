package stages

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/pipeline"
)

const (
	// NoTaskMCPSentinel is returned when preflight finds no task management integration.
	NoTaskMCPSentinel = "NO_TASK_MCP_CONNECTED"

	// WeakTaskContextMarker indicates that the task description lacks sufficient detail for validation.
	WeakTaskContextMarker = "<!-- drixy-weak-task-context -->"

	// BusinessLogicTimeout is the maximum duration allocated for business rules validation.
	BusinessLogicTimeout = 5 * time.Minute
)

var (
	requirementKeywords = []string{
		"requirement",
		"acceptance criteria",
		"user story",
		"given",
		"when",
		"then",
	}

	ticketKeyRegex = regexp.MustCompile(`(?i)[A-Za-z][A-Za-z0-9_]+-\d+`)
	issueRefRegex  = regexp.MustCompile(`(?:^|[\s(])#(\d+)\b`)
	issueURLRegex  = regexp.MustCompile(`(?i)https?://[^\s)>\]"']*/issues/(\d+)`)
	taskLinkRegex  = regexp.MustCompile(`https?://[^\s)>\]"']+`)

	managedTaskMCPHints = map[string]string{
		"scandrix-issues-default": "gitissues",
		"linear-default":          "linear",
		"atlassian-rovo-default":  "atlassianrovo",
		"notion-default":          "notion",
	}

	ticketKeyMCPs = []string{
		"jira",
		"linear",
		"clickup",
		"githubissues",
		"gitissues",
		"atlassianrovo",
	}

	taskManagementHints = []string{
		"jira",
		"linear",
		"notion",
		"clickup",
		"googledocs",
		"atlassianrovo",
		"githubissues",
		"gitissues",
	}

	mcpURLPatterns = map[string][]string{
		"jira":          {"atlassian.net", "jira."},
		"linear":        {"linear.app"},
		"notion":        {"notion.so", "notion.site"},
		"clickup":       {"clickup.com"},
		"googledocs":    {"docs.google.com"},
		"githubissues":  {"github.com"},
		"atlassianrovo": {"atlassian.net"},
	}

	limitationIndicators = []string{
		"need task information",
		"need pull request diff",
		"insufficient task context",
		"limited task context",
		"could not validate",
		"without the actual code changes, i can",
		"preciso do diff da pull request",
		"preciso de informacoes da task",
		"contexto insuficiente da task",
		"contexto limitado da task",
		"nao consegui validar",
		"sem as alteracoes de codigo",
		"mcp connection failed",
		"mcp integration required",
		"no compatible mcp integration",
	}

	noGapIndicators = []string{
		"no gaps",
		"no issues",
		"fully compliant",
		"no business logic gap",
		"all requirements met",
		"implementation is complete",
		"no violations",
		"✅ compliant",
		"status: ✅",
		"sem bloqueios identificados",
		"requirements covered",
		`"needsmoreinfo": false`,
	}
)

// BusinessSignals carries extracted requirements, tickets, and external tracking links.
type BusinessSignals struct {
	TicketKeys          []string `json:"ticket_keys"`
	TaskLinks           []string `json:"task_links"`
	RequirementKeywords []string `json:"requirement_keywords"`
}

// BusinessRulesValidationInput holds parameters supplied to the business rules agent.
type BusinessRulesValidationInput struct {
	UserQuestion    string          `json:"user_question"`
	PRNumber        int             `json:"pr_number"`
	HeadRef         string          `json:"head_ref"`
	BaseRef         string          `json:"base_ref"`
	RepositoryID    uuid.UUID       `json:"repository_id"`
	RepositoryName  string          `json:"repository_name"`
	PRDescription   string          `json:"pr_description"`
	PlatformType    string          `json:"platform_type"`
	DefaultBranch   string          `json:"default_branch"`
	BusinessSignals BusinessSignals `json:"business_signals"`
	OrgID           string          `json:"org_id"`
	TeamID          string          `json:"team_id"`
	ByokModel       string          `json:"byok_model,omitempty"`
	ByokModelID     string          `json:"byok_model_id,omitempty"`
	ThreadID        string          `json:"thread_id,omitempty"`
}

// BusinessRulesValidationAgent defines the agent invocation signature.
type BusinessRulesValidationAgent interface {
	Execute(ctx context.Context, input BusinessRulesValidationInput) (string, error)
}

// MCPConnection represents a connected tool or external service integration.
type MCPConnection struct {
	OrganizationID string `json:"organization_id"`
	IntegrationID  string `json:"integration_id"`
	Category       string `json:"category"`
	AppName        string `json:"app_name"`
	Provider       string `json:"provider"`
	IsConnected    bool   `json:"is_connected"`
	IsActive       bool   `json:"is_active"`
}

// MCPManagerService queries active tool connections for an organization.
type MCPManagerService interface {
	GetConnections(ctx context.Context, orgID, teamID string) ([]MCPConnection, error)
}

// DeepBusinessLogicValidationStage validates that PR code satisfies requirements declared
// in the PR description, linked tickets, or specifications.
type DeepBusinessLogicValidationStage struct {
	logger     *slog.Logger
	agent      BusinessRulesValidationAgent
	mcpManager MCPManagerService
}

// NewDeepBusinessLogicValidationStage instantiates the stage.
func NewDeepBusinessLogicValidationStage(
	logger *slog.Logger,
	agent BusinessRulesValidationAgent,
	mcpManager MCPManagerService,
) *DeepBusinessLogicValidationStage {
	if logger == nil {
		logger = slog.Default()
	}
	return &DeepBusinessLogicValidationStage{
		logger:     logger.With("stage", "DeepBusinessLogicValidationStage"),
		agent:      agent,
		mcpManager: mcpManager,
	}
}

// Name returns stage identifier.
func (s *DeepBusinessLogicValidationStage) Name() string {
	return "DeepBusinessLogicValidationStage"
}

// Execute performs validation against linked business rules and specifications.
func (s *DeepBusinessLogicValidationStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	s.logger.Info("[BUSINESS-LOGIC] Stage entered — evaluating run conditions",
		"workspace_id", pCtx.WorkspaceID.String(),
		"pr_number", pCtx.PullNumber,
		"repository_id", pCtx.RepositoryID.String(),
		"business_logic_enabled", pCtx.ResolvedConfig.ReviewOptions.BusinessLogic,
		"has_pr_body", pCtx.Description != "",
	)

	skipDecision := s.evaluateSkip(ctx, pCtx)
	if skipDecision != nil {
		s.logger.Info("[BUSINESS-LOGIC] Skipped business logic validation",
			"reason", skipDecision.Reason,
			"message", skipDecision.Message,
		)
		pCtx.BusinessLogicResults = []domain.CodeSuggestion{}
		pCtx.BusinessLogicOutcome = &pipeline.BusinessLogicOutcomeInfo{
			Kind:    "skipped",
			Reason:  skipDecision.Reason,
			Message: skipDecision.Message,
		}
		return nil
	}

	prBody := pCtx.Description
	combinedText := s.buildSignalSources(pCtx)
	signals := s.detectSignals(combinedText, prBody)
	prBodyHash := s.computePrBodyHash(prBody)

	threadID := s.createBusinessLogicThread(pCtx)
	input := BusinessRulesValidationInput{
		UserQuestion:   "@drixy -v business-logic",
		PRNumber:       pCtx.PullNumber,
		HeadRef:        pCtx.Branch,
		BaseRef:        pCtx.BaseBranch,
		RepositoryID:   pCtx.RepositoryID,
		RepositoryName: pCtx.RepoNamespace,
		PRDescription:  prBody,
		PlatformType:   string(pCtx.Provider),
		DefaultBranch:  pCtx.BaseBranch,
		BusinessSignals: BusinessSignals{
			TicketKeys:          signals.TicketKeys,
			TaskLinks:           signals.TaskLinks,
			RequirementKeywords: signals.RequirementKeywords,
		},
		OrgID:       pCtx.WorkspaceID.String(),
		TeamID:      pCtx.RepositoryID.String(),
		ByokModel:   pCtx.ResolvedConfig.ByokModel,
		ByokModelID: pCtx.ResolvedConfig.ByokModelID,
		ThreadID:    threadID,
	}

	s.logger.Info("[BUSINESS-LOGIC] Running business-rules validation agent",
		"ticket_keys_count", len(signals.TicketKeys),
		"task_links_count", len(signals.TaskLinks),
		"keywords_count", len(signals.RequirementKeywords),
	)

	execCtx, cancel := context.WithTimeout(ctx, BusinessLogicTimeout)
	defer cancel()

	resultChan := make(chan string, 1)
	errChan := make(chan error, 1)

	go func() {
		res, err := s.agent.Execute(execCtx, input)
		if err != nil {
			errChan <- err
			return
		}
		resultChan <- res
	}()

	select {
	case <-execCtx.Done():
		err := execCtx.Err()
		s.logger.Error("[BUSINESS-LOGIC] Validation timed out or canceled", "error", err)
		pCtx.AddError(s.Name(), "BusinessRulesValidationAgent", err, "partial", map[string]interface{}{
			"pr_number": pCtx.PullNumber,
			"timeout":   true,
		})
		pCtx.BusinessLogicResults = []domain.CodeSuggestion{}
		pCtx.BusinessLogicOutcome = &pipeline.BusinessLogicOutcomeInfo{
			Kind:    "error",
			Message: fmt.Sprintf("Business logic validation failed: %v", err),
		}
		return nil

	case err := <-errChan:
		s.logger.Error("[BUSINESS-LOGIC] Agent execution error", "error", err)
		pCtx.AddError(s.Name(), "BusinessRulesValidationAgent", err, "partial", map[string]interface{}{
			"pr_number": pCtx.PullNumber,
		})
		pCtx.BusinessLogicResults = []domain.CodeSuggestion{}
		pCtx.BusinessLogicOutcome = &pipeline.BusinessLogicOutcomeInfo{
			Kind:    "error",
			Message: fmt.Sprintf("Business logic validation failed: %v", err),
		}
		return nil

	case result := <-resultChan:
		if result == NoTaskMCPSentinel {
			s.logger.Info("[BUSINESS-LOGIC] Skipped — no task-management MCP connected")
			pCtx.BusinessLogicResults = []domain.CodeSuggestion{}
			pCtx.BusinessLogicOutcome = &pipeline.BusinessLogicOutcomeInfo{
				Kind:    "skipped",
				Reason:  "no_task_mcp",
				Message: "Skipped: no task-management MCP connected.",
			}
			return nil
		}

		classificationKind, classificationMsg := s.classifyResult(result)

		if classificationKind == "limitation" {
			s.logger.Warn("[BUSINESS-LOGIC] Agent could not validate",
				"message", classificationMsg,
			)

			if s.isWeakTaskContext(result) {
				limitationSuggestion := domain.CodeSuggestion{
					ID:                 uuid.New(),
					SuggestionContent:  result,
					OneSentenceSummary: "Task description is insufficient for business logic validation.",
					Label:              "business_logic",
					Severity:           domain.SeverityMedium,
					DeliveryStatus:     domain.DeliveryStatusNotSent,
					CreatedAt:          time.Now().UTC(),
					UpdatedAt:          time.Now().UTC(),
				}
				pCtx.BusinessLogicResults = []domain.CodeSuggestion{limitationSuggestion}
				pCtx.BusinessLogicOutcome = &pipeline.BusinessLogicOutcomeInfo{
					Kind:    "skipped",
					Reason:  "weak_task_context",
					Message: fmt.Sprintf("Skipped: task context is too weak for validation (%s).", classificationMsg),
				}
				return nil
			}

			pCtx.BusinessLogicResults = []domain.CodeSuggestion{}
			pCtx.BusinessLogicOutcome = &pipeline.BusinessLogicOutcomeInfo{
				Kind:    "skipped",
				Reason:  "agent_limitation",
				Message: fmt.Sprintf("Skipped: business logic validation could not run (%s).", classificationMsg),
			}
			return nil
		}

		if classificationKind == "no_gap" {
			noGapSuggestion := domain.CodeSuggestion{
				ID:                 uuid.New(),
				SuggestionContent:  result,
				OneSentenceSummary: "Business logic validation passed — PR aligns with task requirements.",
				Label:              "business_logic",
				Severity:           domain.SeverityLow,
				DeliveryStatus:     domain.DeliveryStatusNotSent,
				CreatedAt:          time.Now().UTC(),
				UpdatedAt:          time.Now().UTC(),
			}
			pCtx.BusinessLogicResults = []domain.CodeSuggestion{noGapSuggestion}
			pCtx.BusinessLogicPrBodyHash = prBodyHash
			pCtx.BusinessLogicOutcome = &pipeline.BusinessLogicOutcomeInfo{
				Kind:    "success",
				Message: "PR aligns with the requirements stated in the description.",
			}
			return nil
		}

		// Gap found
		gapSuggestion := domain.CodeSuggestion{
			ID:                 uuid.New(),
			SuggestionContent:  result,
			OneSentenceSummary: "Business logic gap detected based on PR requirements.",
			Label:              "business_logic",
			Severity:           domain.SeverityMedium,
			DeliveryStatus:     domain.DeliveryStatusNotSent,
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
		}
		pCtx.BusinessLogicResults = []domain.CodeSuggestion{gapSuggestion}
		pCtx.BusinessLogicPrBodyHash = prBodyHash
		pCtx.BusinessLogicOutcome = &pipeline.BusinessLogicOutcomeInfo{
			Kind:    "gap_found",
			Message: "Business logic gap detected — see PR-level comment.",
		}
		return nil
	}
}

type skipDecision struct {
	Reason  string
	Message string
}

func (s *DeepBusinessLogicValidationStage) evaluateSkip(ctx context.Context, pCtx *pipeline.PipelineContext) *skipDecision {
	if pCtx.WorkspaceID == uuid.Nil {
		return &skipDecision{
			Reason:  "missing_org",
			Message: "Missing organization context.",
		}
	}
	if pCtx.PullNumber == 0 {
		return &skipDecision{
			Reason:  "missing_pr",
			Message: "Missing pull request data.",
		}
	}
	if pCtx.RepositoryID == uuid.Nil {
		return &skipDecision{
			Reason:  "missing_repo",
			Message: "Missing repository data.",
		}
	}

	if !pCtx.ResolvedConfig.ReviewOptions.BusinessLogic {
		return &skipDecision{
			Reason:  "option_off",
			Message: "Business logic validation is disabled in the code review configuration.",
		}
	}

	connectedMCPs := s.getConnectedTaskManagementMCPs(ctx, pCtx)
	if len(connectedMCPs) == 0 {
		return &skipDecision{
			Reason:  "no_task_mcp",
			Message: "Skipped: no task-management MCP connected (Jira, Atlassian Rovo, Linear, Notion, ClickUp, etc.).",
		}
	}

	combinedText := s.buildSignalSources(pCtx)
	if !s.hasRelevantBusinessSignals(combinedText, connectedMCPs) {
		return &skipDecision{
			Reason:  "no_signals",
			Message: "Skipped: no ticket key or task link matching a connected MCP found in the PR description, title or branch name.",
		}
	}

	currentHash := s.computePrBodyHash(pCtx.Description)
	var lastHash string
	if pCtx.LastExecution != nil && pCtx.PipelineMetadata != nil {
		if h, ok := pCtx.PipelineMetadata["businessLogicHash"].(string); ok {
			lastHash = h
		}
	}

	forceFullRerun := pCtx.Origin == "command-force"
	if pCtx.PipelineMetadata != nil {
		if force, ok := pCtx.PipelineMetadata["forceFullRerun"].(bool); ok && force {
			forceFullRerun = true
		}
	}

	if !forceFullRerun && lastHash != "" && lastHash == currentHash {
		return &skipDecision{
			Reason:  "unchanged_body",
			Message: "Skipped: PR description has not changed since the last review.",
		}
	}

	return nil
}

func (s *DeepBusinessLogicValidationStage) buildSignalSources(pCtx *pipeline.PipelineContext) string {
	parts := []string{}
	if pCtx.Description != "" {
		parts = append(parts, pCtx.Description)
	}
	if pCtx.Title != "" {
		parts = append(parts, pCtx.Title)
	}
	if pCtx.Branch != "" {
		parts = append(parts, pCtx.Branch)
	}
	return strings.Join(parts, " ")
}

func (s *DeepBusinessLogicValidationStage) detectSignals(combined, body string) BusinessSignals {
	return BusinessSignals{
		TicketKeys:          s.detectTicketKeys(combined),
		TaskLinks:           s.detectTaskLinks(combined),
		RequirementKeywords: s.detectRequirementKeywords(body),
	}
}

func (s *DeepBusinessLogicValidationStage) getConnectedTaskManagementMCPs(ctx context.Context, pCtx *pipeline.PipelineContext) []string {
	if s.mcpManager == nil {
		return []string{}
	}

	connections, err := s.mcpManager.GetConnections(ctx, pCtx.WorkspaceID.String(), pCtx.RepositoryID.String())
	if err != nil {
		s.logger.Warn("[BUSINESS-LOGIC] Failed to fetch MCP connections", "error", err)
		return []string{}
	}

	matched := []string{}
	for _, conn := range connections {
		if !conn.IsConnected && !conn.IsActive {
			continue
		}

		if conn.Category == "task-management" {
			if canonical, ok := managedTaskMCPHints[conn.IntegrationID]; ok {
				if !containsString(matched, canonical) {
					matched = append(matched, canonical)
				}
				continue
			}
		}

		s.appendTaskManagementHints(&matched, []string{conn.AppName, conn.Provider, conn.IntegrationID})
	}

	return matched
}

func (s *DeepBusinessLogicValidationStage) appendTaskManagementHints(matched *[]string, aliases []string) {
	for _, alias := range aliases {
		normalized := s.normalizeMCPAlias(alias)
		if normalized == "" {
			continue
		}
		for _, hint := range taskManagementHints {
			if strings.Contains(normalized, hint) || strings.Contains(hint, normalized) {
				if !containsString(*matched, hint) {
					*matched = append(*matched, hint)
				}
			}
		}
	}
}

func (s *DeepBusinessLogicValidationStage) normalizeMCPAlias(val string) string {
	val = strings.ToLower(strings.TrimSpace(val))
	var sb strings.Builder
	for _, r := range val {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func (s *DeepBusinessLogicValidationStage) hasRelevantBusinessSignals(combinedText string, connectedMCPs []string) bool {
	ticketKeys := s.detectTicketKeys(combinedText)
	if len(ticketKeys) > 0 {
		for _, mcp := range connectedMCPs {
			if containsString(ticketKeyMCPs, mcp) {
				return true
			}
		}
	}

	// Git issue references (#123)
	hasGitIssuesMCP := containsString(connectedMCPs, "gitissues") || containsString(connectedMCPs, "githubissues")
	if hasGitIssuesMCP && issueRefRegex.MatchString(combinedText) {
		return true
	}

	// URLs matching connected MCP domains
	urls := s.detectTaskLinks(combinedText)
	for _, rawURL := range urls {
		parsed, err := url.Parse(rawURL)
		if err != nil {
			continue
		}
		host := strings.ToLower(parsed.Host)
		for _, mcp := range connectedMCPs {
			patterns := mcpURLPatterns[mcp]
			for _, pat := range patterns {
				if strings.Contains(host, pat) {
					return true
				}
			}
		}
	}

	return false
}

func (s *DeepBusinessLogicValidationStage) detectTicketKeys(text string) []string {
	keysMap := make(map[string]struct{})

	// Standard Jira/Linear style PROJECT-123
	matches := ticketKeyRegex.FindAllString(text, -1)
	for _, m := range matches {
		keysMap[strings.ToUpper(m)] = struct{}{}
	}

	// Issue ref (#123)
	issueRefs := issueRefRegex.FindAllStringSubmatch(text, -1)
	for _, ref := range issueRefs {
		if len(ref) > 1 {
			keysMap["#"+ref[1]] = struct{}{}
		}
	}

	// Full issue URLs (github.com/owner/repo/issues/123)
	issueURLs := issueURLRegex.FindAllStringSubmatch(text, -1)
	for _, u := range issueURLs {
		if len(u) > 1 {
			keysMap["#"+u[1]] = struct{}{}
		}
	}

	keys := make([]string, 0, len(keysMap))
	for k := range keysMap {
		keys = append(keys, k)
	}
	return keys
}

func (s *DeepBusinessLogicValidationStage) detectTaskLinks(text string) []string {
	return taskLinkRegex.FindAllString(text, -1)
}

func (s *DeepBusinessLogicValidationStage) detectRequirementKeywords(body string) []string {
	lower := strings.ToLower(body)
	matched := []string{}
	for _, kw := range requirementKeywords {
		if strings.Contains(lower, kw) {
			matched = append(matched, kw)
		}
	}
	return matched
}

func (s *DeepBusinessLogicValidationStage) computePrBodyHash(body string) string {
	h := sha256.Sum256([]byte(body))
	return hex.EncodeToString(h[:])
}

func (s *DeepBusinessLogicValidationStage) classifyResult(result string) (kind string, message string) {
	if strings.TrimSpace(result) == "" {
		return "limitation", "Business logic agent returned an empty response."
	}

	lower := strings.ToLower(result)
	for _, indicator := range limitationIndicators {
		if strings.Contains(lower, indicator) {
			return "limitation", s.firstNonEmptyLine(result)
		}
	}

	for _, indicator := range noGapIndicators {
		if strings.Contains(lower, indicator) {
			return "no_gap", ""
		}
	}

	return "gap_found", ""
}

func (s *DeepBusinessLogicValidationStage) isWeakTaskContext(result string) bool {
	return strings.Contains(result, WeakTaskContextMarker)
}

func (s *DeepBusinessLogicValidationStage) firstNonEmptyLine(text string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		trimmed = strings.TrimLeft(trimmed, "# \t")
		trimmed = strings.TrimRight(trimmed, ": \t")
		if trimmed != "" {
			if len(trimmed) > 240 {
				return trimmed[:237] + "…"
			}
			return trimmed
		}
	}
	if len(text) > 240 {
		return text[:237] + "…"
	}
	return text
}

func (s *DeepBusinessLogicValidationStage) createBusinessLogicThread(pCtx *pipeline.PipelineContext) string {
	user := pCtx.Author
	if user == "" {
		user = "anon"
	}
	return fmt.Sprintf("vbl:%s:%s:%s:%d:%s",
		pCtx.WorkspaceID.String(),
		pCtx.RepositoryID.String(),
		pCtx.RepoNamespace,
		pCtx.PullNumber,
		user,
	)
}

func containsString(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
