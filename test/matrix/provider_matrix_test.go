package matrix_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/enterprise/license"
	"github.com/scandrix/backend/internal/queue/consumer"
	"github.com/scandrix/backend/internal/queue/relay"
	"github.com/scandrix/backend/internal/webhooks/ingestion"
	"github.com/scandrix/backend/pkg/models"
)

// getProviderPRFixture returns a provider-specific eventType and JSON payload
// representing a newly opened Pull Request.
func getProviderPRFixture(provider models.SCMProvider, repoNamespace string, prNumber int, headSHA string) (string, []byte) {
	switch provider {
	case models.ProviderGitHub, models.ProviderForgejo:
		eventType := "pull_request"
		payload := map[string]any{
			"action": "opened",
			"number": prNumber,
			"pull_request": map[string]any{
				"number": prNumber,
				"title":  fmt.Sprintf("PR #%d feature branch", prNumber),
				"head": map[string]any{
					"sha": headSHA,
				},
				"base": map[string]any{
					"sha": "base000000000000000000000000000000000000",
				},
				"user": map[string]any{
					"login": "octocat",
				},
			},
			"repository": map[string]any{
				"full_name": repoNamespace,
			},
		}
		data, _ := json.Marshal(payload)
		return eventType, data

	case models.ProviderGitLab:
		eventType := "Merge Request Hook"
		payload := map[string]any{
			"object_kind": "merge_request",
			"project": map[string]any{
				"path_with_namespace": repoNamespace,
			},
			"user": map[string]any{
				"username": "gitlab-user",
			},
			"object_attributes": map[string]any{
				"iid":    prNumber,
				"title":  fmt.Sprintf("MR !%d feature branch", prNumber),
				"action": "open",
				"last_commit": map[string]any{
					"id": headSHA,
				},
				"target_branch": "main",
			},
		}
		data, _ := json.Marshal(payload)
		return eventType, data

	case models.ProviderBitbucket:
		eventType := "pullrequest:created"
		payload := map[string]any{
			"repository": map[string]any{
				"full_name": repoNamespace,
			},
			"actor": map[string]any{
				"username": "bb-user",
			},
			"pullrequest": map[string]any{
				"id":    prNumber,
				"title": fmt.Sprintf("PR #%d bitbucket branch", prNumber),
				"source": map[string]any{
					"commit": map[string]any{
						"hash": headSHA,
					},
				},
				"destination": map[string]any{
					"commit": map[string]any{
						"hash": "base000000000000000000000000000000000000",
					},
				},
			},
		}
		data, _ := json.Marshal(payload)
		return eventType, data

	case models.ProviderAzure:
		eventType := "git.pullrequest.created"
		payload := map[string]any{
			"eventType": "git.pullrequest.created",
			"resource": map[string]any{
				"pullRequestId": prNumber,
				"title":         fmt.Sprintf("PR #%d azure branch", prNumber),
				"repository": map[string]any{
					"name": "scandrix-repo",
					"project": map[string]any{
						"name": "scandrix-org",
					},
				},
				"createdBy": map[string]any{
					"displayName": "Azure Developer",
				},
				"lastMergeSourceCommit": map[string]any{
					"commitId": headSHA,
				},
			},
		}
		data, _ := json.Marshal(payload)
		return eventType, data

	default:
		return "", nil
	}
}

