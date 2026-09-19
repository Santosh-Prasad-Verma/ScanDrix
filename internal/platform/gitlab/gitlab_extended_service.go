package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// -------------------------------------------------------------------------------------
// Extended GitLab REST API v4 Models
// -------------------------------------------------------------------------------------

// ProjectHook models a GitLab project webhook.
type ProjectHook struct {
	ID                    int       `json:"id"`
	URL                   string    `json:"url"`
	CreatedAt             time.Time `json:"created_at"`
	PushEvents            bool      `json:"push_events"`
	TagPushEvents         bool      `json:"tag_push_events"`
	MergeRequestsEvents   bool      `json:"merge_requests_events"`
	IssuesEvents          bool      `json:"issues_events"`
	NoteEvents            bool      `json:"note_events"`
	PipelineEvents        bool      `json:"pipeline_events"`
	JobEvents             bool      `json:"job_events"`
	EnableSSLVerification bool      `json:"enable_ssl_verification"`
}

// AddProjectHookRequest specifies parameters to create or update a webhook.
type AddProjectHookRequest struct {
	URL                   string `json:"url"`
	PushEvents            bool   `json:"push_events,omitempty"`
	TagPushEvents         bool   `json:"tag_push_events,omitempty"`
	MergeRequestsEvents   bool   `json:"merge_requests_events,omitempty"`
	IssuesEvents          bool   `json:"issues_events,omitempty"`
	NoteEvents            bool   `json:"note_events,omitempty"`
	PipelineEvents        bool   `json:"pipeline_events,omitempty"`
	JobEvents             bool   `json:"job_events,omitempty"`
	EnableSSLVerification bool   `json:"enable_ssl_verification,omitempty"`
	Token                 string `json:"token,omitempty"`
}

// MRApprovalRule models an approval rule for a GitLab merge request.
type MRApprovalRule struct {
	ID                int      `json:"id"`
	Name              string   `json:"name"`
	RuleType          string   `json:"rule_type"` // regular, code_owner, any_approver
	ApprovalsRequired int      `json:"approvals_required"`
	EligibleApprovers []string `json:"eligible_approvers,omitempty"`
	Users             []struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
	} `json:"users,omitempty"`
}

// CreateMRApprovalRuleRequest specifies options to create an approval rule.
type CreateMRApprovalRuleRequest struct {
	Name              string `json:"name"`
	ApprovalsRequired int    `json:"approvals_required"`
	UserIDs           []int  `json:"user_ids,omitempty"`
	GroupIDs          []int  `json:"group_ids,omitempty"`
}

// ProtectedTag models a git tag protected from deletion/force-push.
type ProtectedTag struct {
	Name               string `json:"name"`
	CreateAccessLevels []struct {
		AccessLevel            int    `json:"access_level"`
		AccessLevelDescription string `json:"access_level_description"`
	} `json:"create_access_levels"`
}

// ProjectRelease models a release record in GitLab.
type ProjectRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	ReleasedAt  time.Time `json:"released_at"`
	Commit      struct {
		ID        string `json:"id"`
		ShortID   string `json:"short_id"`
		Title     string `json:"title"`
	} `json:"commit"`
}

// CreateReleaseRequest specifies parameters to create a project release.
type CreateReleaseRequest struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Ref         string `json:"ref,omitempty"`
}

