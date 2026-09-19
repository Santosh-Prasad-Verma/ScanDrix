package azuredevops

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAzureDevOps_PolicyConfigurations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/myorg/myproj/_apis/policy/configurations" && r.Method == http.MethodGet:
			resp := map[string]any{
				"value": []map[string]any{
					{
						"id":         10,
						"url":        "https://dev.azure.com/myorg/myproj/_apis/policy/configurations/10",
						"isEnabled":  true,
						"isBlocking": true,
						"type": map[string]any{
							"id":          "fa4e907d-c16b-4a4c-9dc2-49da101cb070",
							"displayName": "Minimum number of reviewers",
						},
						"settings": map[string]any{
							"minimumApproverCount": 2,
							"creatorVoteCounts":    false,
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/myorg/myproj/_apis/policy/configurations/10" && r.Method == http.MethodGet:
			resp := map[string]any{
				"id":         10,
				"url":        "https://dev.azure.com/myorg/myproj/_apis/policy/configurations/10",
				"isEnabled":  true,
				"isBlocking": true,
				"type": map[string]any{
					"id":          "fa4e907d-c16b-4a4c-9dc2-49da101cb070",
					"displayName": "Minimum number of reviewers",
				},
				"settings": map[string]any{
					"minimumApproverCount": 2,
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/myorg/myproj/_apis/policy/configurations" && r.Method == http.MethodPost:
			var body CreatePolicyConfigRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			resp := map[string]any{
				"id":         11,
				"url":        "https://dev.azure.com/myorg/myproj/_apis/policy/configurations/11",
				"isEnabled":  body.IsEnabled,
				"isBlocking": body.IsBlocking,
				"type":       body.Type,
				"settings":   body.Settings,
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/myorg/myproj/_apis/policy/configurations/10" && r.Method == http.MethodPut:
			var body UpdatePolicyConfigRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			resp := map[string]any{
				"id":         10,
				"url":        "https://dev.azure.com/myorg/myproj/_apis/policy/configurations/10",
				"isEnabled":  *body.IsEnabled,
				"isBlocking": *body.IsBlocking,
				"type": map[string]any{
					"id":          "fa4e907d-c16b-4a4c-9dc2-49da101cb070",
					"displayName": "Minimum number of reviewers",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/myorg/myproj/_apis/policy/configurations/10" && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)

		case r.URL.Path == "/myorg/myproj/_apis/policy/evaluations/eval-101" && r.Method == http.MethodPatch:
			resp := map[string]any{
				"evaluationId": "eval-101",
				"status":       "queued",
				"startedDate":  time.Now().Format(time.RFC3339),
			}
			_ = json.NewEncoder(w).Encode(resp)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	svc := NewAzureDevOpsAdvancedService(nil, server.Client(), server.URL)
	ctx := context.Background()

	// 1. Get configurations
	configs, err := svc.GetPolicyConfigurations(ctx, "pat-token", "myorg", "myproj", "myrepo", "refs/heads/main")
	if err != nil {
		t.Fatalf("GetPolicyConfigurations error: %v", err)
	}
	if len(configs) != 1 || configs[0].Settings.MinimumApproverCount != 2 {
		t.Fatalf("unexpected configs: %+v", configs)
	}

	// 2. Get single configuration
	cfg, err := svc.GetPolicyConfiguration(ctx, "pat-token", "myorg", "myproj", 10)
	if err != nil {
		t.Fatalf("GetPolicyConfiguration error: %v", err)
	}
	if cfg.ID != 10 {
		t.Fatalf("expected ID 10, got %d", cfg.ID)
	}

	// 3. Create configuration
	newCfg, err := svc.CreatePolicyConfiguration(ctx, "pat-token", "myorg", "myproj", CreatePolicyConfigRequest{
		Type: PolicyTypeInfo{
			ID:          "build-policy-id",
			DisplayName: "Build Validation",
		},
		IsEnabled:  true,
		IsBlocking: true,
		Settings: PolicySettings{
			BuildDefinitionID: 42,
			DisplayName:       "CI Validation",
		},
	})
	if err != nil {
		t.Fatalf("CreatePolicyConfiguration error: %v", err)
	}
	if newCfg.ID != 11 {
		t.Fatalf("expected created ID 11, got %d", newCfg.ID)
	}

	// 4. Update configuration
	enabled := false
	blocking := false
	updCfg, err := svc.UpdatePolicyConfiguration(ctx, "pat-token", "myorg", "myproj", 10, UpdatePolicyConfigRequest{
		IsEnabled:  &enabled,
		IsBlocking: &blocking,
	})
	if err != nil {
		t.Fatalf("UpdatePolicyConfiguration error: %v", err)
	}
	if updCfg.IsEnabled {
		t.Fatalf("expected IsEnabled to be false")
	}

	// 5. Delete configuration
	if err := svc.DeletePolicyConfiguration(ctx, "pat-token", "myorg", "myproj", 10); err != nil {
		t.Fatalf("DeletePolicyConfiguration error: %v", err)
	}

	// 6. Requeue evaluation
	reVal, err := svc.RequeuePolicyEvaluation(ctx, "pat-token", "myorg", "myproj", "eval-101")
	if err != nil {
		t.Fatalf("RequeuePolicyEvaluation error: %v", err)
	}
	if reVal.Status != "queued" {
		t.Fatalf("expected status queued, got %s", reVal.Status)
	}
}

func TestAzureDevOps_BuildsAndPipelines(t *testing.T) {
	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/myorg/myproj/_apis/build/definitions":
			resp := map[string]any{
				"value": []map[string]any{
					{
						"id":          5,
						"name":        "Backend CI",
						"path":        "\\Pipelines",
						"type":        "build",
						"queueStatus": "enabled",
						"revision":    3,
						"url":         "https://dev.azure.com/myorg/myproj/_apis/build/definitions/5",
						"project":     map[string]any{"name": "myproj"},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/myorg/myproj/_apis/build/builds" && r.Method == http.MethodPost:
			resp := map[string]any{
				"id":          1001,
				"buildNumber": "20260919.1",
				"status":      "notStarted",
				"result":      "",
				"queueTime":   now.Format(time.RFC3339),
				"startTime":   now.Format(time.RFC3339),
				"sourceBranch": "refs/heads/feature/x",
				"definition": map[string]any{
					"id":      5,
					"name":    "Backend CI",
					"project": map[string]any{"name": "myproj"},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/myorg/myproj/_apis/build/builds/1001" && r.Method == http.MethodGet:
			resp := map[string]any{
				"id":          1001,
				"buildNumber": "20260919.1",
				"status":      "completed",
				"result":      "succeeded",
				"queueTime":   now.Format(time.RFC3339),
				"startTime":   now.Format(time.RFC3339),
				"finishTime":  now.Add(2 * time.Minute).Format(time.RFC3339),
				"sourceBranch": "refs/heads/feature/x",
				"definition": map[string]any{
					"id":      5,
					"name":    "Backend CI",
					"project": map[string]any{"name": "myproj"},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/myorg/myproj/_apis/build/builds" && r.Method == http.MethodGet:
			resp := map[string]any{
				"value": []map[string]any{
					{
						"id":          1001,
						"buildNumber": "20260919.1",
						"status":      "completed",
						"result":      "succeeded",
						"sourceBranch": "refs/heads/feature/x",
						"definition": map[string]any{
							"id":      5,
							"name":    "Backend CI",
							"project": map[string]any{"name": "myproj"},
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/myorg/myproj/_apis/build/builds/1001/timeline":
			resp := map[string]any{
				"records": []map[string]any{
					{
						"id":              "step-1",
						"type":            "Task",
						"name":            "Go Test",
						"order":           1,
						"state":           "completed",
						"result":          "succeeded",
						"percentComplete": 100,
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/myorg/myproj/_apis/build/builds/1001/artifacts":
			resp := map[string]any{
				"value": []map[string]any{
					{
						"id":   1,
						"name": "coverage-report",
						"resource": map[string]any{
							"type":        "container",
							"downloadUrl": "https://dev.azure.com/myorg/myproj/_apis/build/builds/1001/artifacts?artifactName=coverage-report",
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/myorg/myproj/_apis/build/builds/1001" && r.Method == http.MethodPatch:
			resp := map[string]any{
				"id":          1001,
				"buildNumber": "20260919.1",
				"status":      "cancelling",
				"result":      "canceled",
				"definition": map[string]any{
					"id":      5,
					"name":    "Backend CI",
					"project": map[string]any{"name": "myproj"},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	svc := NewAzureDevOpsAdvancedService(nil, server.Client(), server.URL)
	ctx := context.Background()

	// 1. Get definitions
	defs, err := svc.GetBuildDefinitions(ctx, "pat-token", "myorg", "myproj")
	if err != nil {
		t.Fatalf("GetBuildDefinitions error: %v", err)
	}
	if len(defs) != 1 || defs[0].Name != "Backend CI" {
		t.Fatalf("unexpected definitions: %+v", defs)
	}

	// 2. Queue build
	queued, err := svc.QueueBuild(ctx, "pat-token", "myorg", "myproj", QueueBuildRequest{
		DefinitionID: 5,
		SourceBranch: "refs/heads/feature/x",
	})
	if err != nil {
		t.Fatalf("QueueBuild error: %v", err)
	}
	if queued.ID != 1001 {
		t.Fatalf("expected build ID 1001, got %d", queued.ID)
	}

	// 3. Get build
	b, err := svc.GetBuild(ctx, "pat-token", "myorg", "myproj", 1001)
	if err != nil {
		t.Fatalf("GetBuild error: %v", err)
	}
	if b.Result != "succeeded" {
		t.Fatalf("expected succeeded result, got %s", b.Result)
	}

	// 4. List builds
	builds, err := svc.ListBuilds(ctx, "pat-token", "myorg", "myproj", ListBuildsOptions{
		Definitions:  []int{5},
		StatusFilter: "completed",
		Top:          10,
	})
	if err != nil {
		t.Fatalf("ListBuilds error: %v", err)
	}
	if len(builds) != 1 {
		t.Fatalf("expected 1 build, got %d", len(builds))
	}

	// 5. Get timeline
	timeline, err := svc.GetBuildTimeline(ctx, "pat-token", "myorg", "myproj", 1001)
	if err != nil {
		t.Fatalf("GetBuildTimeline error: %v", err)
	}
	if len(timeline) != 1 || timeline[0].Name != "Go Test" {
		t.Fatalf("unexpected timeline: %+v", timeline)
	}

	// 6. Get artifacts
	artifacts, err := svc.GetBuildArtifacts(ctx, "pat-token", "myorg", "myproj", 1001)
	if err != nil {
		t.Fatalf("GetBuildArtifacts error: %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].Name != "coverage-report" {
		t.Fatalf("unexpected artifacts: %+v", artifacts)
	}

	// 7. Cancel build
	canceled, err := svc.CancelBuild(ctx, "pat-token", "myorg", "myproj", 1001, "superseded")
	if err != nil {
		t.Fatalf("CancelBuild error: %v", err)
	}
	if canceled.Status != "cancelling" {
		t.Fatalf("expected cancelling status, got %s", canceled.Status)
	}
}

func TestAzureDevOps_StatusesAndIterations(t *testing.T) {
	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/myorg/myproj/_apis/git/repositories/myrepo/pullRequests/10/statuses" && r.Method == http.MethodPost:
			var input PRStatusInput
			_ = json.NewDecoder(r.Body).Decode(&input)
			resp := map[string]any{
				"id":           201,
				"state":        input.State,
				"description":  input.Description,
				"context":      input.Context,
				"targetUrl":    input.TargetURL,
				"creationDate": now.Format(time.RFC3339),
				"updatedDate":  now.Format(time.RFC3339),
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/myorg/myproj/_apis/git/repositories/myrepo/pullRequests/10/statuses" && r.Method == http.MethodGet:
			resp := map[string]any{
				"value": []map[string]any{
					{
						"id":          201,
						"state":       "succeeded",
						"description": "ScanDrix code analysis passed",
						"context": map[string]any{
							"genre": "scandrix",
							"name":  "security-audit",
						},
						"targetUrl":    "https://scandrix.dev/review/10",
						"creationDate": now.Format(time.RFC3339),
						"updatedDate":  now.Format(time.RFC3339),
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/myorg/myproj/_apis/git/repositories/myrepo/pullRequests/10/statuses/201" && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)

		case r.URL.Path == "/myorg/myproj/_apis/git/repositories/myrepo/pullRequests/10/iterations/1/changes":
			resp := map[string]any{
				"changeTrackingId": 1,
				"changeEntries": []map[string]any{
					{
						"changeId":   1,
						"changeType": "edit",
						"item": map[string]any{
							"path":     "/internal/platform/service.go",
							"objectId": "sha123",
						},
					},
				},
				"nextSkip": 0,
				"nextTop":  0,
			}
			_ = json.NewEncoder(w).Encode(resp)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	svc := NewAzureDevOpsAdvancedService(nil, server.Client(), server.URL)
	ctx := context.Background()

	// 1. Create PR status
	status, err := svc.CreatePullRequestStatus(ctx, "pat-token", "myorg", "myproj", "myrepo", 10, PRStatusInput{
		State:       "succeeded",
		Description: "ScanDrix code analysis passed",
		Context: PRStatusContext{
			Genre: "scandrix",
			Name:  "security-audit",
		},
		TargetURL: "https://scandrix.dev/review/10",
	})
	if err != nil {
		t.Fatalf("CreatePullRequestStatus error: %v", err)
	}
	if status.ID != 201 || status.State != "succeeded" {
		t.Fatalf("unexpected status: %+v", status)
	}

	// 2. Get PR statuses
	statuses, err := svc.GetPullRequestStatuses(ctx, "pat-token", "myorg", "myproj", "myrepo", 10)
	if err != nil {
		t.Fatalf("GetPullRequestStatuses error: %v", err)
	}
	if len(statuses) != 1 || statuses[0].Context.Genre != "scandrix" {
		t.Fatalf("unexpected statuses: %+v", statuses)
	}

	// 3. Delete PR status
	if err := svc.DeletePullRequestStatus(ctx, "pat-token", "myorg", "myproj", "myrepo", 10, 201); err != nil {
		t.Fatalf("DeletePullRequestStatus error: %v", err)
	}

	// 4. Get iteration changes
	changes, err := svc.GetIterationChanges(ctx, "pat-token", "myorg", "myproj", "myrepo", 10, 1, 100, 0)
	if err != nil {
		t.Fatalf("GetIterationChanges error: %v", err)
	}
	if len(changes.Changes) != 1 || changes.Changes[0].Item.Path != "/internal/platform/service.go" {
		t.Fatalf("unexpected changes: %+v", changes)
	}

	// 5. Compare iterations
	comp, err := svc.CompareIterations(ctx, "pat-token", "myorg", "myproj", "myrepo", 10, 1, 0)
	if err != nil {
		t.Fatalf("CompareIterations error: %v", err)
	}
	if len(comp.Changes) != 1 {
		t.Fatalf("unexpected comparison changes: %+v", comp)
	}
}