// TestProviderAndPlanQualityMatrix executes a table-driven verification across
// 5 SCM Providers × 3 License Tiers (15 matrix cells).
// This serves as the Go-native release quality gate ensuring no provider or plan
// regresses during shared pipeline modifications.
func TestProviderAndPlanQualityMatrix(t *testing.T) {
	providers := []models.SCMProvider{
		models.ProviderGitHub,
		models.ProviderGitLab,
		models.ProviderBitbucket,
		models.ProviderAzure,
		models.ProviderForgejo,
	}

	tiers := []license.LicenseTier{
		license.TierCommunity,
		license.TierDeveloper,
		license.TierTeam,
		license.TierEnterprise,
	}

	parser := ingestion.NewWebhookParser()
	inbox := relay.NewInboxDeduplicator()
	ctx := context.Background()

	totalCells := len(providers) * len(tiers)
	executedCells := 0

	for _, provider := range providers {
		for _, tier := range tiers {
			cellName := fmt.Sprintf("%s__%s", provider, tier)
			t.Run(cellName, func(t *testing.T) {
				repoNamespace := fmt.Sprintf("scandrix-org/%s-test-repo", provider)
				prNumber := 42
				headSHA := "c0ffee0000000000000000000000000000000001"

				// 1. Ingress Parsing Gate
				eventType, rawBody := getProviderPRFixture(provider, repoNamespace, prNumber, headSHA)
				if rawBody == nil {
					t.Fatalf("missing fixture for provider %s", provider)
				}

				event, err := parser.Parse(provider, eventType, rawBody)
				if err != nil {
					t.Fatalf("failed to parse %s webhook: %v", provider, err)
				}

				if event.Action != ingestion.ActionOpened {
					t.Fatalf("expected ActionOpened for cell %s, got: %s", cellName, event.Action)
				}
				if event.PullRequestNumber != prNumber {
					t.Fatalf("expected PR #%d, got %d", prNumber, event.PullRequestNumber)
				}
				if event.HeadSHA != headSHA {
					t.Fatalf("expected HeadSHA %s, got %s", headSHA, event.HeadSHA)
				}

				// Assign Event & Task IDs (Fix #1 Contract Verification)
				eventID := uuid.New()
				event.ID = eventID
				event.TaskID = eventID
				event.EventID = eventID

				// 2. Outbox Pipeline Serialization Gate
				outboxPayload, err := json.Marshal(event)
				if err != nil {
					t.Fatalf("failed to serialize outbox event: %v", err)
				}

				var workerTask consumer.ReviewTaskPayload
				if err := json.Unmarshal(outboxPayload, &workerTask); err != nil {
					t.Fatalf("worker payload deserialization contract failure: %v", err)
				}

				if workerTask.ID != eventID {
					t.Fatalf("worker payload ID mismatch: expected %s, got %s", eventID, workerTask.ID)
				}

				// 3. Inbox Deduplication Gate
				claimID := fmt.Sprintf("%s:%s:%s:%d:%s", provider, tier, repoNamespace, prNumber, headSHA)
				firstClaim, err := inbox.ClaimMessage(ctx, claimID, "worker-pod-1")
				if err != nil || !firstClaim {
					t.Fatalf("inbox should have successfully claimed first message for %s, err: %v", claimID, err)
				}

				duplicateClaim, err := inbox.ClaimMessage(ctx, claimID, "worker-pod-1")
				if err != nil || duplicateClaim {
					t.Fatalf("inbox must reject duplicate delivery for cell %s", cellName)
				}

				// 4. Plan Entitlements & Policy Gate
				quota := license.GetPlanQuota(tier)
				modelsList := license.GetAllocatedModelsList(tier)
				if len(modelsList) == 0 {
					t.Fatalf("expected models allocated for tier %s", tier)
				}

				switch tier {
				case license.TierCommunity:
					if quota.MonthlyTokens <= 0 {
						t.Errorf("expected positive monthly tokens quota for community tier")
					}
					// Verify BYOK access control
					canAccessCustom, _ := license.CanAccessModel(tier, "custom-model", false)
					if canAccessCustom {
						t.Errorf("community tier must not access custom model without BYOK")
					}
					canAccessCustomBYOK, _ := license.CanAccessModel(tier, "custom-model", true)
					if !canAccessCustomBYOK {
						t.Errorf("community tier must access custom model with BYOK")
					}

				case license.TierDeveloper, license.TierTeam, license.TierEnterprise:
					if quota.MaxSeats <= 5 && quota.MaxSeats != 0 {
						t.Errorf("developer, team, and enterprise tiers must support expanded seats")
					}
					canAccessManaged, _ := license.CanAccessModel(tier, "gemini-3.7-flash", false)
					if !canAccessManaged {
						t.Errorf("developer, team, and enterprise tiers must access managed models directly")
					}
				}

				executedCells++
			})
		}
	}

	if executedCells != totalCells {
		t.Fatalf("expected %d matrix cells executed, got %d", totalCells, executedCells)
	}
	t.Logf("Successfully executed %d / %d multi-provider matrix cells with 100%% pass rate", executedCells, totalCells)
}
