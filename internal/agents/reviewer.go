package agents

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/agents/deliberation"
	"github.com/scandrix/backend/internal/codeanalysis/languages"
	"github.com/scandrix/backend/internal/drixy"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/internal/sandbox/syntax"
	"github.com/scandrix/backend/internal/usecases/feedback"
	"github.com/scandrix/backend/pkg/models"
)

// AgentThought records an individual reasoning step during automated review deliberation.
type AgentThought struct {
	StepIndex   int       `json:"step_index"`
	Phase       string    `json:"phase"` // "PLAN", "ANALYSIS", "DELIBERATION", "SYNTHESIS"
	Observation string    `json:"observation"`
	Action      string    `json:"action"`
	Timestamp   time.Time `json:"timestamp"`
}

// ReviewPlan represents the decomposition of files to review based on risk and language.
type ReviewPlan struct {
	ReviewID        uuid.UUID `json:"review_id"`
	HighRiskFiles   []string  `json:"high_risk_files"`
	StandardFiles   []string  `json:"standard_files"`
	SuppressedFiles []string  `json:"suppressed_files"`
	EstimatedTokens int       `json:"estimated_tokens"`
}

// AutonomousReviewer coordinates multi-turn agentic code analysis.
type AutonomousReviewer struct {
	ruleEvaluator   *rules.Evaluator
	llmGateway      *llm.Gateway
	deliberator     *deliberation.AgentDeliberator
	syntaxValidator *syntax.SandboxSyntaxValidator
	feedbackTracker *feedback.FeedbackTracker
	maxIterations   int
}

// NewAutonomousReviewer initializes the autonomous review agent.
func NewAutonomousReviewer(evaluator *rules.Evaluator) *AutonomousReviewer {
	return &AutonomousReviewer{
		ruleEvaluator:   evaluator,
		deliberator:     deliberation.NewAgentDeliberator(0.70),
		syntaxValidator: syntax.NewSandboxSyntaxValidator(),
		maxIterations:   5,
	}
}

// SetLLMGateway attaches the AI gateway for deep semantic model inference.
func (a *AutonomousReviewer) SetLLMGateway(gw *llm.Gateway) {
	a.llmGateway = gw
}

// SetDeliberator attaches custom consensus adjudication.
func (a *AutonomousReviewer) SetDeliberator(d *deliberation.AgentDeliberator) {
	a.deliberator = d
}

// SetFeedbackTracker attaches workspace sentiment feedback memory.
func (a *AutonomousReviewer) SetFeedbackTracker(fb *feedback.FeedbackTracker) {
	a.feedbackTracker = fb
}

// PlanReview analyzes patch metadata and formulates an optimal review strategy.
func (a *AutonomousReviewer) PlanReview(patches []*diff.FilePatch) *ReviewPlan {
	plan := &ReviewPlan{
		ReviewID:        uuid.New(),
		HighRiskFiles:   make([]string, 0),
		StandardFiles:   make([]string, 0),
		SuppressedFiles: make([]string, 0),
	}

	for _, p := range patches {
		lang := languages.DetectLanguage(p.NewPath, "")
		isSecuritySensitive := strings.Contains(p.NewPath, "auth") ||
			strings.Contains(p.NewPath, "crypto") ||
			strings.Contains(p.NewPath, "db") ||
			strings.Contains(p.NewPath, "sql") ||
			strings.Contains(p.NewPath, "deploy") ||
			strings.Contains(p.NewPath, "api")

		if isSecuritySensitive || p.Additions > 100 {
			plan.HighRiskFiles = append(plan.HighRiskFiles, p.NewPath)
		} else if lang.IsCodeFile() {
			plan.StandardFiles = append(plan.StandardFiles, p.NewPath)
		}

		plan.EstimatedTokens += (p.Additions + p.Deletions) * 4
	}

	return plan
}

