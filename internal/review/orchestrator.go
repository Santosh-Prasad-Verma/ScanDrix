package review

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/integrations/github"
	"github.com/scandrix/backend/internal/integrations/pm"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/platform"
	"github.com/scandrix/backend/internal/provenance/intoto"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/internal/storage"
	"github.com/scandrix/backend/pkg/crypto"
	"github.com/scandrix/backend/pkg/models"
)

// SCMPublisher publishes inline reviews and updates commit checks on SCM providers.
type SCMPublisher interface {
	SubmitPullRequestReview(ctx context.Context, owner, repo string, pullNumber int, submission github.PullReviewSubmission) error
	UpdateCheckRun(ctx context.Context, owner, repo string, checkRunID int64, req github.UpdateCheckRunRequest) error
}

// Orchestrator coordinates the end-to-end pull request code review pipeline.
type Orchestrator struct {
	repo            *database.Repository
	llmGateway      *llm.Gateway
	artifactClient  *storage.ArtifactClient
	rulesEvaluator  *rules.Evaluator
	scmPublisher    SCMPublisher
	platformAdapter platform.SCMAdapter
	attestor        *intoto.ProvenanceAttestor
	autoTicketMgr   *pm.AutoTicketManager
}

// NewOrchestrator initializes the review orchestration engine.
func NewOrchestrator(
	repo *database.Repository,
	llmGateway *llm.Gateway,
	artifactClient *storage.ArtifactClient,
	rulesEvaluator *rules.Evaluator,
) *Orchestrator {
	return &Orchestrator{
		repo:           repo,
		llmGateway:     llmGateway,
		artifactClient: artifactClient,
		rulesEvaluator: rulesEvaluator,
	}
}

// SetSCMPublisher binds an active SCM client for publishing reviews back to GitHub/GitLab.
func (o *Orchestrator) SetSCMPublisher(pub SCMPublisher) {
	o.scmPublisher = pub
}

// SetSCMAdapter binds a unified platform adapter across GitHub, GitLab, Bitbucket, Azure DevOps, and Forgejo.
func (o *Orchestrator) SetSCMAdapter(adapter platform.SCMAdapter) {
	o.platformAdapter = adapter
}

// SetProvenanceAttestor attaches an in-toto provenance signer.
func (o *Orchestrator) SetProvenanceAttestor(attestor *intoto.ProvenanceAttestor) {
	o.attestor = attestor
}

// SetAutoTicketManager attaches a project management auto-ticketing manager.
func (o *Orchestrator) SetAutoTicketManager(mgr *pm.AutoTicketManager) {
	o.autoTicketMgr = mgr
}

// ExecutionTask represents the input job submitted to the review pipeline.
type ExecutionTask struct {
	ReviewID      uuid.UUID
	WorkspaceID   uuid.UUID
	RepositoryID  uuid.UUID
	RepoNamespace string
	PullNumber    int
	Title         string
	HeadSHA       string
	BaseSHA       string
	Author        string
	RawDiff       string
	CustomRules   string
	CheckRunID    int64
	Provider      models.SCMProvider
	SCMAdapter    platform.SCMAdapter
}

