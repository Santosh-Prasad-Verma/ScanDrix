package webhooks

import (
	"fmt"
	"strconv"
	"strings"
)

// ForgejoMapper handles Forgejo and Gitea webhook payloads.
type ForgejoMapper struct{}

func (m *ForgejoMapper) MapRepository(payload map[string]any) *MappedRepository {
	repoRaw, ok := payload["repository"].(map[string]any)
	if !ok {
		return nil
	}

	id := fmt.Sprintf("%v", repoRaw["id"])
	name, _ := repoRaw["name"].(string)
	fullName, _ := repoRaw["full_name"].(string)
	url, _ := repoRaw["html_url"].(string)
	if url == "" {
		url, _ = repoRaw["url"].(string)
	}
	defBranch, _ := repoRaw["default_branch"].(string)
	isPrivate, _ := repoRaw["private"].(bool)

	return &MappedRepository{
		ID:            id,
		Name:          name,
		FullName:      fullName,
		URL:           url,
		DefaultBranch: defBranch,
		IsPrivate:     isPrivate,
	}
}

func (m *ForgejoMapper) MapPullRequest(payload map[string]any) *MappedPullRequest {
	prRaw, ok := payload["pull_request"].(map[string]any)
	if !ok {
		return nil
	}

	var num int
	switch v := prRaw["number"].(type) {
	case float64:
		num = int(v)
	case int:
		num = v
	case string:
		num, _ = strconv.Atoi(v)
	}

	title, _ := prRaw["title"].(string)
	body, _ := prRaw["body"].(string)
	url, _ := prRaw["html_url"].(string)
	if url == "" {
		url, _ = prRaw["url"].(string)
	}
	state, _ := prRaw["state"].(string)
	isDraft, _ := prRaw["draft"].(bool)

	var user *MappedUser
	if u, ok := prRaw["user"].(map[string]any); ok {
		user = &MappedUser{
			ID:        fmt.Sprintf("%v", u["id"]),
			Login:     fmt.Sprintf("%v", u["login"]),
			Name:      fmt.Sprintf("%v", u["full_name"]),
			AvatarURL: fmt.Sprintf("%v", u["avatar_url"]),
		}
	}

	headRaw, _ := prRaw["head"].(map[string]any)
	baseRaw, _ := prRaw["base"].(map[string]any)

	headRef, _ := headRaw["ref"].(string)
	headSha, _ := headRaw["sha"].(string)
	baseRef, _ := baseRaw["ref"].(string)
	baseSha, _ := baseRaw["sha"].(string)

	var headRepoFullName, baseRepoFullName string
	if headRepo, ok := headRaw["repo"].(map[string]any); ok {
		headRepoFullName, _ = headRepo["full_name"].(string)
	}
	if baseRepo, ok := baseRaw["repo"].(map[string]any); ok {
		baseRepoFullName, _ = baseRepo["full_name"].(string)
	}

	var labels []string
	if labelsRaw, ok := prRaw["labels"].([]any); ok {
		for _, l := range labelsRaw {
			if lMap, ok := l.(map[string]any); ok {
				if lName, ok := lMap["name"].(string); ok {
					labels = append(labels, lName)
				}
			}
		}
	}

	return &MappedPullRequest{
		ID:      fmt.Sprintf("%v", prRaw["id"]),
		Number:  num,
		Title:   title,
		Body:    body,
		URL:     url,
		State:   state,
		IsDraft: isDraft || IsDraftTitle(title),
		Head: MappedCommitRef{
			RepoFullName: headRepoFullName,
			Ref:          headRef,
			SHA:          headSha,
		},
		Base: MappedCommitRef{
			RepoFullName: baseRepoFullName,
			Ref:          baseRef,
			SHA:          baseSha,
		},
		User:       user,
		Labels:     labels,
		Repository: m.MapRepository(payload),
	}
}

func (m *ForgejoMapper) MapComment(payload map[string]any) *MappedComment {
	commentRaw, ok := payload["comment"].(map[string]any)
	if !ok {
		return nil
	}

	id := fmt.Sprintf("%v", commentRaw["id"])
	body, _ := commentRaw["body"].(string)
	url, _ := commentRaw["html_url"].(string)

	var user *MappedUser
	if u, ok := commentRaw["user"].(map[string]any); ok {
		user = &MappedUser{
			ID:        fmt.Sprintf("%v", u["id"]),
			Login:     fmt.Sprintf("%v", u["login"]),
			Name:      fmt.Sprintf("%v", u["full_name"]),
			AvatarURL: fmt.Sprintf("%v", u["avatar_url"]),
		}
	}

	return &MappedComment{
		ID:   id,
		Body: body,
		URL:  url,
		User: user,
	}
}

func (m *ForgejoMapper) MapAction(action string) MappedAction {
	switch strings.ToLower(action) {
	case "opened":
		return ActionOpened
	case "synchronized", "edited":
		return ActionUpdated
	case "closed":
		return ActionClosed
	case "reopened":
		return ActionReopened
	case "created":
		return ActionCommentCreated
	default:
		return ActionUnknown
	}
}
