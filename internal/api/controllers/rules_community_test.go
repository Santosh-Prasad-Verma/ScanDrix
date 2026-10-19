// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package controllers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/rules/community"
)

func TestHandleListCommunityPacks(t *testing.T) {
	srv := httptest.NewServer(controllers.NewRulesController(nil).Routes())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/community/packs")
	if err != nil {
		t.Fatalf("failed calling /community/packs: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got: %d", resp.StatusCode)
	}

	var packs []community.RulePack
	if err := json.NewDecoder(resp.Body).Decode(&packs); err != nil {
		t.Fatalf("failed decoding packs: %v", err)
	}

	if len(packs) == 0 {
		t.Fatal("expected at least one community pack")
	}

	// Filter by search query
	searchResp, err := http.Get(srv.URL + "/community/packs?search=owasp")
	if err != nil {
		t.Fatalf("failed calling search: %v", err)
	}
	defer searchResp.Body.Close()

	var searchPacks []community.RulePack
	if err := json.NewDecoder(searchResp.Body).Decode(&searchPacks); err != nil {
		t.Fatalf("failed decoding search packs: %v", err)
	}

	if len(searchPacks) == 0 {
		t.Fatal("expected to find OWASP pack via search")
	}
}

func TestHandleGetCommunityPack(t *testing.T) {
	srv := httptest.NewServer(controllers.NewRulesController(nil).Routes())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/community/packs/pack-owasp-top10")
	if err != nil {
		t.Fatalf("failed calling get pack: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got: %d", resp.StatusCode)
	}

	var pack community.RulePack
	if err := json.NewDecoder(resp.Body).Decode(&pack); err != nil {
		t.Fatalf("failed decoding pack: %v", err)
	}

	if pack.ID != "pack-owasp-top10" {
		t.Errorf("expected pack-owasp-top10, got %s", pack.ID)
	}
}