// ProcessReview executes all analysis stages, records findings, and uploads scan artifacts.
func (o *Orchestrator) ProcessReview(ctx context.Context, task ExecutionTask) error {
	slog.Info("Starting pull request review execution",
		"review_id", task.ReviewID,
		"repo", task.RepoNamespace,
		"pr", task.PullNumber,
	)

	// Update state to PROCESSING
	if o.repo != nil {
		_ = o.repo.UpdateReviewState(ctx, task.WorkspaceID, task.ReviewID, models.ReviewStateProcessing, 0)
	}

	// Step 1: Parse git unified diff stream
	patches, err := diff.ParseUnifiedDiff(strings.NewReader(task.RawDiff))
	if err != nil {
		slog.Error("Failed to parse unified diff", "error", err)
		if o.repo != nil {
			_ = o.repo.UpdateReviewState(ctx, task.WorkspaceID, task.ReviewID, models.ReviewStateFailed, 0)
		}
		return fmt.Errorf("diff parse failed: %w", err)
	}

	var allFindings []models.CodeFinding

	// Step 2: Evaluate deterministic custom workspace rules
	if o.rulesEvaluator != nil {
		ruleFindings := o.rulesEvaluator.EvaluatePatches(task.ReviewID, task.WorkspaceID, patches)
		allFindings = append(allFindings, ruleFindings...)
		slog.Info("Rule evaluation finished", "findings", len(ruleFindings))
	}

	// Step 3: Run AI Gateway deep semantic analysis
	if o.llmGateway != nil && len(task.RawDiff) > 0 {
		llmReq := llm.ReviewRequest{
			RepoNamespace: task.RepoNamespace,
			PullTitle:     task.Title,
			DiffContent:   task.RawDiff,
			CustomRules:   task.CustomRules,
		}

		if o.repo != nil {
			if tok, _ := o.repo.GetDecryptedIntegrationToken(ctx, task.WorkspaceID, models.SCMProvider("anthropic")); tok != "" {
				llmReq.BYOKAnthropicKey = tok
			}
			if tok, _ := o.repo.GetDecryptedIntegrationToken(ctx, task.WorkspaceID, models.SCMProvider("openai")); tok != "" {
				llmReq.BYOKOpenAIKey = tok
			}
			if tok, _ := o.repo.GetDecryptedIntegrationToken(ctx, task.WorkspaceID, models.SCMProvider("gemini")); tok != "" {
				llmReq.BYOKGeminiKey = tok
			}
			if tok, _ := o.repo.GetDecryptedIntegrationToken(ctx, task.WorkspaceID, models.SCMProvider("deepseek")); tok != "" {
				llmReq.BYOKDeepSeekKey = tok
			}
			if tok, _ := o.repo.GetDecryptedIntegrationToken(ctx, task.WorkspaceID, models.SCMProvider("openrouter")); tok != "" {
				llmReq.BYOKOpenRouterKey = tok
			}
		}

		aiResp, err := o.llmGateway.AnalyzeDiff(ctx, llmReq)
		if err != nil {
			slog.Warn("AI review synthesis warning", "error", err)
		} else {
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

				allFindings = append(allFindings, models.CodeFinding{
					ID:            uuid.New(),
					ReviewID:      task.ReviewID,
					WorkspaceID:   task.WorkspaceID,
					FilePath:      f.FilePath,
					StartLine:     f.StartLine,
					EndLine:       f.EndLine,
					Severity:      sev,
					Category:      f.Category,
					Title:         f.Title,
					Description:   f.Description,
					Remediation:   f.Remediation,
					SuggestedDiff: f.SuggestedDiff,
					Fingerprint:   crypto.FingerprintSHA256(f.FilePath + ":" + f.Title + ":" + f.Category),
				})
			}
			slog.Info("AI synthesis completed", "ai_findings", len(aiResp.Findings))
		}
	}

	// Step 4: Archive diff and audit report to Appwrite Storage
	if o.artifactClient != nil {
		artifactID := fmt.Sprintf("diff-%s", task.ReviewID.String())
		_, err := o.artifactClient.UploadArtifact(
			ctx,
			"review-artifacts",
			artifactID,
			fmt.Sprintf("pr-%d.diff", task.PullNumber),
			bytes.NewReader([]byte(task.RawDiff)),
		)
		if err != nil {
			slog.Warn("Appwrite storage artifact upload deferred", "error", err)
		}
	}

	// Step 5: Semantic Memory check and Batch insert findings into PostgreSQL with Row-Level Security
	if o.repo != nil {
		var activeFindings []models.CodeFinding
		for _, f := range allFindings {
			isSuppressed := false
			if mem, err := o.repo.GetSecurityMemoryByFingerprint(ctx, task.WorkspaceID, f.Fingerprint); err == nil && mem != nil {
				slog.Info("Suppressing repeat finding matching false-positive fingerprint memory", "fingerprint", f.Fingerprint, "title", f.Title)
				isSuppressed = true
			}
			if !isSuppressed {
				activeFindings = append(activeFindings, f)
			}
		}
		allFindings = activeFindings

		if err := o.repo.BatchInsertFindings(ctx, task.WorkspaceID, allFindings); err != nil {
			slog.Error("Failed saving review findings", "error", err)
			_ = o.repo.UpdateReviewState(ctx, task.WorkspaceID, task.ReviewID, models.ReviewStateFailed, 0)
			return fmt.Errorf("failed persisting findings: %w", err)
		}

		// Step 6: Transition review state to COMPLETED
		if err := o.repo.UpdateReviewState(ctx, task.WorkspaceID, task.ReviewID, models.ReviewStateCompleted, len(allFindings)); err != nil {
			return fmt.Errorf("failed finalizing review: %w", err)
		}

		// Step 6.1: Evaluate findings for automated PM ticket generation (Jira, Linear, Azure Boards)
		if o.autoTicketMgr != nil && len(allFindings) > 0 {
			prURL := ""
			if task.RepoNamespace != "" && task.PullNumber > 0 {
				prURL = fmt.Sprintf("https://github.com/%s/pull/%d", task.RepoNamespace, task.PullNumber)
			}
			go func() {
				bgCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
				defer cancel()
				_, _ = o.autoTicketMgr.ProcessFindingsForAutoTicket(bgCtx, task.WorkspaceID, task.RepositoryID, allFindings, prURL)
			}()
		}
	}

	hasCriticalOrHigh := false
	criticalCount := 0
	highCount := 0
	for _, f := range allFindings {
		if f.Severity == models.SeverityCritical {
			hasCriticalOrHigh = true
			criticalCount++
		} else if f.Severity == models.SeverityHigh {
			hasCriticalOrHigh = true
			highCount++
		}
	}

	// Step 7: Dispatch Inline Review Comments and Check Run update to SCM Provider
	adapter := o.platformAdapter
	if adapter == nil && task.SCMAdapter != nil {
		adapter = task.SCMAdapter
	}

	conclusion := platform.ConclusionSuccess
	commitStatusState := platform.StatusSuccess
	summaryText := "ScanDrix automated security review passed with zero critical findings."

	if hasCriticalOrHigh {
		conclusion = platform.ConclusionFailure
		commitStatusState = platform.StatusFailure
		summaryText = fmt.Sprintf("ScanDrix detected %d actionable findings requiring resolution.", len(allFindings))
	}

	if adapter != nil && task.RepoNamespace != "" && task.PullNumber > 0 {
		var inlineSpecs []platform.InlineCommentSpec
		for _, f := range allFindings {
			inlineSpecs = append(inlineSpecs, platform.InlineCommentSpec{
				FilePath:  f.FilePath,
				Line:      f.EndLine,
				StartLine: f.StartLine,
				Body:      formatCommentBody(f),
			})
		}
		if len(inlineSpecs) > 0 {
			if err := adapter.PostInlineComments(ctx, task.RepoNamespace, task.PullNumber, inlineSpecs); err != nil {
				slog.Warn("Failed posting platform inline comments", "error", err)
			}
		}
		if err := adapter.PostReviewSummary(ctx, task.RepoNamespace, task.PullNumber, sanitizeMarkdownComment(summaryText), conclusion); err != nil {
			slog.Warn("Failed posting platform review summary", "error", err)
		}
		if task.HeadSHA != "" {
			_ = adapter.SetCommitStatus(ctx, task.RepoNamespace, task.HeadSHA, "scandrix/review", commitStatusState, "https://scandrix.dev", summaryText)
		}
	} else if o.scmPublisher != nil && task.RepoNamespace != "" && task.PullNumber > 0 {
		parts := strings.Split(task.RepoNamespace, "/")
		if len(parts) == 2 {
			owner, repoName := parts[0], parts[1]

			var comments []github.ReviewCommentPayload
			var annotations []github.CheckRunAnnotation

			for _, f := range allFindings {
				comments = append(comments, github.ReviewCommentPayload{
					Path: f.FilePath,
					Line: f.EndLine,
					Side: "RIGHT",
					Body: formatCommentBody(f),
				})

				level := "warning"
				if f.Severity == models.SeverityCritical || f.Severity == models.SeverityHigh {
					level = "failure"
				} else if f.Severity == models.SeverityInfo || f.Severity == models.SeverityLow {
					level = "notice"
				}

				if len(annotations) < 50 {
					annotations = append(annotations, github.CheckRunAnnotation{
						Path:            f.FilePath,
						StartLine:       f.StartLine,
						EndLine:         f.EndLine,
						AnnotationLevel: level,
						Title:           fmt.Sprintf("[%s] %s", f.Severity, f.Title),
						Message:         f.Description,
					})
				}
			}

			event := "APPROVE"
			if hasCriticalOrHigh {
				event = "REQUEST_CHANGES"
			}

			submission := github.PullReviewSubmission{
				CommitID: task.HeadSHA,
				Body:     fmt.Sprintf("## 🚀 ScanDrix Automated Code Assurance Report\n\n%s\n\n- **Total Findings:** %d\n- **Review ID:** `%s`", summaryText, len(allFindings), task.ReviewID.String()),
				Event:    event,
				Comments: comments,
			}

			if err := o.scmPublisher.SubmitPullRequestReview(ctx, owner, repoName, task.PullNumber, submission); err != nil {
				slog.Warn("Failed publishing review comments to SCM", "error", err)
			} else {
				slog.Info("Successfully published review comments to SCM", "owner", owner, "repo", repoName, "pr", task.PullNumber)
			}

			if task.CheckRunID > 0 {
				now := time.Now().UTC()
				updateReq := github.UpdateCheckRunRequest{
					Status:      "completed",
					Conclusion:  strings.ToLower(string(conclusion)),
					CompletedAt: &now,
					Output: &github.CheckRunOutput{
						Title:       fmt.Sprintf("ScanDrix Review: %s", strings.ToUpper(string(conclusion))),
						Summary:     summaryText,
						Annotations: annotations,
					},
				}
				if err := o.scmPublisher.UpdateCheckRun(ctx, owner, repoName, task.CheckRunID, updateReq); err != nil {
					slog.Warn("Failed updating SCM check run", "error", err)
				}
			}
		}
	}

	// Step 8: Generate and sign cryptographic in-toto DSSE and SLSA v1.0 provenance attestations
	attestor := o.attestor
	var keyID string
	if attestor == nil {
		masterSecret := os.Getenv("SCANDRIX_ENCRYPTION_KEY")
		if masterSecret == "" {
			masterSecret = os.Getenv("KMS_MASTER_KEY")
		}
		privKey, pubKey, kid := intoto.DeriveTenantKeypair(task.WorkspaceID, masterSecret)
		keyID = kid
		attestor = intoto.NewProvenanceAttestor(keyID, privKey, pubKey)
	}

	decision := intoto.DecisionApproved
	if criticalCount > 0 {
		decision = intoto.DecisionBlockedCritical
	} else if highCount > 0 {
		decision = intoto.DecisionChangesRequested
	}
	pred := intoto.ReviewAttestationPredicate{
		WorkspaceID:           task.WorkspaceID,
		RepoNamespace:         task.RepoNamespace,
		CommitSHA:             task.HeadSHA,
		PullRequestNumber:     task.PullNumber,
		Decision:              decision,
		TotalFindings:         len(allFindings),
		CriticalCount:         criticalCount,
		HighCount:             highCount,
		ConsensusScore:        1.0,
		AttestedAt:            time.Now().UTC(),
		ReviewerAgentIdentity: "scandrix-orchestrator-v1",
	}

	// 8.1 In-Toto Code Review Attestation
	if env, err := attestor.AttestAndSign(pred); err != nil {
		slog.Warn("Failed generating in-toto provenance attestation", "review_id", task.ReviewID, "error", err)
	} else {
		slog.Info("Successfully generated in-toto DSSE provenance attestation", "review_id", task.ReviewID, "payload_type", env.PayloadType)
		if o.repo != nil {
			_ = o.repo.InsertReviewAttestation(ctx, intoto.AttestationRecord{
				WorkspaceID:   task.WorkspaceID,
				ReviewID:      task.ReviewID,
				PredicateType: intoto.PredicateTypeReview,
				Decision:      decision,
				KeyID:         keyID,
				Envelope:      *env,
				CreatedAt:     time.Now().UTC(),
			})
		}
		if o.artifactClient != nil {
			if envJSON, err := json.Marshal(env); err == nil {
				_, _ = o.artifactClient.UploadArtifact(ctx, "attestations", task.ReviewID.String(), fmt.Sprintf("attestation-intoto-%s.json", task.ReviewID.String()), bytes.NewReader(envJSON))
			}
		}
	}

	// 8.2 SLSA Provenance v1.0 Attestation
	if slsaEnv, err := attestor.AttestAndSignSLSA(pred, task.ReviewID, time.Now().UTC().Add(-1*time.Minute)); err != nil {
		slog.Warn("Failed generating SLSA provenance attestation", "review_id", task.ReviewID, "error", err)
	} else {
		slog.Info("Successfully generated SLSA v1.0 provenance attestation", "review_id", task.ReviewID, "payload_type", slsaEnv.PayloadType)
		if o.repo != nil {
			_ = o.repo.InsertReviewAttestation(ctx, intoto.AttestationRecord{
				WorkspaceID:   task.WorkspaceID,
				ReviewID:      task.ReviewID,
				PredicateType: intoto.PredicateTypeSLSA,
				Decision:      decision,
				KeyID:         keyID,
				Envelope:      *slsaEnv,
				CreatedAt:     time.Now().UTC(),
			})
		}
		if o.artifactClient != nil {
			if envJSON, err := json.Marshal(slsaEnv); err == nil {
				_, _ = o.artifactClient.UploadArtifact(ctx, "attestations", task.ReviewID.String(), fmt.Sprintf("attestation-slsa-%s.json", task.ReviewID.String()), bytes.NewReader(envJSON))
			}
		}
	}

	slog.Info("Review execution completed successfully",
		"review_id", task.ReviewID,
		"total_findings", len(allFindings),
	)

	return nil
}

