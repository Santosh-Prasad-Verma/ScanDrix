package azuredevops

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
// Policy Configuration Models
// -------------------------------------------------------------------------------------

// PolicyTypeInfo describes the type of branch policy.
type PolicyTypeInfo struct {
	ID          string `json:"id"`
	URL         string `json:"url,omitempty"`
	DisplayName string `json:"displayName"`
	Description string `json:"description,omitempty"`
}

// PolicyScope defines repository and ref scope for a policy configuration.
type PolicyScope struct {
	RepositoryID string `json:"repositoryId,omitempty"`
	RefName      string `json:"refName,omitempty"`
	MatchKind    string `json:"matchKind,omitempty"` // Exact, Prefix, DefaultBranch
}

// PolicySettings encapsulates configuration settings for various policy types.
type PolicySettings struct {
	Scope                 []PolicyScope `json:"scope,omitempty"`
	MinimumApproverCount  int           `json:"minimumApproverCount,omitempty"`
	CreatorVoteCounts     bool          `json:"creatorVoteCounts,omitempty"`
	AllowDownvotes        bool          `json:"allowDownvotes,omitempty"`
	ResetOnSourcePush     bool          `json:"resetOnSourcePush,omitempty"`
	BuildDefinitionID     int           `json:"buildDefinitionId,omitempty"`
	DisplayName           string        `json:"displayName,omitempty"`
	QueueBuildOnCreate    bool          `json:"queueBuildOnCreate,omitempty"`
	ManualQueueOnly       bool          `json:"manualQueueOnly,omitempty"`
	ValidDuration         int           `json:"validDuration,omitempty"`
	StatusName            string        `json:"statusName,omitempty"`
	StatusGenre           string        `json:"statusGenre,omitempty"`
	AuthorID              string        `json:"authorId,omitempty"`
	RequireAllWorkItems   bool          `json:"requireAllWorkItems,omitempty"`
	RequireResolution     bool          `json:"requireResolution,omitempty"`
	CustomSettings        map[string]any `json:"customSettings,omitempty"`
}

// PolicyConfiguration represents an Azure DevOps policy configuration.
type PolicyConfiguration struct {
	ID          int            `json:"id"`
	URL         string         `json:"url"`
	Type        PolicyTypeInfo `json:"type"`
	IsEnabled   bool           `json:"isEnabled"`
	IsBlocking  bool           `json:"isBlocking"`
	IsDeleted   bool           `json:"isDeleted"`
	Settings    PolicySettings `json:"settings"`
	Revision    int            `json:"revision"`
	CreatedDate time.Time      `json:"createdDate"`
}

// CreatePolicyConfigRequest is the payload for creating a policy configuration.
type CreatePolicyConfigRequest struct {
	Type       PolicyTypeInfo `json:"type"`
	IsEnabled  bool           `json:"isEnabled"`
	IsBlocking bool           `json:"isBlocking"`
	Settings   PolicySettings `json:"settings"`
}

// UpdatePolicyConfigRequest is the payload for updating a policy configuration.
type UpdatePolicyConfigRequest struct {
	IsEnabled  *bool           `json:"isEnabled,omitempty"`
	IsBlocking *bool           `json:"isBlocking,omitempty"`
	Settings   *PolicySettings `json:"settings,omitempty"`
}

// PolicyEvaluationRecord models an evaluation record for a pull request.
type PolicyEvaluationRecord struct {
	EvaluationID  string              `json:"evaluationId"`
	Status        string              `json:"status"` // queued, running, approved, rejected, notApplicable, broken
	Configuration PolicyConfiguration `json:"configuration"`
	StartedDate   time.Time           `json:"startedDate"`
	CompletedDate time.Time           `json:"completedDate,omitempty"`
	Context       map[string]any      `json:"context,omitempty"`
}

// -------------------------------------------------------------------------------------
// Build and Pipeline Models
// -------------------------------------------------------------------------------------

// BuildDefinition represents an Azure DevOps build pipeline definition.
type BuildDefinition struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	Type        string    `json:"type"`
	QueueStatus string    `json:"queueStatus"`
	Revision    int       `json:"revision"`
	Project     string    `json:"project"`
	URL         string    `json:"url"`
	CreatedDate time.Time `json:"createdDate,omitempty"`
}

