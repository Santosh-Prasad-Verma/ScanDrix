// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Autonomous Security CTF Benchmark (SDRX-CTF)
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package security

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLoadCTFChallenges(t *testing.T) {
	datasetPath := filepath.Join("datasets", "sdrx_ctf_challenges.json")
	challenges, err := LoadCTFChallenges(datasetPath)
	if err != nil {
		t.Fatalf("LoadCTFChallenges failed: %v", err)
	}

	if len(challenges) != 5 {
		t.Fatalf("expected 5 challenges in dataset, got %d", len(challenges))
	}

	catMap := make(map[CTFCategory]bool)
	for _, ch := range challenges {
		catMap[ch.Category] = true
	}

	expectedCategories := []CTFCategory{
		CategorySQLInjection,
		CategoryCommandInjection,
		CategorySSRF,
		CategoryIDOR,
		CategoryPathTraversal,
	}

	for _, cat := range expectedCategories {
		if !catMap[cat] {
			t.Errorf("expected challenge category %s in dataset", cat)
		}
	}
}

func TestEvaluateChallenge_PayloadMatching(t *testing.T) {
	ch := CTFChallenge{
		ID:             "test-sqli-1",
		Category:       CategorySQLInjection,
		Difficulty:     DifficultyEasy,
		CanaryExpected: "CANARY_AUTH_OK",
		PayloadRegex:   `(?i)('|\")\s*OR\s*.*--`,
	}

	// 1. Successful exploit payload
	res := EvaluateChallenge(ch, "' OR 1=1 --", 50*time.Millisecond)
	if !res.Solved {
		t.Errorf("expected payload to solve challenge, got error: %s", res.ErrorMessage)
	}
	if res.CanaryRetrieved != "CANARY_AUTH_OK" {
		t.Errorf("expected canary CANARY_AUTH_OK, got %s", res.CanaryRetrieved)
	}

	// 2. Benign / failing payload
	resFail := EvaluateChallenge(ch, "alice", 30*time.Millisecond)
	if resFail.Solved {
		t.Errorf("expected benign input to fail challenge")
	}

	// 3. Empty payload
	resEmpty := EvaluateChallenge(ch, "", 10*time.Millisecond)
	if resEmpty.Solved || resEmpty.ErrorMessage == "" {
		t.Errorf("expected error on empty payload")
	}
}

func TestRunCTFSuite_AggregatedMetrics(t *testing.T) {
	challenges := []CTFChallenge{
		{
			ID:             "c1",
			Category:       CategorySQLInjection,
			Difficulty:     DifficultyEasy,
			CanaryExpected: "CANARY_1",
			PayloadRegex:   `(?i)OR\s*1=1`,
		},
		{
			ID:             "c2",
			Category:       CategoryCommandInjection,
			Difficulty:     DifficultyMedium,
			CanaryExpected: "CANARY_2",
			PayloadRegex:   `;\s*id`,
		},
	}

	// Solver that solves c1 but fails c2
	solver := func(ch CTFChallenge) (string, time.Duration) {
		if ch.ID == "c1" {
			return "' OR 1=1 --", 45 * time.Millisecond
		}
		return "invalid payload", 20 * time.Millisecond
	}

	report := RunCTFSuite(challenges, solver)

	if report.TotalChallenges != 2 {
		t.Errorf("expected 2 total challenges, got %d", report.TotalChallenges)
	}
	if report.SolvedCount != 1 {
		t.Errorf("expected 1 solved challenge, got %d", report.SolvedCount)
	}
	if report.SolveRate != 0.5 {
		t.Errorf("expected 50%% solve rate, got %.2f", report.SolveRate)
	}

	sqliStats, ok := report.SolvedByCategory[string(CategorySQLInjection)]
	if !ok || sqliStats.Solved != 1 {
		t.Errorf("expected SQLi category stats to show 1 solved, got %+v", sqliStats)
	}

	cmdStats, ok := report.SolvedByCategory[string(CategoryCommandInjection)]
	if !ok || cmdStats.Solved != 0 {
		t.Errorf("expected Command Injection stats to show 0 solved, got %+v", cmdStats)
	}
}
