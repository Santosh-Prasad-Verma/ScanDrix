// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Evaluation Suite
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package results

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"
)

// EvalRunRecord represents a single recorded evaluation execution line.
type EvalRunRecord struct {
	Timestamp  time.Time      `json:"timestamp"`
	Suite      string         `json:"suite"`
	Model      string         `json:"model"`
	Provider   string         `json:"provider"`
	Passed     bool           `json:"passed"`
	Score      float64        `json:"score"`
	Metrics    map[string]any `json:"metrics,omitempty"`
}

// EvalAggregator accumulates multiple run records into aggregate statistics.
type AggregateStats struct {
	TotalRuns  int     `json:"total_runs"`
	PassedRuns int     `json:"passed_runs"`
	PassRate   float64 `json:"pass_rate"`
	MeanScore  float64 `json:"mean_score"`
}

// ResultsRecorder manages JSONL append-only evaluation result streams.
type ResultsRecorder struct {
	mu       sync.Mutex
	filePath string
}

// NewResultsRecorder creates a recorder writing to the target JSONL file.
func NewResultsRecorder(filePath string) *ResultsRecorder {
	return &ResultsRecorder{
		filePath: filePath,
	}
}

// Record appends a single evaluation record to the results file.
func (r *ResultsRecorder) Record(rec EvalRunRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if rec.Timestamp.IsZero() {
		rec.Timestamp = time.Now()
	}

	bytes, err := json.Marshal(rec)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(r.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(append(bytes, '\n'))
	return err
}

// Aggregate reads all records from the file and computes summary metrics.
func (r *ResultsRecorder) Aggregate() (AggregateStats, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	f, err := os.Open(r.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return AggregateStats{}, nil
		}
		return AggregateStats{}, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var total, passed int
	var scoreSum float64

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}

		var rec EvalRunRecord
		if err := json.Unmarshal([]byte(line), &rec); err == nil {
			total++
			if rec.Passed {
				passed++
			}
			scoreSum += rec.Score
		}
	}

	if total == 0 {
		return AggregateStats{}, nil
	}

	return AggregateStats{
		TotalRuns:  total,
		PassedRuns: passed,
		PassRate:   float64(passed) / float64(total),
		MeanScore:  scoreSum / float64(total),
	}, scanner.Err()
}
