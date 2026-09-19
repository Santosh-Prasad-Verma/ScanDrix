package checker_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/review/checker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestASTSyntaxMatrix_TypeScriptComplexFeatures(t *testing.T) {
	v := checker.NewASTSyntaxValidator()
	ctx := context.Background()

	t.Run("TypeScript Generics and Async Arrow Functions", func(t *testing.T) {
		code := `
export interface PaginatedResult<T> {
	items: readonly T[];
	total: number;
	cursor?: string | null;
}

export const fetchPaginated = async <T, R extends PaginatedResult<T>>(
	url: string,
	options?: RequestInit
): Promise<R> => {
	const response = await fetch(url, options);
	if (!response.ok) {
		throw new Error("HTTP " + response.status);
	}
	const data: R = await response.json();
	return data;
};
`
		report := v.ValidateFile(ctx, "src/api/client.ts", code)
		assert.True(t, report.IsValid)
		assert.Equal(t, checker.LangTypeScript, report.Language)
		assert.Empty(t, report.Diagnostics)
	})

	t.Run("TSX Functional Component with React Hooks and Fragments", func(t *testing.T) {
		code := `
import React, { useState, useEffect, useMemo } from 'react';

interface FindingCardProps {
	id: string;
	title: string;
	severity: 'CRITICAL' | 'HIGH' | 'MEDIUM' | 'LOW';
	onDismiss?: (id: string) => void;
}

export const FindingCard: React.FC<FindingCardProps> = ({ id, title, severity, onDismiss }) => {
	const [expanded, setExpanded] = useState<boolean>(false);
	const badgeColor = useMemo(() => {
		switch (severity) {
			case 'CRITICAL': return 'bg-red-500 text-white';
			case 'HIGH': return 'bg-orange-500 text-white';
			default: return 'bg-slate-700 text-slate-200';
		}
	}, [severity]);

	return (
		<>
			<div className="p-4 border rounded-lg shadow-sm" data-testid="finding-card">
				<div className="flex items-center justify-between">
					<h3 className="font-semibold text-lg">{title}</h3>
					<span className={badgeColor}>{severity}</span>
				</div>
				{expanded && <div className="mt-2 text-sm text-gray-600">Finding details...</div>}
				<button onClick={() => setExpanded(!expanded)}>Toggle</button>
				{onDismiss && <button onClick={() => onDismiss(id)}>Dismiss</button>}
			</div>
		</>
	);
};
`
		report := v.ValidateFile(ctx, "components/FindingCard.tsx", code)
		assert.True(t, report.IsValid)
		assert.Equal(t, checker.LangTSX, report.Language)
		assert.Empty(t, report.Diagnostics)
	})

	t.Run("TypeScript Unbalanced Parenthesis Fails Validation", func(t *testing.T) {
		code := `
export const mapData = (data: string[] => {
	return data;
};
`
		report := v.ValidateFile(ctx, "src/broken.ts", code)
		assert.False(t, report.IsValid)
		assert.NotEmpty(t, report.Diagnostics)
	})
}

func TestASTSyntaxMatrix_PythonModernFeatures(t *testing.T) {
	v := checker.NewASTSyntaxValidator()
	ctx := context.Background()

	t.Run("Python 3.10+ Pattern Matching, Type Hints, and Context Managers", func(t *testing.T) {
		code := `
import asyncio
from typing import Dict, Any, Optional, List
from contextlib import asynccontextmanager

class FindingProcessor:
    def __init__(self, workspace_id: str) -> None:
        self.workspace_id = workspace_id
        self._cache: Dict[str, Any] = {}

    @asynccontextmanager
    async def session_scope(self):
        session = {"id": "test-session", "active": True}
        try:
            yield session
        finally:
            session["active"] = False

    async def categorize_severity(self, finding: Dict[str, Any]) -> str:
        match finding.get("score"):
            case float(x) if x >= 0.90:
                return "CRITICAL"
            case float(x) if x >= 0.70:
                return "HIGH"
            case float(x) if x >= 0.40:
                return "MEDIUM"
            case _:
                return "LOW"
`
		report := v.ValidateFile(ctx, "engine/processor.py", code)
		assert.True(t, report.IsValid)
		assert.Equal(t, checker.LangPython, report.Language)
		assert.Empty(t, report.Diagnostics)
	})

	t.Run("Python Syntax Error Indentation and Missing Colon", func(t *testing.T) {
		code := `
def calculate_score(finding)
    score = 0
    return score
`
		report := v.ValidateFile(ctx, "engine/broken.py", code)
		assert.False(t, report.IsValid)
		assert.NotEmpty(t, report.Diagnostics)
	})
}