// ProjectDeployment models an environment deployment execution.
type ProjectDeployment struct {
	ID          int       `json:"id"`
	IID         int       `json:"iid"`
	Ref         string    `json:"ref"`
	SHA         string    `json:"sha"`
	Status      string    `json:"status"` // created, running, success, failed, canceled
	Environment struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"environment"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AcceptMergeRequestRequest specifies options when merging a merge request.
type AcceptMergeRequestRequest struct {
	MergeCommitMessage       string `json:"merge_commit_message,omitempty"`
	SquashCommitMessage      string `json:"squash_commit_message,omitempty"`
	Squash                   bool   `json:"squash,omitempty"`
	ShouldRemoveSourceBranch bool   `json:"should_remove_source_branch,omitempty"`
}

// MergeRequestRecord models the response after an MR action.
type MergeRequestRecord struct {
	ID              int       `json:"id"`
	IID             int       `json:"iid"`
	State           string    `json:"state"` // merged, opened, closed
	MergeCommitSHA  string    `json:"merge_commit_sha"`
	SourceBranch    string    `json:"source_branch"`
	TargetBranch    string    `json:"target_branch"`
	MergedAt        time.Time `json:"merged_at,omitempty"`
}

// -------------------------------------------------------------------------------------
// Project Webhook Management Methods
// -------------------------------------------------------------------------------------

// ListProjectHooks retrieves all webhooks configured on a project.
func (s *GitLabAdvancedService) ListProjectHooks(
	ctx context.Context,
	token string,
	projectID any,
) ([]ProjectHook, error) {
	endpoint := fmt.Sprintf("%s/projects/%s/hooks", s.baseURL, formatProjectID(projectID))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create list project hooks request: %w", err)
	}
	s.applyAuth(req, token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute list project hooks request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list project hooks error: status %d: %s", resp.StatusCode, string(b))
	}

	var hooks []ProjectHook
	if err := json.NewDecoder(resp.Body).Decode(&hooks); err != nil {
		return nil, fmt.Errorf("decode project hooks response: %w", err)
	}
	return hooks, nil
}

// GetProjectHook retrieves details for a single webhook by ID.
func (s *GitLabAdvancedService) GetProjectHook(
	ctx context.Context,
	token string,
	projectID any,
	hookID int,
) (*ProjectHook, error) {
	endpoint := fmt.Sprintf("%s/projects/%s/hooks/%d", s.baseURL, formatProjectID(projectID), hookID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create get project hook request: %w", err)
	}
	s.applyAuth(req, token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute get project hook request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get project hook error: status %d: %s", resp.StatusCode, string(b))
	}

	var hook ProjectHook
	if err := json.NewDecoder(resp.Body).Decode(&hook); err != nil {
		return nil, fmt.Errorf("decode project hook response: %w", err)
	}
	return &hook, nil
}

// AddProjectHook creates a new webhook for a project.
func (s *GitLabAdvancedService) AddProjectHook(
	ctx context.Context,
	token string,
	projectID any,
	reqPayload AddProjectHookRequest,
) (*ProjectHook, error) {
	endpoint := fmt.Sprintf("%s/projects/%s/hooks", s.baseURL, formatProjectID(projectID))

	bodyBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal add project hook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create add project hook request: %w", err)
	}
	s.applyAuth(req, token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute add project hook request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("add project hook error: status %d: %s", resp.StatusCode, string(b))
	}

	var hook ProjectHook
	if err := json.NewDecoder(resp.Body).Decode(&hook); err != nil {
		return nil, fmt.Errorf("decode added project hook response: %w", err)
	}
	return &hook, nil
}