// BuildRecord represents a single execution of a build pipeline.
type BuildRecord struct {
	ID            int             `json:"id"`
	BuildNumber   string          `json:"buildNumber"`
	Status        string          `json:"status"` // inProgress, completed, cancelling, postponed, notStarted
	Result        string          `json:"result"` // succeeded, partiallySucceeded, failed, canceled
	QueueTime     time.Time       `json:"queueTime"`
	StartTime     time.Time       `json:"startTime"`
	FinishTime    time.Time       `json:"finishTime,omitempty"`
	SourceBranch  string          `json:"sourceBranch"`
	SourceVersion string          `json:"sourceVersion"`
	Definition    BuildDefinition `json:"definition"`
	URL           string          `json:"url"`
}

// QueueBuildRequest specifies options for queueing a new build.
type QueueBuildRequest struct {
	DefinitionID  int               `json:"definitionId"`
	SourceBranch  string            `json:"sourceBranch,omitempty"`
	SourceVersion string            `json:"sourceVersion,omitempty"`
	Parameters    map[string]string `json:"parameters,omitempty"`
}

// ListBuildsOptions specifies query parameters when listing builds.
type ListBuildsOptions struct {
	Definitions  []int
	QueuedBy     string
	MinTime      time.Time
	MaxTime      time.Time
	StatusFilter string
	ResultFilter string
	Top          int
}

// BuildTimelineIssue represents an issue/warning/error in the build timeline.
type BuildTimelineIssue struct {
	Type    string `json:"type"` // error, warning
	Category string `json:"category,omitempty"`
	Message string `json:"message"`
}

// BuildTimelineRecord represents a step or phase in a build execution timeline.
type BuildTimelineRecord struct {
	ID              string               `json:"id"`
	ParentID        string               `json:"parentId,omitempty"`
	Type            string               `json:"type"` // Stage, Phase, Job, Task
	Name            string               `json:"name"`
	Order           int                  `json:"order"`
	State           string               `json:"state"` // pending, inProgress, completed
	Result          string               `json:"result,omitempty"`
	StartTime       time.Time            `json:"startTime,omitempty"`
	FinishTime      time.Time            `json:"finishTime,omitempty"`
	PercentComplete int                  `json:"percentComplete,omitempty"`
	Issues          []BuildTimelineIssue `json:"issues,omitempty"`
}

// BuildArtifact represents an artifact produced by a build.
type BuildArtifact struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Resource struct {
		Type        string `json:"type"`
		DownloadURL string `json:"downloadUrl"`
		Properties  map[string]any `json:"properties,omitempty"`
	} `json:"resource"`
}

// -------------------------------------------------------------------------------------
// Pull Request Status Models
// -------------------------------------------------------------------------------------

// PRStatusContext defines the genre and name of a PR status.
type PRStatusContext struct {
	Genre string `json:"genre"`
	Name  string `json:"name"`
}

// PRStatusInput specifies parameters to create or update a PR status.
type PRStatusInput struct {
	State       string          `json:"state"` // succeeded, failed, pending, error
	Description string          `json:"description"`
	Context     PRStatusContext `json:"context"`
	TargetURL   string          `json:"targetUrl,omitempty"`
}

// PRStatusRecord represents a status check attached to an Azure DevOps PR.
type PRStatusRecord struct {
	ID           int             `json:"id"`
	State        string          `json:"state"`
	Description  string          `json:"description"`
	Context      PRStatusContext `json:"context"`
	TargetURL    string          `json:"targetUrl,omitempty"`
	CreatedBy    string          `json:"createdBy,omitempty"`
	CreationDate time.Time       `json:"creationDate"`
	UpdatedDate  time.Time       `json:"updatedDate"`
}

// -------------------------------------------------------------------------------------
// Iteration Changes Models
// -------------------------------------------------------------------------------------

