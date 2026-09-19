// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package webhooks

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/platform/domain/contracts"
	"github.com/scandrix/backend/pkg/models"
)

// ExtractRepoFullName attempts to find the "owner/repo" full name from any PR object.
func ExtractRepoFullName(raw map[string]any) string {
	if raw == nil {
		return ""
	}

	if repo, ok := raw["repository"].(string); ok && strings.Contains(repo, "/") {
		return repo
	}

	if repoMap, ok := raw["repository"].(map[string]any); ok {
		if fn, ok := repoMap["full_name"].(string); ok && fn != "" {
			return fn
		}
		if fn, ok := repoMap["fullName"].(string); ok && fn != "" {
			return fn
		}
		if pwn, ok := repoMap["path_with_namespace"].(string); ok && pwn != "" {
			return pwn
		}
	}

	// Try base.repo
	if base, ok := raw["base"].(map[string]any); ok {
		if baseRepo, ok := base["repo"].(map[string]any); ok {
			if fn, ok := baseRepo["full_name"].(string); ok && fn != "" {
				return fn
			}
			if fn, ok := baseRepo["fullName"].(string); ok && fn != "" {
				return fn
			}
		}
	}

	// Try target.path_with_namespace (GitLab)
	if target, ok := raw["target"].(map[string]any); ok {
		if pwn, ok := target["path_with_namespace"].(string); ok && pwn != "" {
			return pwn
		}
	}

	// Try destination.repository.full_name (Bitbucket)
	if dest, ok := raw["destination"].(map[string]any); ok {
		if destRepo, ok := dest["repository"].(map[string]any); ok {
			if fn, ok := destRepo["full_name"].(string); ok && fn != "" {
				return fn
			}
			if fn, ok := destRepo["fullName"].(string); ok && fn != "" {
				return fn
			}
		}
	}

	return ""
}

// GetMappedPlatform returns the IMappedPlatform implementation for a given provider.
func GetMappedPlatform(provider models.SCMProvider) contracts.IMappedPlatform {
	switch provider {
	case models.SCMProviderGitHub:
		return &GitHubMappedPlatform{}
	case models.SCMProviderGitLab:
		return &GitLabMappedPlatform{}
	case models.SCMProviderBitbucket:
		return &BitbucketMappedPlatform{}
	case models.SCMProviderAzureDevOps:
		return &AzureReposMappedPlatform{}
	case models.SCMProviderForgejo:
		return &ForgejoMappedPlatform{}
	default:
		return nil
	}
}

// --- GitHub Mapped Platform ---

type GitHubMappedPlatform struct{}

func (g *GitHubMappedPlatform) MapPullRequest(payload any) *contracts.IMappedPullRequest {
	data, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	prMap, ok := data["pull_request"].(map[string]any)
	if !ok {
		return nil
	}

	title, _ := prMap["title"].(string)
	body, _ := prMap["body"].(string)
	number := intFromAny(prMap["number"])
	isDraft, _ := prMap["draft"].(bool)
	url, _ := prMap["html_url"].(string)

	headRef, headSha, headFullName := extractBranchInfo(prMap["head"])
	baseRef, _, baseFullName := extractBranchInfo(prMap["base"])

	defaultBranch := ""
	if repoMap, ok := data["repository"].(map[string]any); ok {
		defaultBranch, _ = repoMap["default_branch"].(string)
	}

	return &contracts.IMappedPullRequest{
		Repository: data["repository"],
		Title:      title,
		Body:       body,
		Number:     number,
		User:       prMap["user"],
		IsDraft:    isDraft,
		URL:        url,
		Head: contracts.IMappedBranchRef{
			Ref: headRef,
			SHA: headSha,
			Repo: struct {
				FullName      string `json:"fullName"`
				DefaultBranch string `json:"defaultBranch,omitempty"`
			}{
				FullName: headFullName,
			},
		},
		Base: contracts.IMappedBranchRef{
			Ref: baseRef,
			Repo: struct {
				FullName      string `json:"fullName"`
				DefaultBranch string `json:"defaultBranch,omitempty"`
			}{
				FullName:      baseFullName,
				DefaultBranch: defaultBranch,
			},
		},
	}
}

func (g *GitHubMappedPlatform) MapUsers(payload any) *contracts.IMappedUsers {
	data, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	prMap, ok := data["pull_request"].(map[string]any)
	if !ok {
		return nil
	}
	return &contracts.IMappedUsers{
		User:      prMap["user"],
		Assignees: prMap["assignees"],
		Reviewers: prMap["requested_reviewers"],
	}
}

