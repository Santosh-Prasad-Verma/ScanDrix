package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/codehound/codehound/shared/domain"
	"github.com/google/uuid"
)

// AIAnalysisReport aggregates reasoning from all 3 tiers.
type AIAnalysisReport struct {
	TriageDecision   TriageDecision
	LogicFinding     *FindingCandidate
	SecurityFinding  *SecurityFinding
	ArbiterRuling    *ArbiterRuling
	TotalCostUSD     float64
	ModelRuns        []domain.ModelRun
	VerifiedFindings []domain.Finding
	Evidences        []domain.Evidence
}

// Router orchestrates multi-model execution across Tier 1, Tier 2, and Tier 3.
type Router struct {
	client *Client
}

// NewRouter initializes the AI Router with an OpenRouter client.
func NewRouter(client *Client) *Router {
	return &Router{client: client}
}

// AnalyzeCode executes the full 3-Tier AI consensus pipeline over a code file.
func (r *Router) AnalyzeCode(ctx context.Context, tenantID, projectID, scanID uuid.UUID, filePath, language, code string) (*AIAnalysisReport, error) {
	report := &AIAnalysisReport{
		ModelRuns:        make([]domain.ModelRun, 0),
		VerifiedFindings: make([]domain.Finding, 0),
		Evidences:        make([]domain.Evidence, 0),
	}

	sanitizedPrompt := SanitizePromptInput(filePath, language, code)

	// ==========================================
	// TIER 1: Fast Syntactic & Noise Filter
	// ==========================================
	triageRaw, run1, err := r.client.Complete(ctx, tenantID, projectID, scanID, "TRIAGE_AGENT", ModelGeminiFlash, SystemPromptTriage, sanitizedPrompt, true)
	if err != nil {
		return nil, fmt.Errorf("tier-1 triage failed: %w", err)
	}
	report.ModelRuns = append(report.ModelRuns, run1)
	report.TotalCostUSD += run1.TotalCostUSD

	var triageDec TriageDecision
	if err := json.Unmarshal([]byte(triageRaw), &triageDec); err != nil {
		triageDec = TriageDecision{IsClean: false, AnomalyDetected: true, Confidence: 0.5, Summary: "Unparseable triage output"}
	}
	report.TriageDecision = triageDec

	// If Tier 1 is confident the code is clean, pass early (cost optimization)
	if triageDec.IsClean && !triageDec.AnomalyDetected && triageDec.Confidence >= 0.90 {
		return report, nil
	}

	// ==========================================
	// TIER 2: Dual Specialized Reasoning Agents (Parallel)
	// ==========================================
	var wg sync.WaitGroup
	var agentARaw, agentBRaw string
	var runA, runB domain.ModelRun
	var errA, errB error

	wg.Add(2)
	// Agent A: DeepSeek-R1 (Logic Bug Hunter)
	go func() {
		defer wg.Done()
		agentARaw, runA, errA = r.client.Complete(ctx, tenantID, projectID, scanID, "LOGIC_BUG_HUNTER", ModelDeepSeekR1, SystemPromptLogicBugHunter, sanitizedPrompt, true)
	}()

	// Agent B: Claude 3.7 Sonnet (Security Analyst)
	go func() {
		defer wg.Done()
		agentBRaw, runB, errB = r.client.Complete(ctx, tenantID, projectID, scanID, "SECURITY_ANALYST", ModelClaudeSonnet, SystemPromptSecurityAnalyst, sanitizedPrompt, true)
	}()

	wg.Wait()

	if errA == nil {
		report.ModelRuns = append(report.ModelRuns, runA)
		report.TotalCostUSD += runA.TotalCostUSD
		var logicCandidate FindingCandidate
		if err := json.Unmarshal([]byte(agentARaw), &logicCandidate); err == nil && logicCandidate.HasBug {
			report.LogicFinding = &logicCandidate
		}
	}

	if errB == nil {
		report.ModelRuns = append(report.ModelRuns, runB)
		report.TotalCostUSD += runB.TotalCostUSD
		var secFinding SecurityFinding
		if err := json.Unmarshal([]byte(agentBRaw), &secFinding); err == nil && secFinding.HasVulnerability {
			report.SecurityFinding = &secFinding
		}
	}

	// If neither agent found anything, return
	if report.LogicFinding == nil && report.SecurityFinding == nil {
		return report, nil
	}

	// ==========================================
	// TIER 3: Arbiter / Conflict Judge
	// ==========================================
	arbiterInput := fmt.Sprintf("Source Code:\n%s\n\nAgent A (Logic Bug Hunter):\n%s\n\nAgent B (Security Analyst):\n%s",
		sanitizedPrompt, agentARaw, agentBRaw)

	arbiterRaw, run3, err := r.client.Complete(ctx, tenantID, projectID, scanID, "ARBITER_JUDGE", ModelOpenAIo3Mini, SystemPromptArbiterJudge, arbiterInput, true)
	if err == nil {
		report.ModelRuns = append(report.ModelRuns, run3)
		report.TotalCostUSD += run3.TotalCostUSD
		var ruling ArbiterRuling
		if err := json.Unmarshal([]byte(arbiterRaw), &ruling); err == nil {
			report.ArbiterRuling = &ruling

			if ruling.Ruling == "CONFIRMED_VULNERABILITY" {
				findingID := uuid.New()
				var cwe *string
				title := "AI Confirmed Issue in " + filePath
				desc := ruling.Justification

				if report.SecurityFinding != nil {
					cweVal := report.SecurityFinding.CWEID
					cwe = &cweVal
					title = report.SecurityFinding.Title
				} else if report.LogicFinding != nil {
					desc = fmt.Sprintf("%s (Bug Type: %s)", desc, report.LogicFinding.BugType)
				}

				hash := sha256.Sum256([]byte(fmt.Sprintf("AI:%s:%s", filePath, title)))
				canonicalKey := fmt.Sprintf("AI-ARBITER-%s", hex.EncodeToString(hash[:8]))

				sev := domain.FindingSeverityHigh
				if ruling.FinalSeverity == "CRITICAL" {
					sev = domain.FindingSeverityCritical
				}

				finding := domain.Finding{
					ID:              findingID,
					TenantID:        tenantID,
					ProjectID:       projectID,
					CanonicalKey:    canonicalKey,
					Category:        domain.FindingCategorySecurityVuln,
					Severity:        sev,
					State:           domain.FindingStateVerifiedProven,
					Confidence:      ruling.ConsensusScore,
					Title:           title,
					Description:     desc,
					PrimaryFile:     filePath,
					PrimaryLine:     1,
					CWEID:           cwe,
					FirstSeenScanID: scanID,
					LastSeenScanID:  scanID,
				}

				evidence := domain.Evidence{
					ID:             uuid.New(),
					FindingID:      findingID,
					EvidenceType:   "AI_MULTI_MODEL_CONSENSUS",
					SourceAnalyzer: "CODEHOUND_3TIER_ARBITER",
					Strength:       "CRITICAL",
					Summary:        fmt.Sprintf("Tier-3 Arbiter confirmed vulnerability with consensus score %.2f", ruling.ConsensusScore),
					Payload: map[string]any{
						"arbiter_ruling":  ruling.Ruling,
						"consensus_score": ruling.ConsensusScore,
						"justification":   ruling.Justification,
						"models_used":     []string{ModelGeminiFlash, ModelDeepSeekR1, ModelClaudeSonnet, ModelOpenAIo3Mini},
						"total_cost_usd":  report.TotalCostUSD,
					},
				}

				report.VerifiedFindings = append(report.VerifiedFindings, finding)
				report.Evidences = append(report.Evidences, evidence)
			}
		}
	}

	return report, nil
}
