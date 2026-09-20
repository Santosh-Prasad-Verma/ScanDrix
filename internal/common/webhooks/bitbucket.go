package webhooks

import (
	"fmt"
	"strconv"
	"strings"
)

// BitbucketMapper handles Bitbucket Cloud and Data Center webhook payloads.
type BitbucketMapper struct{}

func (m *BitbucketMapper) MapRepository(payload map[string]any) *MappedRepository {
	repoRaw, ok := payload["repository"].(map[string]any)
	if !ok {
		// Fallback for Bitbucket Data Center
		if pr, prOk := payload["pullrequest"].(map[string]any); prOk {
			if toRef, refOk := pr["toRef"].(map[string]any); refOk {
				repoRaw, _ = toRef["repository"].(map[string]any)
			}
		}
		if repoRaw == nil {
			return nil
		}
	}

	id := fmt.Sprintf("%v", repoRaw["uuid"])
	if id == "<nil>" || id == "" {
		id = fmt.Sprintf("%v", repoRaw["id"])
	}

	name, _ := repoRaw["name"].(string)
	fullName, _ := repoRaw["full_name"].(string)
	if fullName == "" {
		fullName = name
	}

	var url string
	if links, ok := repoRaw["links"].(map[string]any); ok {
		if html, ok := links["html"].(map[string]any); ok {
			url, _ = html["href"].(string)
		}
	}

	isPrivate, _ := repoRaw["is_private"].(bool)
	if !isPrivate {
		if pub, ok := repoRaw["public"].(bool); ok {
			isPrivate = !pub
		}
	}

	return &MappedRepository{
		ID:        id,
		Name:      name,
		FullName:  fullName,
		URL:       url,
		IsPrivate: isPrivate,
	}
}

func (m *BitbucketMapper) MapPullRequest(payload map[string]any) *MappedPullRequest {
	prRaw, ok := payload["pullrequest"].(map[string]any)
	if !ok {
		return nil
	}

	var num int
	switch v := prRaw["id"].(type) {
	case float64:
		num = int(v)
	case int:
		num = v
	case string:
		num, _ = strconv.Atoi(v)
	}

	title, _ := prRaw["title"].(string)
	desc, _ := prRaw["description"].(string)
	state, _ := prRaw["state"].(string)
	isDraft, _ := prRaw["draft"].(bool)

	var url string
	if links, ok := prRaw["links"].(map[string]any); ok {
		if html, ok := links["html"].(map[string]any); ok {
			url, _ = html["href"].(string)
		}
	}

	var user *MappedUser
	if author, ok := prRaw["author"].(map[string]any); ok {
		user = &MappedUser{
			ID:    fmt.Sprintf("%v", author["uuid"]),
			Login: fmt.Sprintf("%v", author["nickname"]),
			Name:  fmt.Sprintf("%v", author["display_name"]),
		}
	}

	var headRef, headSha, headRepoFullName string
	var baseRef, baseSha, baseRepoFullName string

	// Cloud structure
	if src, ok := prRaw["source"].(map[string]any); ok {
		if br, ok := src["branch"].(map[string]any); ok {
			headRef, _ = br["name"].(string)
		}
		if c, ok := src["commit"].(map[string]any); ok {
			headSha, _ = c["hash"].(string)
		}
		if r, ok := src["repository"].(map[string]any); ok {
			headRepoFullName, _ = r["full_name"].(string)
		}
	}
	if dst, ok := prRaw["destination"].(map[string]any); ok {
		if br, ok := dst["branch"].(map[string]any); ok {
			baseRef, _ = br["name"].(string)
		}
		if c, ok := dst["commit"].(map[string]any); ok {
			baseSha, _ = c["hash"].(string)
		}
		if r, ok := dst["repository"].(map[string]any); ok {
			baseRepoFullName, _ = r["full_name"].(string)
		}
	}

	// Data Center structure fallback
	if headRef == "" {
		if fromRef, ok := prRaw["fromRef"].(map[string]any); ok {
			headRef, _ = fromRef["displayId"].(string)
			headSha, _ = fromRef["latestCommit"].(string)
			if repo, ok := fromRef["repository"].(map[string]any); ok {
				headRepoFullName, _ = repo["name"].(string)
			}
		}
	}
	if baseRef == "" {
		if toRef, ok := prRaw["toRef"].(map[string]any); ok {
			baseRef, _ = toRef["displayId"].(string)
			baseSha, _ = toRef["latestCommit"].(string)
			if repo, ok := toRef["repository"].(map[string]any); ok {
				baseRepoFullName, _ = repo["name"].(string)
			}
		}
	}

	return &MappedPullRequest{
		ID:      strconv.Itoa(num),
		Number:  num,
		Title:   title,
		Body:    desc,
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
		Repository: m.MapRepository(payload),
	}
}

func (m *BitbucketMapper) MapComment(payload map[string]any) *MappedComment {
	commentRaw, ok := payload["comment"].(map[string]any)
	if !ok {
		return nil
	}

	id := fmt.Sprintf("%v", commentRaw["id"])
	var body string
	if content, ok := commentRaw["content"].(map[string]any); ok {
		body, _ = content["raw"].(string)
	}

	var user *MappedUser
	if u, ok := commentRaw["user"].(map[string]any); ok {
		user = &MappedUser{
			ID:    fmt.Sprintf("%v", u["uuid"]),
			Login: fmt.Sprintf("%v", u["nickname"]),
			Name:  fmt.Sprintf("%v", u["display_name"]),
		}
	}

	return &MappedComment{
		ID:   id,
		Body: body,
		User: user,
	}
}

func (m *BitbucketMapper) MapAction(eventKey string) MappedAction {
	switch strings.ToLower(eventKey) {
	case "pullrequest:created", "pr:opened":
		return ActionOpened
	case "pullrequest:updated", "pr:from_ref_updated", "pr:modified":
		return ActionUpdated
	case "pullrequest:fulfilled", "pr:merged":
		return ActionMerged
	case "pullrequest:rejected", "pr:deleted":
		return ActionClosed
	case "pullrequest:comment_created", "pr:comment:added":
		return ActionCommentCreated
	default:
		return ActionUnknown
	}
}