// DeleteProjectHook deletes a webhook from a project.
func (s *GitLabAdvancedService) DeleteProjectHook(
	ctx context.Context,
	token string,
	projectID any,
	hookID int,
) error {
	endpoint := fmt.Sprintf("%s/projects/%s/hooks/%d", s.baseURL, formatProjectID(projectID), hookID)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create delete project hook request: %w", err)
	}
	s.applyAuth(req, token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute delete project hook request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete project hook error: status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// -------------------------------------------------------------------------------------
// Merge Request Approval Rules Methods
// -------------------------------------------------------------------------------------

// ListMRApprovalRules retrieves approval rules for a merge request.
func (s *GitLabAdvancedService) ListMRApprovalRules(
	ctx context.Context,
	token string,
	projectID any,
	mrIID int,
) ([]MRApprovalRule, error) {
	endpoint := fmt.Sprintf("%s/projects/%s/merge_requests/%d/approval_rules",
		s.baseURL, formatProjectID(projectID), mrIID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create list mr approval rules request: %w", err)
	}
	s.applyAuth(req, token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute list mr approval rules request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list mr approval rules error: status %d: %s", resp.StatusCode, string(b))
	}

	var rules []MRApprovalRule
	if err := json.NewDecoder(resp.Body).Decode(&rules); err != nil {
		return nil, fmt.Errorf("decode mr approval rules response: %w", err)
	}
	return rules, nil
}

// CreateMRApprovalRule adds a new approval rule to a merge request.
func (s *GitLabAdvancedService) CreateMRApprovalRule(
	ctx context.Context,
	token string,
	projectID any,
	mrIID int,
	ruleReq CreateMRApprovalRuleRequest,
) (*MRApprovalRule, error) {
	endpoint := fmt.Sprintf("%s/projects/%s/merge_requests/%d/approval_rules",
		s.baseURL, formatProjectID(projectID), mrIID)

	bodyBytes, err := json.Marshal(ruleReq)
	if err != nil {
		return nil, fmt.Errorf("marshal create mr approval rule payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create mr approval rule request: %w", err)
	}
	s.applyAuth(req, token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute create mr approval rule request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create mr approval rule error: status %d: %s", resp.StatusCode, string(b))
	}

	var rule MRApprovalRule
	if err := json.NewDecoder(resp.Body).Decode(&rule); err != nil {
		return nil, fmt.Errorf("decode created mr approval rule response: %w", err)
	}
	return &rule, nil
}

// DeleteMRApprovalRule deletes an approval rule from a merge request.
func (s *GitLabAdvancedService) DeleteMRApprovalRule(
	ctx context.Context,
	token string,
	projectID any,
	mrIID, ruleID int,
) error {
	endpoint := fmt.Sprintf("%s/projects/%s/merge_requests/%d/approval_rules/%d",
		s.baseURL, formatProjectID(projectID), mrIID, ruleID)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create delete mr approval rule request: %w", err)
	}
	s.applyAuth(req, token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute delete mr approval rule request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete mr approval rule error: status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// -------------------------------------------------------------------------------------
// Protected Tags & Release Management Methods
// -------------------------------------------------------------------------------------

// ListProtectedTags lists all protected tag patterns on a project.
func (s *GitLabAdvancedService) ListProtectedTags(
	ctx context.Context,
	token string,
	projectID any,
) ([]ProtectedTag, error) {
	endpoint := fmt.Sprintf("%s/projects/%s/protected_tags", s.baseURL, formatProjectID(projectID))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create list protected tags request: %w", err)
	}
	s.applyAuth(req, token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute list protected tags request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list protected tags error: status %d: %s", resp.StatusCode, string(b))
	}

	var tags []ProtectedTag
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, fmt.Errorf("decode protected tags response: %w", err)
	}
	return tags, nil
}

// ProtectTag protects a tag or tag pattern.
func (s *GitLabAdvancedService) ProtectTag(
	ctx context.Context,
	token string,
	projectID any,
	tagName string,
	createAccessLevel int,
) (*ProtectedTag, error) {
	endpoint := fmt.Sprintf("%s/projects/%s/protected_tags", s.baseURL, formatProjectID(projectID))

	payload := map[string]any{
		"name":                tagName,
		"create_access_level": createAccessLevel,
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create protect tag request: %w", err)
	}
	s.applyAuth(req, token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute protect tag request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("protect tag error: status %d: %s", resp.StatusCode, string(b))
	}

	var tag ProtectedTag
	if err := json.NewDecoder(resp.Body).Decode(&tag); err != nil {
		return nil, fmt.Errorf("decode protected tag response: %w", err)
	}
	return &tag, nil
}