func sanitizeMarkdownComment(input string) string {
	// Neutralize dangerous HTML tags & event handlers in PR comments
	sanitized := strings.ReplaceAll(input, "<script", "&lt;script")
	sanitized = strings.ReplaceAll(sanitized, "</script>", "&lt;/script&gt;")
	sanitized = strings.ReplaceAll(sanitized, "<iframe", "&lt;iframe")
	sanitized = strings.ReplaceAll(sanitized, "</iframe", "&lt;/iframe")
	sanitized = strings.ReplaceAll(sanitized, "<object", "&lt;object")
	sanitized = strings.ReplaceAll(sanitized, "<embed", "&lt;embed")
	sanitized = strings.ReplaceAll(sanitized, "javascript:", "blocked-javascript:")
	sanitized = strings.ReplaceAll(sanitized, "onerror=", "data-blocked-onerror=")
	sanitized = strings.ReplaceAll(sanitized, "onload=", "data-blocked-onload=")
	return sanitized
}

func formatCommentBody(f models.CodeFinding) string {
	body := fmt.Sprintf("### 🛡️ [%s] %s\n\n%s\n\n**Remediation:**\n%s", f.Severity, f.Title, f.Description, f.Remediation)
	if f.SuggestedDiff != "" {
		body += fmt.Sprintf("\n\n```suggestion\n%s\n```", strings.TrimSpace(f.SuggestedDiff))
	}
	return sanitizeMarkdownComment(body)
}
