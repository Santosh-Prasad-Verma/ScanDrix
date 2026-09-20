// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/scandrix/backend/internal/platform/domain/types"
)

// OrganizationMember represents a member of a GitHub organization.
type OrganizationMember struct {
	ID        int64           `json:"id"`
	Login     string          `json:"login"`
	AvatarURL string          `json:"avatar_url"`
	Type      string          `json:"type"`
	SiteAdmin bool            `json:"site_admin"`
	Role      string          `json:"role,omitempty"`
	Suspended bool            `json:"suspended,omitempty"`
	Perms     map[string]bool `json:"permissions,omitempty"`
}

// MembersAndAuthorsService handles organization membership and pull request author aggregation.
type MembersAndAuthorsService struct {
	client *Client
}

// NewMembersAndAuthorsService creates a new MembersAndAuthorsService.
func NewMembersAndAuthorsService(client *Client) *MembersAndAuthorsService {
	return &MembersAndAuthorsService{client: client}
}

// GetListMembers lists all members in an organization, optionally filtered by role.
func (s *MembersAndAuthorsService) GetListMembers(ctx context.Context, org string, role string, page, perPage int) ([]OrganizationMember, error) {
	if org == "" {
		return nil, fmt.Errorf("organization name is required")
	}
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 30
	}

	endpoint := fmt.Sprintf("/orgs/%s/members?page=%d&per_page=%d", url.PathEscape(org), page, perPage)
	if role != "" {
		endpoint += fmt.Sprintf("&role=%s", url.QueryEscape(role))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.client.baseURL+endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d for org members", resp.StatusCode)
	}

	var members []OrganizationMember
	if err := json.NewDecoder(resp.Body).Decode(&members); err != nil {
		return nil, err
	}

	return members, nil
}

// FilterMembers filters a list of candidate logins to only those that are active members of the organization.
func (s *MembersAndAuthorsService) FilterMembers(ctx context.Context, org string, logins []string) ([]OrganizationMember, error) {
	if len(logins) == 0 {
		return nil, nil
	}

	active := make([]OrganizationMember, 0, len(logins))
	for _, login := range logins {
		endpoint := fmt.Sprintf("/orgs/%s/members/%s", url.PathEscape(org), url.PathEscape(login))
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.client.baseURL+endpoint, nil)
		if err != nil {
			continue
		}

		resp, err := s.client.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
			active = append(active, OrganizationMember{
				Login: login,
			})
		}
	}

	return active, nil
}

// GetSuspendedStatusBatch checks whether specified organization members are suspended.
func (s *MembersAndAuthorsService) GetSuspendedStatusBatch(ctx context.Context, org string, logins []string) (map[string]bool, error) {
	results := make(map[string]bool, len(logins))
	for _, login := range logins {
		endpoint := fmt.Sprintf("/orgs/%s/memberships/%s", url.PathEscape(org), url.PathEscape(login))
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.client.baseURL+endpoint, nil)
		if err != nil {
			results[login] = false
			continue
		}

		resp, err := s.client.Do(req)
		if err != nil {
			results[login] = false
			continue
		}

		var membership struct {
			State string `json:"state"`
			Role  string `json:"role"`
			User  struct {
				Suspended bool `json:"suspended"`
			} `json:"user"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&membership)
		resp.Body.Close()

		results[login] = membership.User.Suspended
	}

	return results, nil
}

// GetPullRequestAuthors retrieves all unique authors of pull requests for a repository.
func (s *MembersAndAuthorsService) GetPullRequestAuthors(ctx context.Context, owner, repo, state string) ([]string, error) {
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("owner and repo are required")
	}
	if state == "" {
		state = "all"
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/pulls?state=%s&per_page=100",
		url.PathEscape(owner), url.PathEscape(repo), url.QueryEscape(state))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.client.baseURL+endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d for pull requests", resp.StatusCode)
	}

	var prs []struct {
		User struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&prs); err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var authors []string
	for _, pr := range prs {
		login := strings.TrimSpace(pr.User.Login)
		if login != "" {
			if _, exists := seen[login]; !exists {
				seen[login] = struct{}{}
				authors = append(authors, login)
			}
		}
	}

	return authors, nil
}

// SearchPullRequestsByTitle searches pull requests in a repository matching a query string in title or body.
func (s *MembersAndAuthorsService) SearchPullRequestsByTitle(ctx context.Context, owner, repo, query string) ([]types.PullRequest, error) {
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("owner and repo are required")
	}

	q := fmt.Sprintf("repo:%s/%s is:pr %s in:title", owner, repo, query)
	endpoint := fmt.Sprintf("/search/issues?q=%s&sort=updated&order=desc", url.QueryEscape(q))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.client.baseURL+endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub search returned status %d", resp.StatusCode)
	}

	var searchResp struct {
		Items []struct {
			Number    int    `json:"number"`
			Title     string `json:"title"`
			State     string `json:"state"`
			HTMLURL   string `json:"html_url"`
			CreatedAt string `json:"created_at"`
			User      struct {
				Login string `json:"login"`
			} `json:"user"`
			PullRequest struct {
				URL string `json:"url"`
			} `json:"pull_request"`
		} `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, err
	}

	results := make([]types.PullRequest, 0, len(searchResp.Items))
	for _, item := range searchResp.Items {
		results = append(results, types.PullRequest{
			ID:           fmt.Sprintf("%d", item.Number),
			Number:       item.Number,
			Title:        item.Title,
			Author:       item.User.Login,
			SourceBranch: "unknown",
			TargetBranch: "main",
			State:        item.State,
			URL:          item.HTMLURL,
		})
	}

	return results, nil
}
