// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package orchestration

import (
	"context"
	"os"
	"strconv"
	"sync"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

// DroppedCandidate pairs a rejected candidate output with its refutation verdict.
type DroppedCandidate[T any] struct {
	Candidate T
	Verdict   contracts.Verdict
}

// VerificationPassParams configures a concurrent verification driver pass.
type VerificationPassParams[T any] struct {
	Candidates  []T
	Verifier    contracts.Verifier[T]
	Concurrency int
}

// VerificationPassResult captures the partitioned results of a verification pass.
type VerificationPassResult[T any] struct {
	Kept         []T
	KeptVerdicts []contracts.Verdict
	Dropped      []DroppedCandidate[T]
}

// DefaultVerificationConcurrency returns the configured concurrency or 4.
func DefaultVerificationConcurrency() int {
	if v := os.Getenv("VERIFICATION_CONCURRENCY"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			return parsed
		}
	}
	return 4
}

// RunVerificationPass runs a generic "checker" pass over candidate outputs.
// It executes ONE Verifier per candidate with bounded concurrency and partitions the results.
//
// Default Semantics: KEEP.
// A candidate is dropped ONLY on an explicit keep=false verdict.
// Any panic or error inside the Verifier fails OPEN (keep=true), ensuring infrastructure
// glitches never silently discard findings.
func RunVerificationPass[T any](
	ctx context.Context,
	params VerificationPassParams[T],
	toolCtx contracts.ToolContext,
) (*VerificationPassResult[T], error) {
	concurrency := params.Concurrency
	if concurrency <= 0 {
		concurrency = DefaultVerificationConcurrency()
	}

	result := &VerificationPassResult[T]{
		Kept:         make([]T, 0),
		KeptVerdicts: make([]contracts.Verdict, 0),
		Dropped:      make([]DroppedCandidate[T], 0),
	}

	candidates := params.Candidates
	if len(candidates) == 0 || params.Verifier == nil {
		return result, nil
	}

	for i := 0; i < len(candidates); i += concurrency {
		end := i + concurrency
		if end > len(candidates) {
			end = len(candidates)
		}
		batch := candidates[i:end]

		type pair struct {
			candidate T
			verdict   contracts.Verdict
		}

		verdictPairs := make([]pair, len(batch))
		var wg sync.WaitGroup

		for idx, cand := range batch {
			wg.Add(1)
			go func(cIndex int, candidate T) {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						// Fail OPEN on unexpected panic
						verdictPairs[cIndex] = pair{
							candidate: candidate,
							verdict: contracts.Verdict{
								Keep:      true,
								Rationale: "verifier panicked — fail open default",
							},
						}
					}
				}()

				verdict, err := params.Verifier.Verify(ctx, candidate, toolCtx)
				if err != nil {
					// Fail OPEN on unexpected verification error
					verdictPairs[cIndex] = pair{
						candidate: candidate,
						verdict: contracts.Verdict{
							Keep:      true,
							Rationale: "verifier failed — fail open default",
						},
					}
					return
				}

				verdictPairs[cIndex] = pair{
					candidate: candidate,
					verdict:   verdict,
				}
			}(idx, cand)
		}

		wg.Wait()

		for _, p := range verdictPairs {
			if p.verdict.Keep {
				result.Kept = append(result.Kept, p.candidate)
				result.KeptVerdicts = append(result.KeptVerdicts, p.verdict)
			} else {
				result.Dropped = append(result.Dropped, DroppedCandidate[T]{
					Candidate: p.candidate,
					Verdict:   p.verdict,
				})
			}
		}
	}

	return result, nil
}
