package azuredevops

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAzureDevOpsWorkItemsService_GetAndUpdate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/myorg/myproj/_apis/wit/workitems/42" && r.Method == http.MethodGet:
			resp := map[string]any{
				"id":  42,
				"url": "https://dev.azure.com/myorg/myproj/_apis/wit/workitems/42",
				"fields": map[string]any{
					"System.Title":        "Fix authentication token expiry",
					"System.WorkItemType": "Bug",
					"System.State":        "Active",
					"System.AssignedTo": map[string]any{
						"displayName": "Alice Dev",
					},
					"System.CreatedBy": map[string]any{
						"displayName": "QA Lead",
					},
					"System.CreatedDate": time.Now().Format(time.RFC3339),
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/myorg/myproj/_apis/wit/workitems/42" && r.Method == http.MethodPatch:
			resp := map[string]any{
				"id":  42,
				"url": "https://dev.azure.com/myorg/myproj/_apis/wit/workitems/42",
				"fields": map[string]any{
					"System.Title": "Fix authentication token expiry",
					"System.State": "Resolved",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := NewAzureDevOpsWorkItemsService(server.Client(), server.URL)
	ctx := context.Background()

	// 1. Get work item
	item, err := svc.GetWorkItem(ctx, "mock-token", "myorg", "myproj", 42)
	if err != nil {
		t.Fatalf("GetWorkItem failed: %v", err)
	}
	if item.ID != 42 || item.Title != "Fix authentication token expiry" || item.WorkItemType != "Bug" || item.State != "Active" {
		t.Errorf("unexpected work item: %+v", item)
	}
	if item.AssignedTo != "Alice Dev" {
		t.Errorf("expected Alice Dev, got %s", item.AssignedTo)
	}

	// 2. Update work item
	patches := []WorkItemUpdatePatch{
		{Op: "replace", Path: "/fields/System.State", Value: "Resolved"},
	}
	updated, err := svc.UpdateWorkItem(ctx, "mock-token", "myorg", "myproj", 42, patches)
	if err != nil {
		t.Fatalf("UpdateWorkItem failed: %v", err)
	}
	if updated.State != "Resolved" {
		t.Errorf("expected Resolved state, got %s", updated.State)
	}

	// 3. Extract work item IDs
	text := "Resolves #42 and addresses #108 in commit abc. Also check (#999)."
	ids := ExtractWorkItemIDs(text)
	if len(ids) != 3 || ids[0] != 42 || ids[1] != 108 || ids[2] != 999 {
		t.Errorf("unexpected extracted IDs: %v", ids)
	}
}