// ExecuteAgenticReview runs multi-turn deliberation: rule evaluation, AI gateway synthesis, and consensus adjudication.
func (a *AutonomousReviewer) ExecuteAgenticReview(ctx context.Context, reviewID, wsID uuid.UUID, patches []*diff.FilePatch) ([]models.CodeFinding, []AgentThought, error) {
	thoughts := make([]AgentThought, 0)

	// Turn 1: Planning
	plan := a.PlanReview(patches)
	thoughts = append(thoughts, AgentThought{
		StepIndex:   1,
		Phase:       "PLAN",
		Observation: fmt.Sprintf("Assessing %d modified file patches (%d high risk). Estimated token load: %d.", len(patches), len(plan.HighRiskFiles), plan.EstimatedTokens),
		Action:      "Decomposing diffs and mapping sensitive file paths.",
		Timestamp:   time.Now().UTC(),
	})

	// Turn 2: Deterministic Rule Analysis
	rawFindings := a.ruleEvaluator.EvaluatePatches(reviewID, wsID, patches)
	thoughts = append(thoughts, AgentThought{
		StepIndex:   2,
		Phase:       "ANALYSIS",
		Observation: fmt.Sprintf("Evaluated static security rules: identified %d potential candidate findings.", len(rawFindings)),
		Action:      "Applying AST context boundaries and suppression filters.",
		Timestamp:   time.Now().UTC(),
	})

	// Turn 3: Multi-Agent LLM Deliberation & Peer Consensus
	var candidates []deliberation.CandidateFinding
	var critiques []deliberation.PeerCritique

	for _, f := range rawFindings {
		candidates = append(candidates, deliberation.ProposeFinding(
			deliberation.PersonaSecurityAuditor,
			f.FilePath,
			f.StartLine,
			f.EndLine,
			f.Title,
			f.Severity,
			0.95,
			f.Description,
			f.SuggestedDiff,
		))
	}

	// Deep Semantic LLM Analysis if gateway is attached
	if a.llmGateway != nil && len(patches) > 0 {
		var diffBuilder strings.Builder
		for _, p := range patches {
			diffBuilder.WriteString(fmt.Sprintf("--- a/%s\n+++ b/%s\n", p.OldPath, p.NewPath))
			for _, h := range p.Hunks {
				diffBuilder.WriteString(h.Header + "\n")
				for _, l := range h.Lines {
					diffBuilder.WriteString(l.Content + "\n")
				}
			}
		}
		diffStr := diffBuilder.String()
		if len(diffStr) > 0 {
			var customRules strings.Builder
			customRules.WriteString(fmt.Sprintf("Identity: %s (%s)\nEnforce rules defined in .drixy/rules/\n", drixy.Name, drixy.Tagline))
			if a.feedbackTracker != nil {
				total, helpfulRate, fpRate := a.feedbackTracker.CalculateWorkspaceSentiment(wsID)
				if total > 0 {
					customRules.WriteString(fmt.Sprintf(
						"Developer Sentiment Memory: Workspace sentiment is %.1f%% helpful with a %.1f%% false-positive rate across %d reviews. Prioritize high signal-to-noise and minimize noise.\n",
						helpfulRate*100, fpRate*100, total,
					))
				}
			}

			aiResp, err := a.llmGateway.AnalyzeDiff(ctx, llm.ReviewRequest{
				WorkspaceID: wsID,
				DiffContent: diffStr,
				CustomRules: customRules.String(),
			})
			if err == nil && aiResp != nil {
				for _, f := range aiResp.Findings {
					sev := models.SeverityMedium
					switch strings.ToUpper(f.Severity) {
					case "CRITICAL":
						sev = models.SeverityCritical
					case "HIGH":
						sev = models.SeverityHigh
					case "LOW":
						sev = models.SeverityLow
					case "INFO":
						sev = models.SeverityInfo
					}
					candidates = append(candidates, deliberation.ProposeFinding(
						deliberation.PersonaSecurityAuditor,
						f.FilePath,
						f.StartLine,
						f.EndLine,
						f.Title,
						sev,
						0.90,
						f.Description,
						f.SuggestedDiff,
					))
				}
			}
		}
	}

	// Turn 4: Syntax Verification & Adjudication
	verifiedFindings := make([]models.CodeFinding, 0)
	if a.deliberator != nil && len(candidates) > 0 {
		accepted, _ := a.deliberator.DeliberateExec(ctx, candidates, critiques)
		for _, f := range accepted {
			// Validate proposed code patch syntax
			if f.SuggestedDiff != "" && a.syntaxValidator != nil {
				syntaxRes := a.syntaxValidator.ValidateSuggestion(f.FilePath, f.SuggestedDiff)
				if !syntaxRes.IsValid {
					// Discard broken suggested diff to prevent proposing bad code to user
					f.SuggestedDiff = ""
				}
			}
			verifiedFindings = append(verifiedFindings, f)
		}
	} else {
		verifiedFindings = rawFindings
	}

	thoughts = append(thoughts, AgentThought{
		StepIndex:   3,
		Phase:       "DELIBERATION",
		Observation: fmt.Sprintf("Deliberation completed: %d findings verified through multi-agent consensus and syntax validation.", len(verifiedFindings)),
		Action:      "Synthesizing final findings with suggested remediations.",
		Timestamp:   time.Now().UTC(),
	})

	return verifiedFindings, thoughts, nil
}