func (g *GitHubMappedPlatform) MapRepository(payload any) *contracts.IMappedRepository {
	data, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	repoMap, ok := data["repository"].(map[string]any)
	if !ok {
		return nil
	}
	id := fmt.Sprintf("%v", repoMap["id"])
	name, _ := repoMap["name"].(string)
	fullName, _ := repoMap["full_name"].(string)
	url, _ := repoMap["html_url"].(string)
	lang, _ := repoMap["language"].(string)

	return &contracts.IMappedRepository{
		ID:       id,
		Name:     name,
		FullName: fullName,
		URL:      url,
		Language: lang,
	}
}

func (g *GitHubMappedPlatform) MapComment(payload any) *contracts.IMappedComment {
	data, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	commentMap, ok := data["comment"].(map[string]any)
	if !ok {
		return nil
	}
	id := fmt.Sprintf("%v", commentMap["id"])
	body, _ := commentMap["body"].(string)
	return &contracts.IMappedComment{
		ID:   id,
		Body: body,
	}
}

func (g *GitHubMappedPlatform) MapAction(payload any, event string) string {
	data, ok := payload.(map[string]any)
	if !ok {
		return ""
	}
	action, _ := data["action"].(string)
	return action
}

// --- GitLab Mapped Platform ---

type GitLabMappedPlatform struct{}

func (gl *GitLabMappedPlatform) MapPullRequest(payload any) *contracts.IMappedPullRequest {
	data, ok := payload.(map[string]any)
	if !ok {
		return nil
	}

	var mrMap map[string]any
	if data["event_type"] == "note" {
		mrMap, _ = data["merge_request"].(map[string]any)
	} else {
		mrMap, _ = data["object_attributes"].(map[string]any)
	}
	if mrMap == nil {
		return nil
	}

	title, _ := mrMap["title"].(string)
	body, _ := mrMap["description"].(string)
	number := intFromAny(mrMap["iid"])
	url, _ := mrMap["url"].(string)
	isDraft := strings.HasPrefix(strings.ToLower(title), "draft:") || strings.HasPrefix(strings.ToLower(title), "wip:")

	sourceBranch, _ := mrMap["source_branch"].(string)
	targetBranch, _ := mrMap["target_branch"].(string)

	headPath := ""
	if src, ok := mrMap["source"].(map[string]any); ok {
		headPath, _ = src["path_with_namespace"].(string)
	}
	targetPath := ""
	targetDefaultBranch := ""
	if tgt, ok := mrMap["target"].(map[string]any); ok {
		targetPath, _ = tgt["path_with_namespace"].(string)
		targetDefaultBranch, _ = tgt["default_branch"].(string)
	}

	var lastCommitSHA string
	if lastCommit, ok := mrMap["last_commit"].(map[string]any); ok {
		lastCommitSHA, _ = lastCommit["id"].(string)
	}

	return &contracts.IMappedPullRequest{
		Repository: data["repository"],
		Title:      title,
		Body:       body,
		Number:     number,
		User:       data["user"],
		IsDraft:    isDraft,
		URL:        url,
		Head: contracts.IMappedBranchRef{
			Ref: sourceBranch,
			SHA: lastCommitSHA,
			Repo: struct {
				FullName      string `json:"fullName"`
				DefaultBranch string `json:"defaultBranch,omitempty"`
			}{
				FullName: headPath,
			},
		},
		Base: contracts.IMappedBranchRef{
			Ref: targetBranch,
			Repo: struct {
				FullName      string `json:"fullName"`
				DefaultBranch string `json:"defaultBranch,omitempty"`
			}{
				FullName:      targetPath,
				DefaultBranch: targetDefaultBranch,
			},
		},
	}
}

func (gl *GitLabMappedPlatform) MapUsers(payload any) *contracts.IMappedUsers {
	data, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	return &contracts.IMappedUsers{
		User:      data["user"],
		Assignees: data["assignees"],
		Reviewers: data["reviewers"],
	}
}

func (gl *GitLabMappedPlatform) MapRepository(payload any) *contracts.IMappedRepository {
	data, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	project, ok := data["project"].(map[string]any)
	if !ok {
		return nil
	}
	return &contracts.IMappedRepository{
		ID:       fmt.Sprintf("%v", project["id"]),
		Name:     fmt.Sprintf("%v", project["name"]),
		FullName: fmt.Sprintf("%v", project["path_with_namespace"]),
		URL:      fmt.Sprintf("%v", project["web_url"]),
	}
}