// IterationChangeItem represents a file modified in a PR iteration.
type IterationChangeItem struct {
	ChangeID   int    `json:"changeId"`
	ChangeType string `json:"changeType"` // add, edit, delete, rename
	Item       struct {
		Path             string `json:"path"`
		OriginalObjectID string `json:"originalObjectId,omitempty"`
		ObjectID         string `json:"objectId,omitempty"`
	} `json:"item"`
}

// IterationChangesResult holds the collection of file changes for an iteration.
type IterationChangesResult struct {
	ChangeTrackingID int                   `json:"changeTrackingId"`
	Changes          []IterationChangeItem `json:"changes"`
	NextSkip         int                   `json:"nextSkip,omitempty"`
	NextTop          int                   `json:"nextTop,omitempty"`
}

// IterationComparisonResult holds differences between two PR iterations.
type IterationComparisonResult struct {
	BaseIterationID   int                   `json:"baseIterationId"`
	TargetIterationID int                   `json:"targetIterationId"`
	Commits           []string              `json:"commits"`
	Changes           []IterationChangeItem `json:"changes"`
}

// -------------------------------------------------------------------------------------
// Policy Configuration API Methods
// -------------------------------------------------------------------------------------

// GetPolicyConfigurations retrieves policy configurations for a repository and ref.
func (s *AzureDevOpsAdvancedService) GetPolicyConfigurations(
	ctx context.Context,
	token, org, project string,
	repositoryID, refName string,
) ([]PolicyConfiguration, error) {
	u, err := url.Parse(fmt.Sprintf("%s/%s/%s/_apis/policy/configurations", s.baseURL, org, project))
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	q := u.Query()
	q.Set("api-version", "7.1-preview.1")
	if repositoryID != "" {
		q.Set("repositoryId", repositoryID)
	}
	if refName != "" {
		q.Set("refName", refName)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create policy configurations request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute policy configurations request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("policy configurations error: status %d: %s", resp.StatusCode, string(b))
	}

	var wrapper struct {
		Value []PolicyConfiguration `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("decode policy configurations response: %w", err)
	}
	return wrapper.Value, nil
}

// GetPolicyConfiguration retrieves a single policy configuration by ID.
func (s *AzureDevOpsAdvancedService) GetPolicyConfiguration(
	ctx context.Context,
	token, org, project string,
	configurationID int,
) (*PolicyConfiguration, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/policy/configurations/%d?api-version=7.1-preview.1",
		s.baseURL, org, project, configurationID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create get policy configuration request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute get policy configuration request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get policy configuration error: status %d: %s", resp.StatusCode, string(b))
	}

	var config PolicyConfiguration
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return nil, fmt.Errorf("decode policy configuration response: %w", err)
	}
	return &config, nil
}

// CreatePolicyConfiguration creates a new branch policy configuration.
func (s *AzureDevOpsAdvancedService) CreatePolicyConfiguration(
	ctx context.Context,
	token, org, project string,
	configReq CreatePolicyConfigRequest,
) (*PolicyConfiguration, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/policy/configurations?api-version=7.1-preview.1",
		s.baseURL, org, project)

	bodyBytes, err := json.Marshal(configReq)
	if err != nil {
		return nil, fmt.Errorf("marshal create policy configuration: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute create policy configuration request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create policy configuration error: status %d: %s", resp.StatusCode, string(b))
	}

	var created PolicyConfiguration
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return nil, fmt.Errorf("decode created policy configuration: %w", err)
	}
	return &created, nil
}

// UpdatePolicyConfiguration updates an existing branch policy configuration.
func (s *AzureDevOpsAdvancedService) UpdatePolicyConfiguration(
	ctx context.Context,
	token, org, project string,
	configurationID int,
	updateReq UpdatePolicyConfigRequest,
) (*PolicyConfiguration, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/policy/configurations/%d?api-version=7.1-preview.1",
		s.baseURL, org, project, configurationID)

	bodyBytes, err := json.Marshal(updateReq)
	if err != nil {
		return nil, fmt.Errorf("marshal update policy configuration: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create update policy request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute update policy configuration request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("update policy configuration error: status %d: %s", resp.StatusCode, string(b))
	}

	var updated PolicyConfiguration
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		return nil, fmt.Errorf("decode updated policy configuration: %w", err)
	}
	return &updated, nil
}

// DeletePolicyConfiguration deletes a branch policy configuration.
func (s *AzureDevOpsAdvancedService) DeletePolicyConfiguration(
	ctx context.Context,
	token, org, project string,
	configurationID int,
) error {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/policy/configurations/%d?api-version=7.1-preview.1",
		s.baseURL, org, project, configurationID)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create delete policy request: %w", err)
	}
	req.SetBasicAuth("", token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute delete policy configuration request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete policy configuration error: status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// RequeuePolicyEvaluation requests a re-evaluation of a specific policy for a PR.
func (s *AzureDevOpsAdvancedService) RequeuePolicyEvaluation(
	ctx context.Context,
	token, org, project string,
	evaluationID string,
) (*PolicyEvaluationRecord, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/policy/evaluations/%s?api-version=7.1-preview.1",
		s.baseURL, org, project, evaluationID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create requeue policy evaluation request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute requeue policy evaluation request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("requeue policy evaluation error: status %d: %s", resp.StatusCode, string(b))
	}

	var eval PolicyEvaluationRecord
	if err := json.NewDecoder(resp.Body).Decode(&eval); err != nil {
		return nil, fmt.Errorf("decode policy evaluation response: %w", err)
	}
	return &eval, nil
}

// -------------------------------------------------------------------------------------
// Build & Pipeline API Methods
// -------------------------------------------------------------------------------------

// GetBuildDefinitions retrieves available build definitions for a project.
func (s *AzureDevOpsAdvancedService) GetBuildDefinitions(
	ctx context.Context,
	token, org, project string,
) ([]BuildDefinition, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/build/definitions?api-version=7.1-preview.7",
		s.baseURL, org, project)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create get build definitions request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute get build definitions request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get build definitions error: status %d: %s", resp.StatusCode, string(b))
	}

	var wrapper struct {
		Value []struct {
			ID          int    `json:"id"`
			Name        string `json:"name"`
			Path        string `json:"path"`
			Type        string `json:"type"`
			QueueStatus string `json:"queueStatus"`
			Revision    int    `json:"revision"`
			URL         string `json:"url"`
			Project     struct {
				Name string `json:"name"`
			} `json:"project"`
			CreatedDate time.Time `json:"createdDate"`
		} `json:"value"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("decode build definitions response: %w", err)
	}

	defs := make([]BuildDefinition, len(wrapper.Value))
	for i, d := range wrapper.Value {
		defs[i] = BuildDefinition{
			ID:          d.ID,
			Name:        d.Name,
			Path:        d.Path,
			Type:        d.Type,
			QueueStatus: d.QueueStatus,
			Revision:    d.Revision,
			Project:     d.Project.Name,
			URL:         d.URL,
			CreatedDate: d.CreatedDate,
		}
	}
	return defs, nil
}