func TestASTSyntaxMatrix_GoAdvancedFeatures(t *testing.T) {
	v := checker.NewASTSyntaxValidator()
	ctx := context.Background()

	t.Run("Go 1.18+ Generics, Channels, and Select", func(t *testing.T) {
		code := `package pipeline

import (
	"context"
	"sync"
	"time"
)

type Result[T any] struct {
	Value T
	Err   error
}

func FanOutProcess[T any, R any](ctx context.Context, items []T, workers int, fn func(context.Context, T) (R, error)) ([]R, error) {
	inCh := make(chan T, len(items))
	outCh := make(chan Result[R], len(items))
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range inCh {
				res, err := fn(ctx, item)
				outCh <- Result[R]{Value: res, Err: err}
			}
		}()
	}

	for _, it := range items {
		inCh <- it
	}
	close(inCh)
	wg.Wait()
	close(outCh)

	results := make([]R, 0, len(items))
	for res := range outCh {
		if res.Err != nil {
			return nil, res.Err
		}
		results = append(results, res.Value)
	}
	return results, nil
}
`
		report := v.ValidateFile(ctx, "pkg/pipeline/fanout.go", code)
		assert.True(t, report.IsValid)
		assert.Equal(t, checker.LangGo, report.Language)
		assert.Empty(t, report.Diagnostics)
		assert.Greater(t, report.Metrics.TokenCount, 5)
	})

	t.Run("Go Syntax Error Unclosed Parenthesis in Signature", func(t *testing.T) {
		code := `package broken

func ExecuteWork(ctx context.Context, name string {
	return
}
`
		report := v.ValidateFile(ctx, "broken.go", code)
		assert.False(t, report.IsValid)
		assert.NotEmpty(t, report.Diagnostics)
	})
}

func TestASTSyntaxMatrix_RustLanguageValidation(t *testing.T) {
	v := checker.NewASTSyntaxValidator()
	ctx := context.Background()

	t.Run("Rust Struct, Lifetimes, Traits, and Pattern Matching", func(t *testing.T) {
		code := `
use std::collections::HashMap;

pub trait ReviewObserver {
	fn on_finding(&mut self, finding_id: &str) -> Result<(), String>;
}

pub struct FindingRegistry<'a> {
	workspace: &'a str,
	findings: HashMap<String, u32>,
}

impl<'a> FindingRegistry<'a> {
	pub fn new(workspace: &'a str) -> Self {
		FindingRegistry {
			workspace,
			findings: HashMap::new(),
		}
	}

	pub fn record(&mut self, id: String, count: u32) {
		self.findings.insert(id, count);
	}
}
`
		report := v.ValidateFile(ctx, "src/registry.rs", code)
		assert.True(t, report.IsValid)
		assert.Equal(t, checker.LangRust, report.Language)
		assert.Empty(t, report.Diagnostics)
	})

	t.Run("Rust Unbalanced Curly Brackets Fails Validation", func(t *testing.T) {
		code := `
pub fn broken() {
    let x = 42;
`
		report := v.ValidateFile(ctx, "src/broken.rs", code)
		assert.False(t, report.IsValid)
		assert.NotEmpty(t, report.Diagnostics)
	})
}

