package ingestion

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// WebhookParser transforms heterogeneous VCS webhook payloads into normalized events.
type WebhookParser struct{}

// NewWebhookParser initializes the parser.
func NewWebhookParser() *WebhookParser {
	return &WebhookParser{}
}

// Parse converts a payload from any supported provider into a NormalizedWebhookEvent.
func (p *WebhookParser) Parse(provider models.SCMProvider, eventType string, body []byte) (*NormalizedWebhookEvent, error) {
	switch provider {
	case models.ProviderGitHub:
		return p.parseGitHub(eventType, body)
	case models.ProviderGitLab:
		return p.parseGitLab(eventType, body)
	case models.ProviderBitbucket:
		return p.parseBitbucket(eventType, body)
	case models.ProviderAzure:
		return p.parseAzureDevOps(eventType, body)
	case models.ProviderForgejo:
		return p.parseForgejo(eventType, body)
	default:
		return nil, fmt.Errorf("unsupported webhook provider: %s", provider)
	}
}

func (p *WebhookParser) parseGitHub(eventType string, body []byte) (*NormalizedWebhookEvent, error) {
	if eventType == "pull_request_review_comment" || eventType == "issue_comment" {
		var commentData struct {
			Action string `json:"action"`
			Issue  *struct {
				Number int `json:"number"`
			} `json:"issue"`
			PullRequest *struct {
				Number int `json:"number"`
			} `json:"pull_request"`
			Comment struct {
				ID       int64  `json:"id"`
				Body     string `json:"body"`
				Path     string `json:"path"`
				DiffHunk string `json:"diff_hunk"`
				User     struct {
					Login string `json:"login"`
				} `json:"user"`
			} `json:"comment"`
			Repository struct {
				FullName string `json:"full_name"`
			} `json:"repository"`
		}

		if err := json.Unmarshal(body, &commentData); err != nil {
			return nil, err
		}

		prNum := 0
		if commentData.PullRequest != nil {
			prNum = commentData.PullRequest.Number
		} else if commentData.Issue != nil {
			prNum = commentData.Issue.Number
		}

		action := ActionCommentCreated
		dismissReason := ""
		lowerBody := strings.ToLower(commentData.Comment.Body)
		if strings.Contains(lowerBody, "@scandrix dismiss") || strings.Contains(lowerBody, "@scandrix false-positive") || strings.Contains(lowerBody, "@scandrix ignore") {
			action = ActionFeedbackDismissed
			dismissReason = "FALSE_POSITIVE"
			if strings.Contains(lowerBody, "wont-fix") || strings.Contains(lowerBody, "won't fix") {
				dismissReason = "WONT_FIX"
			}
		}

		return &NormalizedWebhookEvent{
			ID:                uuid.New(),
			Provider:          models.ProviderGitHub,
			Action:            action,
			DismissalReason:   dismissReason,
			RepoNamespace:     commentData.Repository.FullName,
			PullRequestNumber: prNum,
			CommentID:         commentData.Comment.ID,
			CommentBody:       commentData.Comment.Body,
			CommentFilePath:   commentData.Comment.Path,
			DiffHunk:          commentData.Comment.DiffHunk,
			Sender:            commentData.Comment.User.Login,
			RawPayload:        body,
			ReceivedAt:        time.Now().UTC(),
		}, nil
	}

	if eventType == "installation" {
		var installData struct {
			Action       string `json:"action"`
			Installation struct {
				ID      int64 `json:"id"`
				Account struct {
					Login string `json:"login"`
				} `json:"account"`
			} `json:"installation"`
			Repositories []struct {
				FullName string `json:"full_name"`
			} `json:"repositories"`
			Sender struct {
				Login string `json:"login"`
			} `json:"sender"`
		}
		if err := json.Unmarshal(body, &installData); err != nil {
			return nil, err
		}

		action := ActionIgnored
		switch installData.Action {
		case "created":
			action = ActionInstallationCreated
		case "deleted", "suspend":
			action = ActionInstallationDeleted
		}

		repos := make([]string, len(installData.Repositories))
		for i, r := range installData.Repositories {
			repos[i] = r.FullName
		}

		return &NormalizedWebhookEvent{
			ID:             uuid.New(),
			Provider:       models.ProviderGitHub,
			Action:         action,
			RepoNamespace:  installData.Installation.Account.Login,
			InstallationID: installData.Installation.ID,
			Repositories:   repos,
			Sender:         installData.Sender.Login,
			RawPayload:     body,
			ReceivedAt:     time.Now().UTC(),
		}, nil
	}

	if eventType == "installation_repositories" {
		var repoData struct {
			Action       string `json:"action"`
			Installation struct {
				ID      int64 `json:"id"`
				Account struct {
					Login string `json:"login"`
				} `json:"account"`
			} `json:"installation"`
			RepositoriesAdded []struct {
				FullName string `json:"full_name"`
			} `json:"repositories_added"`
			RepositoriesRemoved []struct {
				FullName string `json:"full_name"`
			} `json:"repositories_removed"`
			Sender struct {
				Login string `json:"login"`
			} `json:"sender"`
		}
		if err := json.Unmarshal(body, &repoData); err != nil {
			return nil, err
		}

		action := ActionIgnored
		var repos []string
		if repoData.Action == "added" {
			action = ActionReposAdded
			for _, r := range repoData.RepositoriesAdded {
				repos = append(repos, r.FullName)
			}
		} else if repoData.Action == "removed" {
			action = ActionReposRemoved
			for _, r := range repoData.RepositoriesRemoved {
				repos = append(repos, r.FullName)
			}
		}

		return &NormalizedWebhookEvent{
			ID:             uuid.New(),
			Provider:       models.ProviderGitHub,
			Action:         action,
			RepoNamespace:  repoData.Installation.Account.Login,
			InstallationID: repoData.Installation.ID,
			Repositories:   repos,
			Sender:         repoData.Sender.Login,
			RawPayload:     body,
			ReceivedAt:     time.Now().UTC(),
		}, nil
	}

	if eventType != "pull_request" {
		return &NormalizedWebhookEvent{Action: ActionIgnored}, nil
	}

	var data struct {
		Action      string `json:"action"`
		Number      int    `json:"number"`
		PullRequest struct {
			Number int    `json:"number"`
			Title  string `json:"title"`
			Head   struct {
				SHA string `json:"sha"`
			} `json:"head"`
			Base struct {
				SHA string `json:"sha"`
			} `json:"base"`
			User struct {
				Login string `json:"login"`
			} `json:"user"`
		} `json:"pull_request"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	prNum := data.Number
	if prNum == 0 {
		prNum = data.PullRequest.Number
	}

	action := mapGitHubAction(data.Action)
	return &NormalizedWebhookEvent{
		ID:                uuid.New(),
		Provider:          models.ProviderGitHub,
		Action:            action,
		RepoNamespace:     data.Repository.FullName,
		PullRequestNumber: prNum,
		Title:             data.PullRequest.Title,
		HeadSHA:           data.PullRequest.Head.SHA,
		BaseSHA:           data.PullRequest.Base.SHA,
		Sender:            data.PullRequest.User.Login,
		RawPayload:        body,
		ReceivedAt:        time.Now().UTC(),
	}, nil
}

func (p *WebhookParser) parseGitLab(eventType string, body []byte) (*NormalizedWebhookEvent, error) {
	var data struct {
		ObjectKind string `json:"object_kind"`
		Project    struct {
			PathWithNamespace string `json:"path_with_namespace"`
		} `json:"project"`
		User struct {
			Username string `json:"username"`
		} `json:"user"`
		ObjectAttributes struct {
			IID        int    `json:"iid"`
			Title      string `json:"title"`
			Action     string `json:"action"`
			LastCommit struct {
				ID string `json:"id"`
			} `json:"last_commit"`
			TargetBranch string `json:"target_branch"`
		} `json:"object_attributes"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	if data.ObjectKind != "merge_request" {
		return &NormalizedWebhookEvent{Action: ActionIgnored}, nil
	}

	action := ActionIgnored
	switch data.ObjectAttributes.Action {
	case "open":
		action = ActionOpened
	case "update":
		action = ActionSynchronize
	case "reopen":
		action = ActionReopened
	case "close":
		action = ActionClosed
	}

	return &NormalizedWebhookEvent{
		ID:                uuid.New(),
		Provider:          models.ProviderGitLab,
		Action:            action,
		RepoNamespace:     data.Project.PathWithNamespace,
		PullRequestNumber: data.ObjectAttributes.IID,
		Title:             data.ObjectAttributes.Title,
		HeadSHA:           data.ObjectAttributes.LastCommit.ID,
		Sender:            data.User.Username,
		RawPayload:        body,
		ReceivedAt:        time.Now().UTC(),
	}, nil
}

func (p *WebhookParser) parseBitbucket(eventType string, body []byte) (*NormalizedWebhookEvent, error) {
	var data struct {
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
		Actor struct {
			Username string `json:"username"`
		} `json:"actor"`
		PullRequest struct {
			ID     int    `json:"id"`
			Title  string `json:"title"`
			Source struct {
				Commit struct {
					Hash string `json:"hash"`
				} `json:"commit"`
			} `json:"source"`
			Destination struct {
				Commit struct {
					Hash string `json:"hash"`
				} `json:"commit"`
			} `json:"destination"`
		} `json:"pullrequest"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	action := ActionIgnored
	switch eventType {
	case "pullrequest:created":
		action = ActionOpened
	case "pullrequest:updated":
		action = ActionSynchronize
	}

	return &NormalizedWebhookEvent{
		ID:                uuid.New(),
		Provider:          models.ProviderBitbucket,
		Action:            action,
		RepoNamespace:     data.Repository.FullName,
		PullRequestNumber: data.PullRequest.ID,
		Title:             data.PullRequest.Title,
		HeadSHA:           data.PullRequest.Source.Commit.Hash,
		BaseSHA:           data.PullRequest.Destination.Commit.Hash,
		Sender:            data.Actor.Username,
		RawPayload:        body,
		ReceivedAt:        time.Now().UTC(),
	}, nil
}

func (p *WebhookParser) parseAzureDevOps(eventType string, body []byte) (*NormalizedWebhookEvent, error) {
	var data struct {
		EventType string `json:"eventType"`
		Resource  struct {
			PullRequestID int    `json:"pullRequestId"`
			Title         string `json:"title"`
			Repository    struct {
				Name    string `json:"name"`
				Project struct {
					Name string `json:"name"`
				} `json:"project"`
			} `json:"repository"`
			CreatedBy struct {
				DisplayName string `json:"displayName"`
			} `json:"createdBy"`
			LastMergeSourceCommit struct {
				CommitID string `json:"commitId"`
			} `json:"lastMergeSourceCommit"`
		} `json:"resource"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	action := ActionIgnored
	if data.EventType == "git.pullrequest.created" {
		action = ActionOpened
	} else if data.EventType == "git.pullrequest.updated" {
		action = ActionSynchronize
	}

	repo := fmt.Sprintf("%s/%s", data.Resource.Repository.Project.Name, data.Resource.Repository.Name)

	return &NormalizedWebhookEvent{
		ID:                uuid.New(),
		Provider:          models.ProviderAzure,
		Action:            action,
		RepoNamespace:     repo,
		PullRequestNumber: data.Resource.PullRequestID,
		Title:             data.Resource.Title,
		HeadSHA:           data.Resource.LastMergeSourceCommit.CommitID,
		Sender:            data.Resource.CreatedBy.DisplayName,
		RawPayload:        body,
		ReceivedAt:        time.Now().UTC(),
	}, nil
}

func (p *WebhookParser) parseForgejo(eventType string, body []byte) (*NormalizedWebhookEvent, error) {
	return p.parseGitHub(eventType, body)
}

func mapGitHubAction(action string) WebhookAction {
	switch action {
	case "opened":
		return ActionOpened
	case "synchronize":
		return ActionSynchronize
	case "reopened":
		return ActionReopened
	case "closed":
		return ActionClosed
	default:
		return ActionIgnored
	}
}

// ExtractRepoNamespace scans body for common repo name or installation account keys before full parsing.
func ExtractRepoNamespace(body []byte) (string, error) {
	var quickCheck struct {
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
		Project struct {
			PathWithNamespace string `json:"path_with_namespace"`
		} `json:"project"`
		Installation struct {
			Account struct {
				Login string `json:"login"`
			} `json:"account"`
		} `json:"installation"`
	}

	if err := json.Unmarshal(body, &quickCheck); err != nil {
		return "", err
	}

	if quickCheck.Repository.FullName != "" {
		return quickCheck.Repository.FullName, nil
	}
	if quickCheck.Project.PathWithNamespace != "" {
		return quickCheck.Project.PathWithNamespace, nil
	}
	if quickCheck.Installation.Account.Login != "" {
		return quickCheck.Installation.Account.Login, nil
	}

	return "", errors.New("could not identify repository or installation namespace")
}