// QueueBuild queues a new execution of a build pipeline.
func (s *AzureDevOpsAdvancedService) QueueBuild(
	ctx context.Context,
	token, org, project string,
	queueReq QueueBuildRequest,
) (*BuildRecord, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/build/builds?api-version=7.1-preview.7",
		s.baseURL, org, project)

	payload := map[string]any{
		"definition": map[string]any{
			"id": queueReq.DefinitionID,
		},
	}
	if queueReq.SourceBranch != "" {
		payload["sourceBranch"] = queueReq.SourceBranch
	}
	if queueReq.SourceVersion != "" {
		payload["sourceVersion"] = queueReq.SourceVersion
	}
	if len(queueReq.Parameters) > 0 {
		paramBytes, _ := json.Marshal(queueReq.Parameters)
		payload["parameters"] = string(paramBytes)
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal queue build payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create queue build request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute queue build request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("queue build error: status %d: %s", resp.StatusCode, string(b))
	}

	return s.parseBuildRecord(resp.Body)
}

// GetBuild retrieves details for a single build by ID.
func (s *AzureDevOpsAdvancedService) GetBuild(
	ctx context.Context,
	token, org, project string,
	buildID int,
) (*BuildRecord, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/build/builds/%d?api-version=7.1-preview.7",
		s.baseURL, org, project, buildID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create get build request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute get build request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get build error: status %d: %s", resp.StatusCode, string(b))
	}

	return s.parseBuildRecord(resp.Body)
}

