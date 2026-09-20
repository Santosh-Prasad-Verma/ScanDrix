package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// GitLabAdvancedService provides extended capabilities for GitLab REST API v4,
// including MR discussions, diff versions, draft notes, approvals, protected branches, and reactions.
type GitLabAdvancedService struct {
	adapter    *Adapter
	httpClient *http.Client
	baseURL    string
}

// NewGitLabAdvancedService creates a new instance of GitLabAdvancedService.
func NewGitLabAdvancedService(adapter *Adapter, httpClient *http.Client, baseURL ...string) *GitLabAdvancedService {
	url := "https://gitlab.com/api/v4"
	if len(baseURL) > 0 && baseURL[0] != "" {
		url = strings.TrimRight(baseURL[0], "/")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &GitLabAdvancedService{
		adapter:    adapter,
		httpClient: httpClient,
		baseURL:    url,
	}
}

// DiscussionThread models a GitLab merge request discussion thread.
type DiscussionThread struct {
	ID             string          `json:"id"`
	IndividualNote bool            `json:"individual_note"`
	Notes          []DiscussionNote `json:"notes"`
}

// DiscussionNote models an individual note inside a discussion thread.
type DiscussionNote struct {
	ID         int       `json:"id"`
	Type       string    `json:"type"` // "DiffNote", "DiscussionNote", ""
	Body       string    `json:"body"`
	Author     string    `json:"author"`
	CreatedAt  time.Time `json:"created_at"`
	Resolvable bool      `json:"resolvable"`
	Resolved   bool      `json:"resolved"`
	Position   *NotePosition `json:"position,omitempty"`
}

// NotePosition models line and commit coordinates for diff notes.
type NotePosition struct {
	BaseSHA      string `json:"base_sha"`
	StartSHA     string `json:"start_sha"`
	HeadSHA      string `json:"head_sha"`
	OldPath      string `json:"old_path,omitempty"`
	NewPath      string `json:"new_path,omitempty"`
	OldLine      int    `json:"old_line,omitempty"`
	NewLine      int    `json:"new_line,omitempty"`
	PositionType string `json:"position_type"` // "text", "image", "file"
}

// MergeRequestDiffVersion models a snapshot version of a merge request's diff.
type MergeRequestDiffVersion struct {
	ID             int       `json:"id"`
	HeadCommitSHA  string    `json:"head_commit_sha"`
	BaseCommitSHA  string    `json:"base_commit_sha"`
	StartCommitSHA string    `json:"start_commit_sha"`
	CreatedAt      time.Time `json:"created_at"`
	State          string    `json:"state"`
}

// MergeRequestApprovals models MR approval configuration and status.
type MergeRequestApprovals struct {
	ID                int      `json:"id"`
	IID               int      `json:"iid"`
	ApprovalsRequired int      `json:"approvals_required"`
	ApprovalsLeft     int      `json:"approvals_left"`
	ApprovedBy        []string `json:"approved_by"`
}

// ProtectedBranchRule models push and merge restrictions for a branch.
type ProtectedBranchRule struct {
	Name             string `json:"name"`
	PushAccessLevel  int    `json:"push_access_level"`
	MergeAccessLevel int    `json:"merge_access_level"`
	AllowForcePush   bool   `json:"allow_force_push"`
	CodeOwnerApprovalRequired bool `json:"code_owner_approval_required"`
}

// GetMergeRequestDiscussions retrieves all discussion threads on a merge request.
func (s *GitLabAdvancedService) GetMergeRequestDiscussions(
	ctx context.Context,
	token string,
	projectID any,
	mrIID int,
) ([]DiscussionThread, error) {
	endpoint := fmt.Sprintf("%s/projects/%v/merge_requests/%d/discussions?per_page=100",
		s.baseURL, projectID, mrIID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab api error: status %d: %s", resp.StatusCode, string(b))
	}

	var rawDiscussions []struct {
		ID             string `json:"id"`
		IndividualNote bool   `json:"individual_note"`
		Notes          []struct {
			ID         int       `json:"id"`
			Type       string    `json:"type"`
			Body       string    `json:"body"`
			CreatedAt  time.Time `json:"created_at"`
			Resolvable bool      `json:"resolvable"`
			Resolved   bool      `json:"resolved"`
			Author     struct {
				Username string `json:"username"`
			} `json:"author"`
			Position *struct {
				BaseSHA      string `json:"base_sha"`
				StartSHA     string `json:"start_sha"`
				HeadSHA      string `json:"head_sha"`
				OldPath      string `json:"old_path"`
				NewPath      string `json:"new_path"`
				OldLine      int    `json:"old_line"`
				NewLine      int    `json:"new_line"`
				PositionType string `json:"position_type"`
			} `json:"position"`
		} `json:"notes"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rawDiscussions); err != nil {
		return nil, fmt.Errorf("decode discussions: %w", err)
	}

	threads := make([]DiscussionThread, len(rawDiscussions))
	for i, d := range rawDiscussions {
		th := DiscussionThread{
			ID:             d.ID,
			IndividualNote: d.IndividualNote,
			Notes:          make([]DiscussionNote, len(d.Notes)),
		}
		for j, n := range d.Notes {
			note := DiscussionNote{
				ID:         n.ID,
				Type:       n.Type,
				Body:       n.Body,
				Author:     n.Author.Username,
				CreatedAt:  n.CreatedAt,
				Resolvable: n.Resolvable,
				Resolved:   n.Resolved,
			}
			if n.Position != nil {
				note.Position = &NotePosition{
					BaseSHA:      n.Position.BaseSHA,
					StartSHA:     n.Position.StartSHA,
					HeadSHA:      n.Position.HeadSHA,
					OldPath:      n.Position.OldPath,
					NewPath:      n.Position.NewPath,
					OldLine:      n.Position.OldLine,
					NewLine:      n.Position.NewLine,
					PositionType: n.Position.PositionType,
				}
			}
			th.Notes[j] = note
		}
		threads[i] = th
	}
	return threads, nil
}

// CreateDiffDiscussion creates a new inline discussion thread on a specific line of an MR diff.
func (s *GitLabAdvancedService) CreateDiffDiscussion(
	ctx context.Context,
	token string,
	projectID any,
	mrIID int,
	body string,
	pos NotePosition,
) (*DiscussionThread, error) {
	endpoint := fmt.Sprintf("%s/projects/%v/merge_requests/%d/discussions",
		s.baseURL, projectID, mrIID)

	payload := map[string]any{
		"body": body,
		"position": map[string]any{
			"base_sha":      pos.BaseSHA,
			"start_sha":     pos.StartSHA,
			"head_sha":      pos.HeadSHA,
			"position_type": "text",
			"new_path":      pos.NewPath,
			"new_line":      pos.NewLine,
		},
	}
	if pos.OldPath != "" {
		payload["position"].(map[string]any)["old_path"] = pos.OldPath
	}
	if pos.OldLine > 0 {
		payload["position"].(map[string]any)["old_line"] = pos.OldLine
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab api error: status %d: %s", resp.StatusCode, string(b))
	}

	var res struct {
		ID    string `json:"id"`
		Notes []struct {
			ID        int       `json:"id"`
			Type      string    `json:"type"`
			Body      string    `json:"body"`
			CreatedAt time.Time `json:"created_at"`
			Author    struct {
				Username string `json:"username"`
			} `json:"author"`
		} `json:"notes"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("decode discussion response: %w", err)
	}

	th := &DiscussionThread{
		ID:    res.ID,
		Notes: make([]DiscussionNote, len(res.Notes)),
	}
	for i, n := range res.Notes {
		th.Notes[i] = DiscussionNote{
			ID:        n.ID,
			Type:      n.Type,
			Body:      n.Body,
			Author:    n.Author.Username,
			CreatedAt: n.CreatedAt,
			Position:  &pos,
		}
	}
	return th, nil
}

// ResolveDiscussion resolves or unresolves a discussion thread.
func (s *GitLabAdvancedService) ResolveDiscussion(
	ctx context.Context,
	token string,
	projectID any,
	mrIID int,
	discussionID string,
	resolved bool,
) error {
	endpoint := fmt.Sprintf("%s/projects/%v/merge_requests/%d/discussions/%s?resolved=%s",
		s.baseURL, projectID, mrIID, discussionID, strconv.FormatBool(resolved))

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gitlab api error: status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// GetMergeRequestDiffVersions retrieves all diff versions for a merge request.
func (s *GitLabAdvancedService) GetMergeRequestDiffVersions(
	ctx context.Context,
	token string,
	projectID any,
	mrIID int,
) ([]MergeRequestDiffVersion, error) {
	endpoint := fmt.Sprintf("%s/projects/%v/merge_requests/%d/versions",
		s.baseURL, projectID, mrIID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab api error: status %d: %s", resp.StatusCode, string(b))
	}

	var versions []MergeRequestDiffVersion
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return nil, fmt.Errorf("decode versions: %w", err)
	}
	return versions, nil
}

// GetMergeRequestApprovals retrieves approval status for a merge request.
func (s *GitLabAdvancedService) GetMergeRequestApprovals(
	ctx context.Context,
	token string,
	projectID any,
	mrIID int,
) (*MergeRequestApprovals, error) {
	endpoint := fmt.Sprintf("%s/projects/%v/merge_requests/%d/approvals",
		s.baseURL, projectID, mrIID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab api error: status %d: %s", resp.StatusCode, string(b))
	}

	var raw struct {
		ID                int `json:"id"`
		IID               int `json:"iid"`
		ApprovalsRequired int `json:"approvals_required"`
		ApprovalsLeft     int `json:"approvals_left"`
		ApprovedBy        []struct {
			User struct {
				Username string `json:"username"`
			} `json:"user"`
		} `json:"approved_by"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode approvals: %w", err)
	}

	res := &MergeRequestApprovals{
		ID:                raw.ID,
		IID:               raw.IID,
		ApprovalsRequired: raw.ApprovalsRequired,
		ApprovalsLeft:     raw.ApprovalsLeft,
		ApprovedBy:        make([]string, len(raw.ApprovedBy)),
	}
	for i, u := range raw.ApprovedBy {
		res.ApprovedBy[i] = u.User.Username
	}
	return res, nil
}

// GetProtectedBranches retrieves protected branch configuration for a project.
func (s *GitLabAdvancedService) GetProtectedBranches(
	ctx context.Context,
	token string,
	projectID any,
) ([]ProtectedBranchRule, error) {
	endpoint := fmt.Sprintf("%s/projects/%v/protected_branches", s.baseURL, projectID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab api error: status %d: %s", resp.StatusCode, string(b))
	}

	var rawRules []struct {
		Name                      string `json:"name"`
		AllowForcePush            bool   `json:"allow_force_push"`
		CodeOwnerApprovalRequired bool   `json:"code_owner_approval_required"`
		PushAccessLevels          []struct {
			AccessLevel int `json:"access_level"`
		} `json:"push_access_levels"`
		MergeAccessLevels []struct {
			AccessLevel int `json:"access_level"`
		} `json:"merge_access_levels"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rawRules); err != nil {
		return nil, fmt.Errorf("decode protected branches: %w", err)
	}

	rules := make([]ProtectedBranchRule, len(rawRules))
	for i, r := range rawRules {
		pushLevel := 0
		if len(r.PushAccessLevels) > 0 {
			pushLevel = r.PushAccessLevels[0].AccessLevel
		}
		mergeLevel := 0
		if len(r.MergeAccessLevels) > 0 {
			mergeLevel = r.MergeAccessLevels[0].AccessLevel
		}
		rules[i] = ProtectedBranchRule{
			Name:                      r.Name,
			PushAccessLevel:           pushLevel,
			MergeAccessLevel:          mergeLevel,
			AllowForcePush:            r.AllowForcePush,
			CodeOwnerApprovalRequired: r.CodeOwnerApprovalRequired,
		}
	}
	return rules, nil
}