func (gl *GitLabMappedPlatform) MapComment(payload any) *contracts.IMappedComment {
	data, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	objAttr, ok := data["object_attributes"].(map[string]any)
	if !ok {
		return nil
	}
	return &contracts.IMappedComment{
		ID:   fmt.Sprintf("%v", objAttr["id"]),
		Body: fmt.Sprintf("%v", objAttr["note"]),
	}
}

func (gl *GitLabMappedPlatform) MapAction(payload any, event string) string {
	data, ok := payload.(map[string]any)
	if !ok {
		return ""
	}
	if objAttr, ok := data["object_attributes"].(map[string]any); ok {
		action, _ := objAttr["action"].(string)
		return action
	}
	return ""
}

// --- Bitbucket Mapped Platform ---

type BitbucketMappedPlatform struct{}

func (b *BitbucketMappedPlatform) MapPullRequest(payload any) *contracts.IMappedPullRequest {
	data, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	prMap, ok := data["pullrequest"].(map[string]any)
	if !ok {
		return nil
	}

	title, _ := prMap["title"].(string)
	body, _ := prMap["description"].(string)
	number := intFromAny(prMap["id"])
	isDraft, _ := prMap["draft"].(bool)

	isDataCenter, _ := data["isDataCenterEvent"].(bool)
	if isDataCenter {
		fromRef, _ := prMap["fromRef"].(map[string]any)
		toRef, _ := prMap["toRef"].(map[string]any)

		fromBranch, _ := fromRef["displayId"].(string)
		fromSHA, _ := fromRef["latestCommit"].(string)
		toBranch, _ := toRef["displayId"].(string)

		return &contracts.IMappedPullRequest{
			Repository: prMap,
			Title:      title,
			Body:       body,
			Number:     number,
			User:       prMap["author"],
			IsDraft:    isDraft,
			Head: contracts.IMappedBranchRef{
				Ref: fromBranch,
				SHA: fromSHA,
			},
			Base: contracts.IMappedBranchRef{
				Ref: toBranch,
			},
		}
	}

	// Bitbucket Cloud
	source, _ := prMap["source"].(map[string]any)
	dest, _ := prMap["destination"].(map[string]any)

	sourceBranch := ""
	sourceSHA := ""
	sourceRepoName := ""
	if source != nil {
		if br, ok := source["branch"].(map[string]any); ok {
			sourceBranch, _ = br["name"].(string)
		}
		if commit, ok := source["commit"].(map[string]any); ok {
			sourceSHA, _ = commit["hash"].(string)
		}
		if repo, ok := source["repository"].(map[string]any); ok {
			sourceRepoName, _ = repo["full_name"].(string)
		}
	}

	destBranch := ""
	destRepoName := ""
	if dest != nil {
		if br, ok := dest["branch"].(map[string]any); ok {
			destBranch, _ = br["name"].(string)
		}
		if repo, ok := dest["repository"].(map[string]any); ok {
			destRepoName, _ = repo["full_name"].(string)
		}
	}

	url := ""
	if links, ok := prMap["links"].(map[string]any); ok {
		if html, ok := links["html"].(map[string]any); ok {
			url, _ = html["href"].(string)
		}
	}

	return &contracts.IMappedPullRequest{
		Repository: data["repository"],
		Title:      title,
		Body:       body,
		Number:     number,
		User:       prMap["author"],
		IsDraft:    isDraft,
		URL:        url,
		Head: contracts.IMappedBranchRef{
			Ref: sourceBranch,
			SHA: sourceSHA,
			Repo: struct {
				FullName      string `json:"fullName"`
				DefaultBranch string `json:"defaultBranch,omitempty"`
			}{
				FullName: sourceRepoName,
			},
		},
		Base: contracts.IMappedBranchRef{
			Ref: destBranch,
			Repo: struct {
				FullName      string `json:"fullName"`
				DefaultBranch string `json:"defaultBranch,omitempty"`
			}{
				FullName:      destRepoName,
				DefaultBranch: destBranch,
			},
		},
	}
}

func (b *BitbucketMappedPlatform) MapUsers(payload any) *contracts.IMappedUsers {
	data, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	prMap, ok := data["pullrequest"].(map[string]any)
	if !ok {
		return nil
	}
	return &contracts.IMappedUsers{
		User:      prMap["author"],
		Assignees: prMap["participants"],
		Reviewers: prMap["reviewers"],
	}
}

