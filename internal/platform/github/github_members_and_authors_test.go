// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMembersAndAuthorsService(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/orgs/my-org/members":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"id": 1, "login": "alice", "type": "User"}, {"id": 2, "login": "bob", "type": "User"}]`))
		case r.URL.Path == "/orgs/my-org/members/alice":
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/orgs/my-org/members/charlie":
			w.WriteHeader(http.StatusNotFound)
		case r.URL.Path == "/orgs/my-org/memberships/alice":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"state": "active", "role": "member", "user": {"suspended": false}}`))
		case r.URL.Path == "/orgs/my-org/memberships/bob":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"state": "active", "role": "member", "user": {"suspended": true}}`))
		case r.URL.Path == "/repos/my-org/my-repo/pulls":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"user": {"login": "alice"}}, {"user": {"login": "bob"}}, {"user": {"login": "alice"}}]`))
		case r.URL.Path == "/search/issues":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"items": [{"number": 101, "title": "Add feature X", "state": "open", "html_url": "https://github.com/my-org/my-repo/pull/101", "user": {"login": "alice"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	client := NewAdapter(ts.URL, "test-token")
	svc := NewMembersAndAuthorsService(client)
	ctx := context.Background()

	// 1. GetListMembers
	members, err := svc.GetListMembers(ctx, "my-org", "", 1, 30)
	if err != nil {
		t.Fatalf("unexpected error getting list members: %v", err)
	}
	if len(members) != 2 || members[0].Login != "alice" {
		t.Fatalf("unexpected members result: %+v", members)
	}

	// 2. FilterMembers
	active, err := svc.FilterMembers(ctx, "my-org", []string{"alice", "charlie"})
	if err != nil {
		t.Fatalf("unexpected error filtering members: %v", err)
	}
	if len(active) != 1 || active[0].Login != "alice" {
		t.Fatalf("unexpected filter members result: %+v", active)
	}

	// 3. GetSuspendedStatusBatch
	suspendedMap, err := svc.GetSuspendedStatusBatch(ctx, "my-org", []string{"alice", "bob"})
	if err != nil {
		t.Fatalf("unexpected error getting suspended batch: %v", err)
	}
	if suspendedMap["alice"] != false || suspendedMap["bob"] != true {
		t.Fatalf("unexpected suspended status map: %+v", suspendedMap)
	}

	// 4. GetPullRequestAuthors
	authors, err := svc.GetPullRequestAuthors(ctx, "my-org", "my-repo", "open")
	if err != nil {
		t.Fatalf("unexpected error getting authors: %v", err)
	}
	if len(authors) != 2 || authors[0] != "alice" || authors[1] != "bob" {
		t.Fatalf("unexpected unique authors: %+v", authors)
	}

	// 5. SearchPullRequestsByTitle
	prs, err := svc.SearchPullRequestsByTitle(ctx, "my-org", "my-repo", "feature")
	if err != nil {
		t.Fatalf("unexpected error searching PRs: %v", err)
	}
	if len(prs) != 1 || prs[0].Number != 101 || prs[0].Title != "Add feature X" {
		t.Fatalf("unexpected search PRs result: %+v", prs)
	}
}
