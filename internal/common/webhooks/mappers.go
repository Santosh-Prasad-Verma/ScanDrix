// Package webhooks implements platform mappers for GitHub, GitLab, Azure, Bitbucket, and Forgejo.
package webhooks

import (
	"fmt"
	"strconv"
	"strings"
)

// GithubMapper handles GitHub webhook payload mapping.
type GithubMapper struct{}

func (m *GithubMapper) MapRepository(payload map[string]any) *MappedRepository {
	repoRaw, ok := payload["repository"].(map[string]any)
	if !ok {
		return nil
	}
	id := fmt.Sprintf("%v", repoRaw["id"])
	name, _ := repoRaw["name"].(string)
	fullName, _ := repoRaw["full_name"].(string)
	url, _ := repoRaw["html_url"].(string)
	lang, _ := repoRaw["language"].(string)
	defBranch, _ := repoRaw["default_branch"].(string)
	isPrivate, _ := repoRaw["private"].(bool)

	return &MappedRepository{
		ID:            id,
		Name:          name,
		FullName:      fullName,
		URL:           url,
		Language:      lang,
		DefaultBranch: defBranch,
		IsPrivate:     isPrivate,
	}
}

func (m *GithubMapper) MapPullRequest(payload map[string]any) *MappedPullRequest {
	prRaw, ok := payload["pull_request"].(map[string]any)
	if !ok {
		return nil
	}

	num, _ := prRaw["number"].(float64)
	title, _ := prRaw["title"].(string)
	body, _ := prRaw["body"].(string)
	url, _ := prRaw["html_url"].(string)
	state, _ := prRaw["state"].(string)
	draft, _ := prRaw["draft"].(bool)

	headRaw, _ := prRaw["head"].(map[string]any)
	baseRaw, _ := prRaw["base"].(map[string]any)

	headSha, _ := headRaw["sha"].(string)
	headRef, _ := headRaw["ref"].(string)
	baseSha, _ := baseRaw["sha"].(string)
	baseRef, _ := baseRaw["ref"].(string)

	var user *MappedUser
	if userRaw, ok := prRaw["user"].(map[string]any); ok {
		user = &MappedUser{
			ID:        fmt.Sprintf("%v", userRaw["id"]),
			Login:     fmt.Sprintf("%v", userRaw["login"]),
			AvatarURL: fmt.Sprintf("%v", userRaw["avatar_url"]),
		}
	}

	var labels []string
	if labelsRaw, ok := prRaw["labels"].([]any); ok {
		for _, l := range labelsRaw {
			if lMap, ok := l.(map[string]any); ok {
				if name, ok := lMap["name"].(string); ok {
					labels = append(labels, name)
				}
			}
		}
	}

	return &MappedPullRequest{
		ID:         fmt.Sprintf("%v", prRaw["id"]),
		Number:     int(num),
		Title:      title,
		Body:       body,
		URL:        url,
		State:      state,
		IsDraft:    draft || IsDraftTitle(title),
		Head: MappedCommitRef{
			Ref: headRef,
			SHA: headSha,
		},
		Base: MappedCommitRef{
			Ref: baseRef,
			SHA: baseSha,
		},
		User:       user,
		Labels:     labels,
		Repository: m.MapRepository(payload),
	}
}

func (m *GithubMapper) MapComment(payload map[string]any) *MappedComment {
	commentRaw, ok := payload["comment"].(map[string]any)
	if !ok {
		return nil
	}
	id := fmt.Sprintf("%v", commentRaw["id"])
	body, _ := commentRaw["body"].(string)
	url, _ := commentRaw["html_url"].(string)
	path, _ := commentRaw["path"].(string)
	line := 0
	if l, ok := commentRaw["line"].(float64); ok {
		line = int(l)
	}

	var user *MappedUser
	if userRaw, ok := commentRaw["user"].(map[string]any); ok {
		user = &MappedUser{
			ID:        fmt.Sprintf("%v", userRaw["id"]),
			Login:     fmt.Sprintf("%v", userRaw["login"]),
			AvatarURL: fmt.Sprintf("%v", userRaw["avatar_url"]),
		}
	}

	return &MappedComment{
		ID:   id,
		Body: body,
		URL:  url,
		Path: path,
		Line: line,
		User: user,
	}
}

func (m *GithubMapper) MapAction(action string) MappedAction {
	switch strings.ToLower(action) {
	case "opened":
		return ActionOpened
	case "synchronize", "reopened":
		return ActionUpdated
	case "closed":
		return ActionClosed
	case "created":
		return ActionCommentCreated
	default:
		return ActionUnknown
	}
}

// GitlabMapper handles GitLab webhook payload mapping.
type GitlabMapper struct{}

func (m *GitlabMapper) MapRepository(payload map[string]any) *MappedRepository {
	projRaw, ok := payload["project"].(map[string]any)
	if !ok {
		return nil
	}
	id := fmt.Sprintf("%v", projRaw["id"])
	name, _ := projRaw["name"].(string)
	fullName, _ := projRaw["path_with_namespace"].(string)
	url, _ := projRaw["web_url"].(string)
	defBranch, _ := projRaw["default_branch"].(string)

	return &MappedRepository{
		ID:            id,
		Name:          name,
		FullName:      fullName,
		URL:           url,
		DefaultBranch: defBranch,
	}
}