func (b *BitbucketMappedPlatform) MapRepository(payload any) *contracts.IMappedRepository {
	data, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	repoMap, ok := data["repository"].(map[string]any)
	if !ok {
		return nil
	}
	uuid := StripCurlyBracesFromUUID(fmt.Sprintf("%v", repoMap["uuid"]))
	return &contracts.IMappedRepository{
		ID:       uuid,
		Name:     fmt.Sprintf("%v", repoMap["name"]),
		FullName: fmt.Sprintf("%v", repoMap["full_name"]),
	}
}

func (b *BitbucketMappedPlatform) MapComment(payload any) *contracts.IMappedComment {
	data, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	commentMap, ok := data["comment"].(map[string]any)
	if !ok {
		return nil
	}
	id := fmt.Sprintf("%v", commentMap["id"])
	rawContent := ""
	if content, ok := commentMap["content"].(map[string]any); ok {
		rawContent, _ = content["raw"].(string)
	} else if text, ok := commentMap["text"].(string); ok {
		rawContent = text // Server / Data Center
	}
	return &contracts.IMappedComment{
		ID:   id,
		Body: rawContent,
	}
}

func (b *BitbucketMappedPlatform) MapAction(payload any, event string) string {
	return event
}

// --- Azure Repos Mapped Platform ---

type AzureReposMappedPlatform struct{}

func (a *AzureReposMappedPlatform) MapPullRequest(payload any) *contracts.IMappedPullRequest {
	data, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	res, ok := data["resource"].(map[string]any)
	if !ok {
		return nil
	}
	prMap, ok := res["pullRequest"].(map[string]any)
	if !ok {
		prMap = res
	}

	number := intFromAny(prMap["pullRequestId"])
	if number == 0 {
		return nil
	}

	title, _ := prMap["title"].(string)
	body, _ := prMap["description"].(string)
	url, _ := prMap["url"].(string)
	isDraft, _ := res["isDraft"].(bool)
	sourceRef, _ := prMap["sourceRefName"].(string)
	targetRef, _ := prMap["targetRefName"].(string)

	repoName := ""
	defaultBranch := ""
	if repo, ok := prMap["repository"].(map[string]any); ok {
		repoName, _ = repo["name"].(string)
		defaultBranch, _ = repo["defaultBranch"].(string)
	}

	return &contracts.IMappedPullRequest{
		Repository: prMap["repository"],
		Title:      title,
		Body:       body,
		Number:     number,
		User:       prMap["createdBy"],
		IsDraft:    isDraft,
		URL:        url,
		Head: contracts.IMappedBranchRef{
			Ref: sourceRef,
			Repo: struct {
				FullName      string `json:"fullName"`
				DefaultBranch string `json:"defaultBranch,omitempty"`
			}{
				FullName: repoName,
			},
		},
		Base: contracts.IMappedBranchRef{
			Ref: targetRef,
			Repo: struct {
				FullName      string `json:"fullName"`
				DefaultBranch string `json:"defaultBranch,omitempty"`
			}{
				FullName:      repoName,
				DefaultBranch: defaultBranch,
			},
		},
	}
}

func (a *AzureReposMappedPlatform) MapUsers(payload any) *contracts.IMappedUsers {
	data, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	res, ok := data["resource"].(map[string]any)
	if !ok {
		return nil
	}
	prMap, ok := res["pullRequest"].(map[string]any)
	if !ok {
		prMap = res
	}
	return &contracts.IMappedUsers{
		User:      prMap["createdBy"],
		Reviewers: prMap["reviewers"],
	}
}

func (a *AzureReposMappedPlatform) MapRepository(payload any) *contracts.IMappedRepository {
	data, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	res, ok := data["resource"].(map[string]any)
	if !ok {
		return nil
	}
	repo, ok := res["repository"].(map[string]any)
	if !ok {
		if pr, ok := res["pullRequest"].(map[string]any); ok {
			repo, _ = pr["repository"].(map[string]any)
		}
	}
	if repo == nil {
		return nil
	}
	return &contracts.IMappedRepository{
		ID:       fmt.Sprintf("%v", repo["id"]),
		Name:     fmt.Sprintf("%v", repo["name"]),
		FullName: fmt.Sprintf("%v", repo["name"]),
		URL:      fmt.Sprintf("%v", repo["remoteUrl"]),
	}
}