// ListBuilds searches and lists builds based on provided filter criteria.
func (s *AzureDevOpsAdvancedService) ListBuilds(
	ctx context.Context,
	token, org, project string,
	opts ListBuildsOptions,
) ([]BuildRecord, error) {
	u, err := url.Parse(fmt.Sprintf("%s/%s/%s/_apis/build/builds", s.baseURL, org, project))
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	q := u.Query()
	q.Set("api-version", "7.1-preview.7")
	if len(opts.Definitions) > 0 {
		defs := make([]string, len(opts.Definitions))
		for i, d := range opts.Definitions {
			defs[i] = strconv.Itoa(d)
		}
		q.Set("definitions", strings.Join(defs, ","))
	}
	if opts.QueuedBy != "" {
		q.Set("queuedBy", opts.QueuedBy)
	}
	if !opts.MinTime.IsZero() {
		q.Set("minTime", opts.MinTime.Format(time.RFC3339))
	}
	if !opts.MaxTime.IsZero() {
		q.Set("maxTime", opts.MaxTime.Format(time.RFC3339))
	}
	if opts.StatusFilter != "" {
		q.Set("statusFilter", opts.StatusFilter)
	}
	if opts.ResultFilter != "" {
		q.Set("resultFilter", opts.ResultFilter)
	}
	if opts.Top > 0 {
		q.Set("$top", strconv.Itoa(opts.Top))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create list builds request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute list builds request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list builds error: status %d: %s", resp.StatusCode, string(b))
	}

	var wrapper struct {
		Value []json.RawMessage `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("decode list builds response: %w", err)
	}

	records := make([]BuildRecord, 0, len(wrapper.Value))
	for _, raw := range wrapper.Value {
		b, err := s.parseBuildRecord(bytes.NewReader(raw))
		if err == nil && b != nil {
			records = append(records, *b)
		}
	}
	return records, nil
}

// GetBuildTimeline retrieves the detailed step execution timeline for a build.
func (s *AzureDevOpsAdvancedService) GetBuildTimeline(
	ctx context.Context,
	token, org, project string,
	buildID int,
) ([]BuildTimelineRecord, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/build/builds/%d/timeline?api-version=7.1-preview.2",
		s.baseURL, org, project, buildID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create get build timeline request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute get build timeline request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get build timeline error: status %d: %s", resp.StatusCode, string(b))
	}

	var wrapper struct {
		Records []BuildTimelineRecord `json:"records"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("decode build timeline response: %w", err)
	}
	return wrapper.Records, nil
}