func (m *GitlabMapper) MapPullRequest(payload map[string]any) *MappedPullRequest {
	attrs, ok := payload["object_attributes"].(map[string]any)
	if !ok {
		return nil
	}

	iid, _ := attrs["iid"].(float64)
	title, _ := attrs["title"].(string)
	desc, _ := attrs["description"].(string)
	url, _ := attrs["url"].(string)
	state, _ := attrs["state"].(string)
	wip, _ := attrs["work_in_progress"].(bool)
	draft, _ := attrs["draft"].(bool)

	lastCommit, _ := attrs["last_commit"].(map[string]any)
	sha, _ := lastCommit["id"].(string)
	sourceBranch, _ := attrs["source_branch"].(string)
	targetBranch, _ := attrs["target_branch"].(string)

	var user *MappedUser
	if userRaw, ok := payload["user"].(map[string]any); ok {
		user = &MappedUser{
			ID:        fmt.Sprintf("%v", userRaw["id"]),
			Login:     fmt.Sprintf("%v", userRaw["username"]),
			Name:      fmt.Sprintf("%v", userRaw["name"]),
			Email:     fmt.Sprintf("%v", userRaw["email"]),
			AvatarURL: fmt.Sprintf("%v", userRaw["avatar_url"]),
		}
	}

	return &MappedPullRequest{
		ID:      fmt.Sprintf("%v", attrs["id"]),
		Number:  int(iid),
		Title:   title,
		Body:    desc,
		URL:     url,
		State:   state,
		IsDraft: wip || draft || IsDraftTitle(title),
		Head: MappedCommitRef{
			Ref: sourceBranch,
			SHA: sha,
		},
		Base: MappedCommitRef{
			Ref: targetBranch,
		},
		User:       user,
		Repository: m.MapRepository(payload),
	}
}

func (m *GitlabMapper) MapComment(payload map[string]any) *MappedComment {
	attrs, ok := payload["object_attributes"].(map[string]any)
	if !ok {
		return nil
	}
	id := fmt.Sprintf("%v", attrs["id"])
	note, _ := attrs["note"].(string)
	url, _ := attrs["url"].(string)

	var user *MappedUser
	if userRaw, ok := payload["user"].(map[string]any); ok {
		user = &MappedUser{
			ID:        fmt.Sprintf("%v", userRaw["id"]),
			Login:     fmt.Sprintf("%v", userRaw["username"]),
			Name:      fmt.Sprintf("%v", userRaw["name"]),
			Email:     fmt.Sprintf("%v", userRaw["email"]),
			AvatarURL: fmt.Sprintf("%v", userRaw["avatar_url"]),
		}
	}

	return &MappedComment{
		ID:   id,
		Body: note,
		URL:  url,
		User: user,
	}
}

func (m *GitlabMapper) MapAction(action string) MappedAction {
	switch strings.ToLower(action) {
	case "open":
		return ActionOpened
	case "update":
		return ActionUpdated
	case "close":
		return ActionClosed
	case "merge":
		return ActionMerged
	default:
		return ActionUnknown
	}
}

// AzureMapper handles Azure DevOps Repos webhook payloads.
type AzureMapper struct{}

func (m *AzureMapper) MapRepository(payload map[string]any) *MappedRepository {
	res, ok := payload["resource"].(map[string]any)
	if !ok {
		return nil
	}
	repo, ok := res["repository"].(map[string]any)
	if !ok {
		return nil
	}
	id, _ := repo["id"].(string)
	name, _ := repo["name"].(string)
	url, _ := repo["remoteUrl"].(string)

	return &MappedRepository{
		ID:       id,
		Name:     name,
		FullName: name,
		URL:      url,
	}
}

func (m *AzureMapper) MapPullRequest(payload map[string]any) *MappedPullRequest {
	res, ok := payload["resource"].(map[string]any)
	if !ok {
		return nil
	}

	prID, _ := res["pullRequestId"].(float64)
	title, _ := res["title"].(string)
	desc, _ := res["description"].(string)
	status, _ := res["status"].(string)
	sourceRef, _ := res["sourceRefName"].(string)
	targetRef, _ := res["targetRefName"].(string)

	isDraft, _ := res["isDraft"].(bool)

	var user *MappedUser
	if createdBy, ok := res["createdBy"].(map[string]any); ok {
		user = &MappedUser{
			ID:        fmt.Sprintf("%v", createdBy["id"]),
			Login:     fmt.Sprintf("%v", createdBy["uniqueName"]),
			Name:      fmt.Sprintf("%v", createdBy["displayName"]),
			AvatarURL: fmt.Sprintf("%v", createdBy["imageUrl"]),
		}
	}

	return &MappedPullRequest{
		ID:      strconv.Itoa(int(prID)),
		Number:  int(prID),
		Title:   title,
		Body:    desc,
		State:   status,
		IsDraft: isDraft || IsDraftTitle(title),
		Head: MappedCommitRef{
			Ref: strings.TrimPrefix(sourceRef, "refs/heads/"),
		},
		Base: MappedCommitRef{
			Ref: strings.TrimPrefix(targetRef, "refs/heads/"),
		},
		User:       user,
		Repository: m.MapRepository(payload),
	}
}

func (m *AzureMapper) MapComment(payload map[string]any) *MappedComment {
	res, ok := payload["resource"].(map[string]any)
	if !ok {
		return nil
	}
	comment, ok := res["comment"].(map[string]any)
	if !ok {
		return nil
	}
	id := fmt.Sprintf("%v", comment["id"])
	content, _ := comment["content"].(string)

	return &MappedComment{
		ID:   id,
		Body: content,
	}
}

func (m *AzureMapper) MapAction(eventType string) MappedAction {
	switch strings.ToLower(eventType) {
	case "git.pullrequest.created":
		return ActionOpened
	case "git.pullrequest.updated":
		return ActionUpdated
	case "git.pullrequest.merged":
		return ActionMerged
	default:
		return ActionUnknown
	}
}