func (a *AzureReposMappedPlatform) MapComment(payload any) *contracts.IMappedComment {
	data, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	res, ok := data["resource"].(map[string]any)
	if !ok {
		return nil
	}
	comment, ok := res["comment"].(map[string]any)
	if !ok {
		return nil
	}
	return &contracts.IMappedComment{
		ID:   fmt.Sprintf("%v", comment["id"]),
		Body: fmt.Sprintf("%v", comment["content"]),
	}
}

func (a *AzureReposMappedPlatform) MapAction(payload any, event string) string {
	return event
}

// --- Forgejo Mapped Platform ---

type ForgejoMappedPlatform struct{}

func (f *ForgejoMappedPlatform) MapPullRequest(payload any) *contracts.IMappedPullRequest {
	data, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	prMap, ok := data["pull_request"].(map[string]any)
	if !ok {
		return nil
	}

	title, _ := prMap["title"].(string)
	body, _ := prMap["body"].(string)
	number := intFromAny(prMap["number"])
	isDraft, _ := prMap["draft"].(bool)
	url, _ := prMap["html_url"].(string)

	headRef, headSha, headFullName := extractBranchInfo(prMap["head"])
	baseRef, _, baseFullName := extractBranchInfo(prMap["base"])

	defaultBranch := ""
	if repoMap, ok := data["repository"].(map[string]any); ok {
		defaultBranch, _ = repoMap["default_branch"].(string)
	}

	return &contracts.IMappedPullRequest{
		Repository: data["repository"],
		Title:      title,
		Body:       body,
		Number:     number,
		User:       prMap["user"],
		IsDraft:    isDraft,
		URL:        url,
		Head: contracts.IMappedBranchRef{
			Ref: headRef,
			SHA: headSha,
			Repo: struct {
				FullName      string `json:"fullName"`
				DefaultBranch string `json:"defaultBranch,omitempty"`
			}{
				FullName: headFullName,
			},
		},
		Base: contracts.IMappedBranchRef{
			Ref: baseRef,
			Repo: struct {
				FullName      string `json:"fullName"`
				DefaultBranch string `json:"defaultBranch,omitempty"`
			}{
				FullName:      baseFullName,
				DefaultBranch: defaultBranch,
			},
		},
	}
}

func (f *ForgejoMappedPlatform) MapUsers(payload any) *contracts.IMappedUsers {
	data, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	prMap, ok := data["pull_request"].(map[string]any)
	if !ok {
		return nil
	}
	return &contracts.IMappedUsers{
		User:      prMap["user"],
		Assignees: prMap["assignees"],
		Reviewers: prMap["requested_reviewers"],
	}
}

func (f *ForgejoMappedPlatform) MapRepository(payload any) *contracts.IMappedRepository {
	data, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	repoMap, ok := data["repository"].(map[string]any)
	if !ok {
		return nil
	}
	return &contracts.IMappedRepository{
		ID:       fmt.Sprintf("%v", repoMap["id"]),
		Name:     fmt.Sprintf("%v", repoMap["name"]),
		FullName: fmt.Sprintf("%v", repoMap["full_name"]),
		URL:      fmt.Sprintf("%v", repoMap["html_url"]),
		Language: fmt.Sprintf("%v", repoMap["language"]),
	}
}

func (f *ForgejoMappedPlatform) MapComment(payload any) *contracts.IMappedComment {
	data, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	commentMap, ok := data["comment"].(map[string]any)
	if !ok {
		return nil
	}
	return &contracts.IMappedComment{
		ID:   fmt.Sprintf("%v", commentMap["id"]),
		Body: fmt.Sprintf("%v", commentMap["body"]),
	}
}

func (f *ForgejoMappedPlatform) MapAction(payload any, event string) string {
	data, ok := payload.(map[string]any)
	if !ok {
		return ""
	}
	action, _ := data["action"].(string)
	return action
}

// Helpers

func intFromAny(v any) int {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

func extractBranchInfo(branchAny any) (ref, sha, repoFullName string) {
	branchMap, ok := branchAny.(map[string]any)
	if !ok {
		return "", "", ""
	}
	ref, _ = branchMap["ref"].(string)
	sha, _ = branchMap["sha"].(string)
	if repo, ok := branchMap["repo"].(map[string]any); ok {
		repoFullName, _ = repo["full_name"].(string)
		if repoFullName == "" {
			repoFullName, _ = repo["fullName"].(string)
		}
	}
	return ref, sha, repoFullName
}
