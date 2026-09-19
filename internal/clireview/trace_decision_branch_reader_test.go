package clireview_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/scandrix/backend/internal/clireview"
)

type mockRepositoryContentReader struct {
	calls        []mockRepoCall
	contentByRef map[string]string
	err          error
}

type mockRepoCall struct {
	orgID    string
	teamID   string
	repoID   string
	filename string
	branch   string
}

func (m *mockRepositoryContentReader) GetRepositoryContentFile(ctx context.Context, orgID, teamID, repoID, filename, branch string) (string, error) {
	m.calls = append(m.calls, mockRepoCall{
		orgID:    orgID,
		teamID:   teamID,
		repoID:   repoID,
		filename: filename,
		branch:   branch,
	})
	if m.err != nil {
		return "", m.err
	}
	key := fmt.Sprintf("%s:%s:%s", orgID, teamID, repoID)
	if content, ok := m.contentByRef[key]; ok {
		return content, nil
	}
	if content, ok := m.contentByRef[repoID]; ok {
		return content, nil
	}
	return "", nil
}

func makeDecision(text string) clireview.TraceContextDecision {
	return clireview.TraceContextDecision{
		CliSessionClassifiedDecision: clireview.CliSessionClassifiedDecision{
			Type:       clireview.DecisionConvention,
			Decision:   text,
			Confidence: 0.9,
			Evidence:   []string{"src/index.ts"},
		},
		Scope: []string{"src/index.ts"},
	}
}

func TestTraceDecisionBranchReaderService(t *testing.T) {
	t.Run("reads the exact branch shard from the requested repository and team", func(t *testing.T) {
		recordA := clireview.TraceDecisionBranchRecord{
			Version:   1,
			Branch:    "main",
			Decisions: []clireview.TraceContextDecision{makeDecision("repo-a decision")},
		}
		recordB := clireview.TraceDecisionBranchRecord{
			Version:   1,
			Branch:    "main",
			Decisions: []clireview.TraceContextDecision{makeDecision("repo-b decision")},
		}
		bytesA, _ := json.Marshal(recordA)
		bytesB, _ := json.Marshal(recordB)

		reader := &mockRepositoryContentReader{
			contentByRef: map[string]string{
				"org-1:team-a:repo-a": base64.StdEncoding.EncodeToString(bytesA),
				"org-1:team-b:repo-b": base64.StdEncoding.EncodeToString(bytesB),
			},
		}

		service := clireview.NewTraceDecisionBranchReaderService(reader)

		repoA, err := service.Read(context.Background(), clireview.ReadTraceDecisionBranchInput{
			OrganizationID: "org-1",
			TeamID:         "team-a",
			RepositoryID:   "repo-a",
			RepositoryName: "repo-a",
			Branch:         "main",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		repoB, err := service.Read(context.Background(), clireview.ReadTraceDecisionBranchInput{
			OrganizationID: "org-1",
			TeamID:         "team-b",
			RepositoryID:   "repo-b",
			RepositoryName: "repo-b",
			Branch:         "main",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if repoA == nil || len(repoA.Decisions) == 0 || repoA.Decisions[0].Decision != "repo-a decision" {
			t.Errorf("expected repo-a decision, got %+v", repoA)
		}
		if repoB == nil || len(repoB.Decisions) == 0 || repoB.Decisions[0].Decision != "repo-b decision" {
			t.Errorf("expected repo-b decision, got %+v", repoB)
		}

		if len(reader.calls) != 2 {
			t.Fatalf("expected 2 calls, got %d", len(reader.calls))
		}
		if reader.calls[0].filename != clireview.TraceRecordPath("main") {
			t.Errorf("expected filename %s, got %s", clireview.TraceRecordPath("main"), reader.calls[0].filename)
		}
		if reader.calls[0].branch != clireview.TraceBranch {
			t.Errorf("expected branch %s, got %s", clireview.TraceBranch, reader.calls[0].branch)
		}
	})

	t.Run("returns null when the provider has no Trace ref", func(t *testing.T) {
		reader := &mockRepositoryContentReader{
			contentByRef: map[string]string{},
		}
		service := clireview.NewTraceDecisionBranchReaderService(reader)

		res, err := service.Read(context.Background(), clireview.ReadTraceDecisionBranchInput{
			OrganizationID: "org-1",
			TeamID:         "team-1",
			RepositoryID:   "repo-1",
			RepositoryName: "repo-1",
			Branch:         "main",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != nil {
			t.Errorf("expected nil result, got %+v", res)
		}
	})

	t.Run("normalizes a provider refs/heads branch to the CLI destination key", func(t *testing.T) {
		record := clireview.TraceDecisionBranchRecord{
			Version:   1,
			Branch:    "feature-x",
			Decisions: []clireview.TraceContextDecision{makeDecision("feature decision")},
		}
		bytes, _ := json.Marshal(record)

		reader := &mockRepositoryContentReader{
			contentByRef: map[string]string{
				"org-1:team-1:repo-1": string(bytes),
			},
		}
		service := clireview.NewTraceDecisionBranchReaderService(reader)

		res, err := service.Read(context.Background(), clireview.ReadTraceDecisionBranchInput{
			OrganizationID: "org-1",
			TeamID:         "team-1",
			RepositoryID:   "repo-1",
			RepositoryName: "repo-1",
			Branch:         "refs/heads/feature-x",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res == nil || res.Branch != "feature-x" {
			t.Errorf("expected branch feature-x, got %+v", res)
		}
		if len(reader.calls) != 1 || reader.calls[0].filename != clireview.TraceRecordPath("feature-x") {
			t.Errorf("expected call with filename for feature-x, got %+v", reader.calls)
		}
	})

	t.Run("rejects content for another branch returned by a provider fallback", func(t *testing.T) {
		record := clireview.TraceDecisionBranchRecord{
			Version:   1,
			Branch:    "not-main",
			Decisions: []clireview.TraceContextDecision{makeDecision("wrong branch")},
		}
		bytes, _ := json.Marshal(record)

		reader := &mockRepositoryContentReader{
			contentByRef: map[string]string{
				"org-1:team-1:repo-1": string(bytes),
			},
		}
		service := clireview.NewTraceDecisionBranchReaderService(reader)

		res, err := service.Read(context.Background(), clireview.ReadTraceDecisionBranchInput{
			OrganizationID: "org-1",
			TeamID:         "team-1",
			RepositoryID:   "repo-1",
			RepositoryName: "repo-1",
			Branch:         "main",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != nil {
			t.Errorf("expected nil for mismatched branch, got %+v", res)
		}
	})

	t.Run("does not issue an unscoped request", func(t *testing.T) {
		reader := &mockRepositoryContentReader{}
		service := clireview.NewTraceDecisionBranchReaderService(reader)

		res, err := service.Read(context.Background(), clireview.ReadTraceDecisionBranchInput{
			OrganizationID: "org-1",
			TeamID:         "", // Missing team scope
			RepositoryID:   "repo-1",
			RepositoryName: "repo-1",
			Branch:         "main",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != nil {
			t.Errorf("expected nil for unscoped request, got %+v", res)
		}
		if len(reader.calls) != 0 {
			t.Errorf("expected no repository content calls for unscoped request, got %d", len(reader.calls))
		}
	})
}
