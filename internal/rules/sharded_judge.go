package rules

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/pkg/models"
)

// ShardEvalResult encapsulates the findings and status of an individual rule evaluation shard.
type ShardEvalResult struct {
	ShardIndex   int                  `json:"shard_index"`
	RulesCount   int                  `json:"rules_count"`
	Findings     []models.CodeFinding `json:"findings"`
	Duration     time.Duration        `json:"duration"`
	Error        error                `json:"error,omitempty"`
	ErrorMessage string               `json:"error_message,omitempty"`
}

// ShardedJudgeStats provides observability into sharded rule evaluation.
type ShardedJudgeStats struct {
	TotalRules      int           `json:"total_rules"`
	ShardsRun       int           `json:"shards_run"`
	ShardsSucceeded int           `json:"shards_succeeded"`
	ShardsErrored   int           `json:"shards_errored"`
	TotalFindings   int           `json:"total_findings"`
	Duration        time.Duration `json:"duration"`
}

// ShardExecutor defines the function signature for evaluating a subset of rules.
type ShardExecutor func(ctx context.Context, shardIndex int, rules []RuleSpec, patches []*diff.FilePatch) ([]models.CodeFinding, error)

// ShardedRuleJudge executes large custom rule sets partitioned into bounded shards.
type ShardedRuleJudge struct {
	shardSize   int
	concurrency int
	executor    ShardExecutor
}

// NewShardedRuleJudge initializes the sharded rule evaluation judge.
func NewShardedRuleJudge(shardSize, concurrency int, executor ShardExecutor) *ShardedRuleJudge {
	if shardSize <= 0 {
		shardSize = 5
	}
	if concurrency <= 0 {
		concurrency = 4
	}
	return &ShardedRuleJudge{
		shardSize:   shardSize,
		concurrency: concurrency,
		executor:    executor,
	}
}

// EvaluateSharded partitions rules into shards, executes concurrently, and aggregates results.
func (j *ShardedRuleJudge) EvaluateSharded(
	ctx context.Context,
	reviewID, wsID uuid.UUID,
	rulesList []RuleSpec,
	patches []*diff.FilePatch,
) ([]models.CodeFinding, ShardedJudgeStats) {
	start := time.Now()
	stats := ShardedJudgeStats{
		TotalRules: len(rulesList),
	}

	if len(rulesList) == 0 || len(patches) == 0 {
		stats.Duration = time.Since(start)
		return nil, stats
	}

	// 1. Partition rules into shards
	var shards [][]RuleSpec
	for i := 0; i < len(rulesList); i += j.shardSize {
		end := i + j.shardSize
		if end > len(rulesList) {
			end = len(rulesList)
		}
		shards = append(shards, rulesList[i:end])
	}
	stats.ShardsRun = len(shards)

	// 2. Concurrently evaluate shards with bounded semaphore
	sem := make(chan struct{}, j.concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var allFindings []models.CodeFinding

	for idx, shard := range shards {
		wg.Add(1)
		go func(sIdx int, rShard []RuleSpec) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			shardStart := time.Now()
			findings, err := j.executor(ctx, sIdx, rShard, patches)
			shardDuration := time.Since(shardStart)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				stats.ShardsErrored++
				slog.Warn("Rule judge shard execution error",
					"shard_index", sIdx,
					"rules_count", len(rShard),
					"duration_ms", shardDuration.Milliseconds(),
					"error", err,
				)
			} else {
				stats.ShardsSucceeded++
				allFindings = append(allFindings, findings...)
			}
		}(idx, shard)
	}

	wg.Wait()

	stats.TotalFindings = len(allFindings)
	stats.Duration = time.Since(start)

	if stats.ShardsErrored > 0 && stats.ShardsErrored < stats.ShardsRun {
		slog.Warn("Sharded rule evaluation partial degradation",
			"shards_run", stats.ShardsRun,
			"shards_succeeded", stats.ShardsSucceeded,
			"shards_errored", stats.ShardsErrored,
			"findings_found", stats.TotalFindings,
		)
	} else if stats.ShardsErrored == stats.ShardsRun && stats.ShardsRun > 0 {
		slog.Error("Sharded rule evaluation total failure across all shards",
			"shards_run", stats.ShardsRun,
			"error", fmt.Errorf("100%% of rule judge shards failed"),
		)
	}

	return allFindings, stats
}