func TestASTSyntaxMatrix_SQLAdvancedQueries(t *testing.T) {
	v := checker.NewASTSyntaxValidator()
	ctx := context.Background()

	t.Run("SQL CTEs, Window Functions, and UPSERT", func(t *testing.T) {
		sql := `
WITH ranked_reviews AS (
	SELECT id, workspace_id, author_username,
	       ROW_NUMBER() OVER (PARTITION BY workspace_id ORDER BY created_at DESC) as rank
	FROM pull_request_reviews
	WHERE state = 'COMPLETED'
)
INSERT INTO review_summaries (workspace_id, top_reviewer, computed_at)
SELECT workspace_id, author_username, NOW()
FROM ranked_reviews
WHERE rank = 1
ON CONFLICT (workspace_id) DO UPDATE SET
	top_reviewer = EXCLUDED.top_reviewer,
	computed_at = EXCLUDED.computed_at;
`
		report := v.ValidateFile(ctx, "migrations/005_summaries.sql", sql)
		assert.True(t, report.IsValid)
		assert.Equal(t, checker.LangSQL, report.Language)
		assert.Empty(t, report.Diagnostics)
	})

	t.Run("SQL Unbalanced Quotes Fails Validation", func(t *testing.T) {
		sql := `SELECT * FROM users WHERE email = 'unterminated_string AND active = true;`
		report := v.ValidateFile(ctx, "query.sql", sql)
		assert.False(t, report.IsValid)
		assert.NotEmpty(t, report.Diagnostics)
	})
}

func TestASTSyntaxMatrix_StructuredDataValidation(t *testing.T) {
	v := checker.NewASTSyntaxValidator()
	ctx := context.Background()

	t.Run("Valid JSON Schema Config", func(t *testing.T) {
		jsonContent := `{
	"version": "1.0",
	"rules": [
		{
			"id": "SEC-001",
			"name": "Hardcoded Secrets",
			"severity": "CRITICAL",
			"enabled": true,
			"patterns": ["AKIA[0-9A-Z]{16}", "ghp_[a-zA-Z0-9]{36}"]
		}
	],
	"review_cadence": {
		"debounce_ms": 500,
		"max_concurrent_files": 16
	}
}`
		report := v.ValidateFile(ctx, ".scandrix/config.json", jsonContent)
		assert.True(t, report.IsValid)
		assert.Equal(t, checker.LangJSON, report.Language)
		assert.Empty(t, report.Diagnostics)
	})

	t.Run("Invalid JSON Missing Comma", func(t *testing.T) {
		badJSON := `{"key": "value" "next": 123}`
		report := v.ValidateFile(ctx, "config.json", badJSON)
		assert.False(t, report.IsValid)
		assert.NotEmpty(t, report.Diagnostics)
	})

	t.Run("Valid YAML Pipeline Workflow", func(t *testing.T) {
		yamlContent := `name: ScanDrix Code Review
on:
  pull_request:
    types: [opened, synchronize, reopened]

jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Run ScanDrix CLI
        run: |
          scandrix review run --pr ${{ github.event.number }}
`
		report := v.ValidateFile(ctx, ".github/workflows/review.yml", yamlContent)
		assert.True(t, report.IsValid)
		assert.Equal(t, checker.LangYAML, report.Language)
		assert.Empty(t, report.Diagnostics)
	})
}

func TestASTSyntaxMatrix_ConcurrentParsingThroughput(t *testing.T) {
	v := checker.NewASTSyntaxValidator()
	workers := 16
	iterations := 100

	goSample := `package mathutil
func Abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
`
	tsSample := `export function clamp(val: number, min: number, max: number): number {
	return Math.min(Math.max(val, min), max);
}
`
	var wg sync.WaitGroup
	start := time.Now()

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			ctx := context.Background()

			for i := 0; i < iterations; i++ {
				if i%2 == 0 {
					rep := v.ValidateFile(ctx, "math.go", goSample)
					require.True(t, rep.IsValid)
				} else {
					rep := v.ValidateFile(ctx, "util.ts", tsSample)
					require.True(t, rep.IsValid)
				}
			}
		}(w)
	}

	wg.Wait()
	elapsed := time.Since(start)

	totalParsed := workers * iterations
	t.Logf("Parsed %d files concurrently across %d workers in %v (%.0f files/sec)", totalParsed, workers, elapsed, float64(totalParsed)/elapsed.Seconds())
	assert.Less(t, elapsed, 10*time.Second)
}
