// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Autonomous Security CTF Benchmark (SDRX-CTF)
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package security

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// EvaluateChallenge evaluates whether an exploit payload successfully neutralizes/solves the challenge.
func EvaluateChallenge(challenge CTFChallenge, generatedPayload string, latency time.Duration) CTFRunResult {
	result := CTFRunResult{
		ChallengeID:      challenge.ID,
		Category:         challenge.Category,
		Difficulty:       challenge.Difficulty,
		GeneratedPayload: generatedPayload,
		Latency:          latency,
	}

	trimmed := strings.TrimSpace(generatedPayload)
	if trimmed == "" {
		result.Solved = false
		result.ErrorMessage = "empty exploit payload supplied"
		return result
	}

	if challenge.PayloadRegex != "" {
		re, err := regexp.Compile(challenge.PayloadRegex)
		if err != nil {
			result.Solved = false
			result.ErrorMessage = fmt.Sprintf("invalid challenge validation regex: %v", err)
			return result
		}

		if re.MatchString(trimmed) {
			result.Solved = true
			result.CanaryRetrieved = challenge.CanaryExpected
			return result
		}
	}

	// Fallback containment check for expected canary or keyword
	if challenge.CanaryExpected != "" && strings.Contains(trimmed, challenge.CanaryExpected) {
		result.Solved = true
		result.CanaryRetrieved = challenge.CanaryExpected
		return result
	}

	result.Solved = false
	result.ErrorMessage = "exploit payload failed to trigger vulnerability canary"
	return result
}

// RunCTFSuite runs the benchmark against a challenge solver and produces an aggregated report.
func RunCTFSuite(challenges []CTFChallenge, solver func(CTFChallenge) (string, time.Duration)) CTFBenchmarkReport {
	report := CTFBenchmarkReport{
		TotalChallenges:    len(challenges),
		SolvedByCategory:   make(map[string]CategoryStats),
		SolvedByDifficulty: make(map[string]DiffStats),
		Results:            make([]CTFRunResult, 0, len(challenges)),
	}

	var totalLatency time.Duration

	for _, ch := range challenges {
		payload, latency := solver(ch)
		totalLatency += latency

		res := EvaluateChallenge(ch, payload, latency)
		report.Results = append(report.Results, res)

		if res.Solved {
			report.SolvedCount++
		}

		// Category stats
		catKey := string(ch.Category)
		cStats := report.SolvedByCategory[catKey]
		cStats.Total++
		if res.Solved {
			cStats.Solved++
		}
		report.SolvedByCategory[catKey] = cStats

		// Difficulty stats
		diffKey := string(ch.Difficulty)
		dStats := report.SolvedByDifficulty[diffKey]
		dStats.Total++
		if res.Solved {
			dStats.Solved++
		}
		report.SolvedByDifficulty[diffKey] = dStats
	}

	if len(challenges) > 0 {
		report.SolveRate = float64(report.SolvedCount) / float64(len(challenges))
		report.MeanLatency = totalLatency / time.Duration(len(challenges))
	}

	for k, v := range report.SolvedByCategory {
		if v.Total > 0 {
			v.SolveRate = float64(v.Solved) / float64(v.Total)
			report.SolvedByCategory[k] = v
		}
	}

	for k, v := range report.SolvedByDifficulty {
		if v.Total > 0 {
			v.SolveRate = float64(v.Solved) / float64(v.Total)
			report.SolvedByDifficulty[k] = v
		}
	}

	return report
}

// LoadCTFChallenges loads CTF challenges from a JSON dataset file.
func LoadCTFChallenges(filePath string) ([]CTFChallenge, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed reading CTF challenges from %s: %w", filePath, err)
	}

	var challenges []CTFChallenge
	if err := json.Unmarshal(data, &challenges); err != nil {
		return nil, fmt.Errorf("failed unmarshaling CTF challenges: %w", err)
	}

	return challenges, nil
}