// UnprotectTag removes protection from a tag pattern.
func (s *GitLabAdvancedService) UnprotectTag(
	ctx context.Context,
	token string,
	projectID any,
	tagName string,
) error {
	endpoint := fmt.Sprintf("%s/projects/%s/protected_tags/%s",
		s.baseURL, formatProjectID(projectID), url.PathEscape(tagName))

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create unprotect tag request: %w", err)
	}
	s.applyAuth(req, token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute unprotect tag request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unprotect tag error: status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// ListReleases lists all releases created for a project.
func (s *GitLabAdvancedService) ListReleases(
	ctx context.Context,
	token string,
	projectID any,
) ([]ProjectRelease, error) {
	endpoint := fmt.Sprintf("%s/projects/%s/releases", s.baseURL, formatProjectID(projectID))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create list releases request: %w", err)
	}
	s.applyAuth(req, token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute list releases request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list releases error: status %d: %s", resp.StatusCode, string(b))
	}

	var releases []ProjectRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decode releases response: %w", err)
	}
	return releases, nil
}

// CreateRelease creates a new release in GitLab.
func (s *GitLabAdvancedService) CreateRelease(
	ctx context.Context,
	token string,
	projectID any,
	reqPayload CreateReleaseRequest,
) (*ProjectRelease, error) {
	endpoint := fmt.Sprintf("%s/projects/%s/releases", s.baseURL, formatProjectID(projectID))

	bodyBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal create release payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create release request: %w", err)
	}
	s.applyAuth(req, token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute create release request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create release error: status %d: %s", resp.StatusCode, string(b))
	}

	var rel ProjectRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode created release response: %w", err)
	}
	return &rel, nil
}

// AcceptMergeRequest merges an open merge request.
func (s *GitLabAdvancedService) AcceptMergeRequest(
	ctx context.Context,
	token string,
	projectID any,
	mrIID int,
	mergeReq AcceptMergeRequestRequest,
) (*MergeRequestRecord, error) {
	endpoint := fmt.Sprintf("%s/projects/%s/merge_requests/%d/merge",
		s.baseURL, formatProjectID(projectID), mrIID)

	bodyBytes, err := json.Marshal(mergeReq)
	if err != nil {
		return nil, fmt.Errorf("marshal accept merge request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create accept merge request: %w", err)
	}
	s.applyAuth(req, token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute accept merge request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("accept merge request error: status %d: %s", resp.StatusCode, string(b))
	}

	var record MergeRequestRecord
	if err := json.NewDecoder(resp.Body).Decode(&record); err != nil {
		return nil, fmt.Errorf("decode merge request record: %w", err)
	}
	return &record, nil
}

// ListDeployments retrieves deployments for a project, optionally filtered by environment.
func (s *GitLabAdvancedService) ListDeployments(
	ctx context.Context,
	token string,
	projectID any,
	envName string,
) ([]ProjectDeployment, error) {
	u, err := url.Parse(fmt.Sprintf("%s/projects/%s/deployments", s.baseURL, formatProjectID(projectID)))
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	q := u.Query()
	if envName != "" {
		q.Set("environment", envName)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create list deployments request: %w", err)
	}
	s.applyAuth(req, token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute list deployments request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list deployments error: status %d: %s", resp.StatusCode, string(b))
	}

	var deployments []ProjectDeployment
	if err := json.NewDecoder(resp.Body).Decode(&deployments); err != nil {
		return nil, fmt.Errorf("decode deployments response: %w", err)
	}
	return deployments, nil
}

func (s *GitLabAdvancedService) applyAuth(req *http.Request, token string) {
	if token != "" {
		if strings.HasPrefix(token, "Bearer ") {
			req.Header.Set("Authorization", token)
		} else {
			req.Header.Set("PRIVATE-TOKEN", token)
		}
	}
}

func formatProjectID(p any) string {
	switch v := p.(type) {
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case string:
		return url.PathEscape(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}
