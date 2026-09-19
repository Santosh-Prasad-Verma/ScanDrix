// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// GITHUB SEARCH & REPOSITORY STATISTICS MODELS

// CodeSearchResultItem represents a code match in GitHub search.
type CodeSearchResultItem struct {
	Name       string  `json:"name"`
	Path       string  `json:"path"`
	SHA        string  `json:"sha"`
	HTMLURL    string  `json:"html_url"`
	Repository RepoRef `json:"repository"`
}

// RepoRef models a minimal repository reference in search results.
type RepoRef struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
	Private  bool   `json:"private"`
	HTMLURL  string `json:"html_url"`
}

// CodeSearchResponse models search results for code queries.
type CodeSearchResponse struct {
	TotalCount        int                    `json:"total_count"`
	IncompleteResults bool                   `json:"incomplete_results"`
	Items             []CodeSearchResultItem `json:"items"`
}

// CommitActivityWeek models a single week's commit activity from the stats API.
type CommitActivityWeek struct {
	Days  []int `json:"days"`  // Commits per day (Sunday-Saturday)
	Total int   `json:"total"` // Total commits this week
	Week  int64 `json:"week"`  // Start of week timestamp (Unix)
}

// CodeFrequencyWeek models weekly additions and deletions [timestamp, additions, deletions].
type CodeFrequencyWeek struct {
	Timestamp time.Time `json:"timestamp"`
	Additions int       `json:"additions"`
	Deletions int       `json:"deletions"`
}

// RepoTrafficViews models repository view counts and unique visitor statistics.
type RepoTrafficViews struct {
	Count   int `json:"count"`
	Uniques int `json:"uniques"`
	Views   []struct {
		Timestamp time.Time `json:"timestamp"`
		Count     int       `json:"count"`
		Uniques   int       `json:"uniques"`
	} `json:"views"`
}

// RepoTrafficClones models repository clone counts and unique cloners.
type RepoTrafficClones struct {
	Count   int `json:"count"`
	Uniques int `json:"uniques"`
	Clones  []struct {
		Timestamp time.Time `json:"timestamp"`
		Count     int       `json:"count"`
		Uniques   int       `json:"uniques"`
	} `json:"clones"`
}

// PunchCardPoint models [day_of_week, hour_of_day, commit_count].
type PunchCardPoint struct {
	DayOfWeek int `json:"dayOfWeek"` // 0 = Sunday, 6 = Saturday
	HourOfDay int `json:"hourOfDay"` // 0-23
	Commits   int `json:"commits"`
}

// GITHUB SEARCH AND STATS SERVICE

// SearchAndStatsService manages code search, commit activity, and traffic analytics.
type SearchAndStatsService struct {
	client *Client
}

// NewSearchAndStatsService initializes a new SearchAndStatsService.
func NewSearchAndStatsService(client *Client) *SearchAndStatsService {
	return &SearchAndStatsService{client: client}
}

// SearchCode searches for code across repositories with optional qualifiers.
func (s *SearchAndStatsService) SearchCode(
	ctx context.Context,
	query string,
	page, perPage int,
) (*CodeSearchResponse, error) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 30
	}

	q := url.Values{}
	q.Set("q", query)
	q.Set("page", strconv.Itoa(page))
	q.Set("per_page", strconv.Itoa(perPage))

	endpoint := fmt.Sprintf("%s/search/code?%s", s.client.baseURL, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github search code failed with status %d", resp.StatusCode)
	}

	var result CodeSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetCommitActivity retrieves 52-week commit activity summary for a repository.
func (s *SearchAndStatsService) GetCommitActivity(
	ctx context.Context,
	owner, repo string,
) ([]CommitActivityWeek, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/stats/commit_activity", s.client.baseURL, owner, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github get commit activity failed with status %d", resp.StatusCode)
	}

	var activity []CommitActivityWeek
	if err := json.NewDecoder(resp.Body).Decode(&activity); err != nil {
		return nil, err
	}

	return activity, nil
}

// GetCodeFrequency retrieves weekly additions and deletions for a repository.
func (s *SearchAndStatsService) GetCodeFrequency(
	ctx context.Context,
	owner, repo string,
) ([]CodeFrequencyWeek, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/stats/code_frequency", s.client.baseURL, owner, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github get code frequency failed with status %d", resp.StatusCode)
	}

	var raw [][]int64
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	weeks := make([]CodeFrequencyWeek, 0, len(raw))
	for _, entry := range raw {
		if len(entry) >= 3 {
			weeks = append(weeks, CodeFrequencyWeek{
				Timestamp: time.Unix(entry[0], 0).UTC(),
				Additions: int(entry[1]),
				Deletions: int(entry[2]),
			})
		}
	}

	return weeks, nil
}

// GetTrafficViews retrieves view and unique visitor analytics for the last 14 days.
func (s *SearchAndStatsService) GetTrafficViews(
	ctx context.Context,
	owner, repo string,
) (*RepoTrafficViews, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/traffic/views", s.client.baseURL, owner, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github get traffic views failed with status %d", resp.StatusCode)
	}

	var views RepoTrafficViews
	if err := json.NewDecoder(resp.Body).Decode(&views); err != nil {
		return nil, err
	}

	return &views, nil
}

// GetTrafficClones retrieves clone analytics for the last 14 days.
func (s *SearchAndStatsService) GetTrafficClones(
	ctx context.Context,
	owner, repo string,
) (*RepoTrafficClones, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/traffic/clones", s.client.baseURL, owner, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github get traffic clones failed with status %d", resp.StatusCode)
	}

	var clones RepoTrafficClones
	if err := json.NewDecoder(resp.Body).Decode(&clones); err != nil {
		return nil, err
	}

	return &clones, nil
}

// GetPunchCard retrieves punch card commit distribution across days and hours.
func (s *SearchAndStatsService) GetPunchCard(
	ctx context.Context,
	owner, repo string,
) ([]PunchCardPoint, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/stats/punch_card", s.client.baseURL, owner, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github get punch card failed with status %d", resp.StatusCode)
	}

	var raw [][]int
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	points := make([]PunchCardPoint, 0, len(raw))
	for _, entry := range raw {
		if len(entry) >= 3 {
			points = append(points, PunchCardPoint{
				DayOfWeek: entry[0],
				HourOfDay: entry[1],
				Commits:   entry[2],
			})
		}
	}

	return points, nil
}