// GetBuildArtifacts retrieves the artifacts associated with a build.
func (s *AzureDevOpsAdvancedService) GetBuildArtifacts(
	ctx context.Context,
	token, org, project string,
	buildID int,
) ([]BuildArtifact, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/build/builds/%d/artifacts?api-version=7.1-preview.5",
		s.baseURL, org, project, buildID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create get build artifacts request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute get build artifacts request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get build artifacts error: status %d: %s", resp.StatusCode, string(b))
	}

	var wrapper struct {
		Value []BuildArtifact `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("decode build artifacts response: %w", err)
	}
	return wrapper.Value, nil
}

// CancelBuild cancels an actively running or queued build.
func (s *AzureDevOpsAdvancedService) CancelBuild(
	ctx context.Context,
	token, org, project string,
	buildID int,
	reason string,
) (*BuildRecord, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/build/builds/%d?api-version=7.1-preview.7",
		s.baseURL, org, project, buildID)

	payload := map[string]any{
		"status": "Cancelling",
	}
	if reason != "" {
		payload["reason"] = reason
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create cancel build request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute cancel build request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("cancel build error: status %d: %s", resp.StatusCode, string(b))
	}

	return s.parseBuildRecord(resp.Body)
}

func (s *AzureDevOpsAdvancedService) parseBuildRecord(r io.Reader) (*BuildRecord, error) {
	var res struct {
		ID            int       `json:"id"`
		BuildNumber   string    `json:"buildNumber"`
		Status        string    `json:"status"`
		Result        string    `json:"result"`
		QueueTime     time.Time `json:"queueTime"`
		StartTime     time.Time `json:"startTime"`
		FinishTime    time.Time `json:"finishTime"`
		SourceBranch  string    `json:"sourceBranch"`
		SourceVersion string    `json:"sourceVersion"`
		URL           string    `json:"url"`
		Definition    struct {
			ID          int    `json:"id"`
			Name        string `json:"name"`
			Path        string `json:"path"`
			Type        string `json:"type"`
			QueueStatus string `json:"queueStatus"`
			Revision    int    `json:"revision"`
			URL         string `json:"url"`
			Project     struct {
				Name string `json:"name"`
			} `json:"project"`
		} `json:"definition"`
	}

	if err := json.NewDecoder(r).Decode(&res); err != nil {
		return nil, fmt.Errorf("decode build record: %w", err)
	}

	return &BuildRecord{
		ID:            res.ID,
		BuildNumber:   res.BuildNumber,
		Status:        res.Status,
		Result:        res.Result,
		QueueTime:     res.QueueTime,
		StartTime:     res.StartTime,
		FinishTime:    res.FinishTime,
		SourceBranch:  res.SourceBranch,
		SourceVersion: res.SourceVersion,
		URL:           res.URL,
		Definition: BuildDefinition{
			ID:          res.Definition.ID,
			Name:        res.Definition.Name,
			Path:        res.Definition.Path,
			Type:        res.Definition.Type,
			QueueStatus: res.Definition.QueueStatus,
			Revision:    res.Definition.Revision,
			Project:     res.Definition.Project.Name,
			URL:         res.Definition.URL,
		},
	}, nil
}

// -------------------------------------------------------------------------------------
// Pull Request Statuses API Methods
// -------------------------------------------------------------------------------------

// CreatePullRequestStatus posts a new status check on a pull request.
func (s *AzureDevOpsAdvancedService) CreatePullRequestStatus(
	ctx context.Context,
	token, org, project, repoID string,
	prID int,
	status PRStatusInput,
) (*PRStatusRecord, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/pullRequests/%d/statuses?api-version=7.1-preview.1",
		s.baseURL, org, project, repoID, prID)

	bodyBytes, err := json.Marshal(status)
	if err != nil {
		return nil, fmt.Errorf("marshal pr status: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create pr status request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute pr status request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create pr status error: status %d: %s", resp.StatusCode, string(b))
	}

	var created PRStatusRecord
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return nil, fmt.Errorf("decode pr status response: %w", err)
	}
	return &created, nil
}

// GetPullRequestStatuses retrieves all status checks for a pull request.
func (s *AzureDevOpsAdvancedService) GetPullRequestStatuses(
	ctx context.Context,
	token, org, project, repoID string,
	prID int,
) ([]PRStatusRecord, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/pullRequests/%d/statuses?api-version=7.1-preview.1",
		s.baseURL, org, project, repoID, prID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create get pr statuses request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute get pr statuses request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get pr statuses error: status %d: %s", resp.StatusCode, string(b))
	}

	var wrapper struct {
		Value []PRStatusRecord `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("decode pr statuses response: %w", err)
	}
	return wrapper.Value, nil
}

