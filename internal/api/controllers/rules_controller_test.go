// ═══════════════════════════════════════════════════════════════════════════
// ScanDrix AI - Rule Studio Controller Tests
// Covers the batch dry-run endpoint, which exists so a dry run over many files
// costs one round trip instead of one per file.
// ═══════════════════════════════════════════════════════════════════════════

package controllers_test

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/api/dtos"
)

func newRulesTestServer() *httptest.Server {
	return httptest.NewServer(controllers.NewRulesController(nil).Routes())
}

func TestHandleTestRuleBatch_ReportsRealDenominators(t *testing.T) {
	srv := newRulesTestServer()
	defer srv.Close()

	body := `{"regex_rule":"password\\s*=","files":[
		{"path":"a.ts","content":"const password = 1"},
		{"path":"b.ts","content":"const x = 2"},
		{"path":"c.ts","content":"password = 3"}]}`

	resp, err := http.Post(srv.URL+"/test/batch", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("batch failed: %v", err)
	}
	defer resp.Body.Close()

	var out dtos.TestRuleBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if out.FilesScanned != 3 {
		t.Errorf("FilesScanned = %d, want 3", out.FilesScanned)
	}
	if out.FilesMatched != 2 {
		t.Errorf("FilesMatched = %d, want 2", out.FilesMatched)
	}
	want := 2.0 / 3.0
	if math.Abs(out.MatchRate-want) > 1e-9 {
		t.Errorf("MatchRate = %v, want %v", out.MatchRate, want)
	}
	if out.Truncated {
		t.Error("a 3-file batch should not report truncation")
	}
}

func TestHandleTestRuleBatch_ZeroFilesYieldsZeroRateNotOne(t *testing.T) {
	srv := newRulesTestServer()
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/test/batch", "application/json",
		strings.NewReader(`{"regex_rule":"x","files":[]}`))
	if err != nil {
		t.Fatalf("batch failed: %v", err)
	}
	defer resp.Body.Close()

	var out dtos.TestRuleBatchResponse
	_ = json.NewDecoder(resp.Body).Decode(&out)

	// Nothing scanned means the rule was not exercised, not that it matched
	// everything. Reporting 1 here would invert the meaning of the score.
	if out.MatchRate != 0 {
		t.Errorf("MatchRate = %v for an empty batch, want 0", out.MatchRate)
	}
	if out.FilesScanned != 0 {
		t.Errorf("FilesScanned = %d, want 0", out.FilesScanned)
	}
}

func TestHandleTestRuleBatch_TruncationIsReported(t *testing.T) {
	srv := newRulesTestServer()
	defer srv.Close()

	files := make([]string, 0, controllers.MaxBatchFiles+25)
	for i := 0; i < controllers.MaxBatchFiles+25; i++ {
		files = append(files, `{"path":"f`+strconv.Itoa(i)+`.ts","content":"x"}`)
	}
	body := `{"regex_rule":"x","files":[` + strings.Join(files, ",") + `]}`

	resp, err := http.Post(srv.URL+"/test/batch", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("batch failed: %v", err)
	}
	defer resp.Body.Close()

	var out dtos.TestRuleBatchResponse
	_ = json.NewDecoder(resp.Body).Decode(&out)

	if !out.Truncated {
		t.Error("an over-limit batch must say it was truncated")
	}
	if out.FilesScanned != controllers.MaxBatchFiles {
		t.Errorf("FilesScanned = %d, want the cap %d", out.FilesScanned, controllers.MaxBatchFiles)
	}
}

func TestHandleTestRuleBatch_InvalidRegexIsRejected(t *testing.T) {
	srv := newRulesTestServer()
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/test/batch", "application/json",
		strings.NewReader(`{"regex_rule":"([","files":[]}`))
	if err != nil {
		t.Fatalf("batch failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an uncompilable pattern", resp.StatusCode)
	}
}