// DeletePullRequestStatus deletes an existing status check by ID.
func (s *AzureDevOpsAdvancedService) DeletePullRequestStatus(
	ctx context.Context,
	token, org, project, repoID string,
	prID int,
	statusID int,
) error {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/pullRequests/%d/statuses/%d?api-version=7.1-preview.1",
		s.baseURL, org, project, repoID, prID, statusID)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create delete pr status request: %w", err)
	}
	req.SetBasicAuth("", token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute delete pr status request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete pr status error: status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// -------------------------------------------------------------------------------------
// Iteration Changes API Methods
// -------------------------------------------------------------------------------------

// GetIterationChanges retrieves the file changes made in a specific PR iteration.
func (s *AzureDevOpsAdvancedService) GetIterationChanges(
	ctx context.Context,
	token, org, project, repoID string,
	prID int,
	iterationID int,
	top, skip int,
) (*IterationChangesResult, error) {
	u, err := url.Parse(fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/pullRequests/%d/iterations/%d/changes",
		s.baseURL, org, project, repoID, prID, iterationID))
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	q := u.Query()
	q.Set("api-version", "7.1-preview.1")
	if top > 0 {
		q.Set("$top", strconv.Itoa(top))
	}
	if skip > 0 {
		q.Set("$skip", strconv.Itoa(skip))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create get iteration changes request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute get iteration changes request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get iteration changes error: status %d: %s", resp.StatusCode, string(b))
	}

	var res struct {
		ChangeTrackingID int `json:"changeTrackingId"`
		ChangeEntries    []struct {
			ChangeID   int    `json:"changeId"`
			ChangeType string `json:"changeType"`
			Item       struct {
				Path             string `json:"path"`
				OriginalObjectID string `json:"originalObjectId"`
				ObjectID         string `json:"objectId"`
			} `json:"item"`
		} `json:"changeEntries"`
		NextSkip int `json:"nextSkip"`
		NextTop  int `json:"nextTop"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("decode iteration changes response: %w", err)
	}

	changes := make([]IterationChangeItem, len(res.ChangeEntries))
	for i, c := range res.ChangeEntries {
		changes[i] = IterationChangeItem{
			ChangeID:   c.ChangeID,
			ChangeType: c.ChangeType,
			Item: struct {
				Path             string `json:"path"`
				OriginalObjectID string `json:"originalObjectId,omitempty"`
				ObjectID         string `json:"objectId,omitempty"`
			}{
				Path:             c.Item.Path,
				OriginalObjectID: c.Item.OriginalObjectID,
				ObjectID:         c.Item.ObjectID,
			},
		}
	}

	return &IterationChangesResult{
		ChangeTrackingID: res.ChangeTrackingID,
		Changes:          changes,
		NextSkip:         res.NextSkip,
		NextTop:          res.NextTop,
	}, nil
}

// CompareIterations compares two pull request iterations to determine commits and file diffs.
func (s *AzureDevOpsAdvancedService) CompareIterations(
	ctx context.Context,
	token, org, project, repoID string,
	prID int,
	targetIterationID, baseIterationID int,
) (*IterationComparisonResult, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/pullRequests/%d/iterations/%d/changes?api-version=7.1-preview.1&$compareTo=%d",
		s.baseURL, org, project, repoID, prID, targetIterationID, baseIterationID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create compare iterations request: %w", err)
	}
	req.SetBasicAuth("", token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute compare iterations request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("compare iterations error: status %d: %s", resp.StatusCode, string(b))
	}

	var res struct {
		ChangeEntries []struct {
			ChangeID   int    `json:"changeId"`
			ChangeType string `json:"changeType"`
			Item       struct {
				Path             string `json:"path"`
				OriginalObjectID string `json:"originalObjectId"`
				ObjectID         string `json:"objectId"`
			} `json:"item"`
		} `json:"changeEntries"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("decode iteration comparison response: %w", err)
	}

	changes := make([]IterationChangeItem, len(res.ChangeEntries))
	for i, c := range res.ChangeEntries {
		changes[i] = IterationChangeItem{
			ChangeID:   c.ChangeID,
			ChangeType: c.ChangeType,
			Item: struct {
				Path             string `json:"path"`
				OriginalObjectID string `json:"originalObjectId,omitempty"`
				ObjectID         string `json:"objectId,omitempty"`
			}{
				Path:             c.Item.Path,
				OriginalObjectID: c.Item.OriginalObjectID,
				ObjectID:         c.Item.ObjectID,
			},
		}
	}

	return &IterationComparisonResult{
		BaseIterationID:   baseIterationID,
		TargetIterationID: targetIterationID,
		Changes:           changes,
	}, nil
}
