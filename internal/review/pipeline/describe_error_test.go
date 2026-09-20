// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package pipeline_test

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/review/pipeline"
)

func TestDescribePipelineError_PrefersClassificationAtThrowSite(t *testing.T) {
	info := &pipeline.ReviewErrorInfo{
		Category:        "MODEL_NOT_FOUND",
		FriendlyMessage: "The configured model is not available for this organization.",
		ProviderMessage: "404 Not Found: model anthropic/claude-3-7-sonnet does not exist",
		OccurredAt:      time.Now().UTC(),
	}

	rawErr := errors.New("raw http 404")
	described := pipeline.DescribePipelineError(rawErr, info)

	if !described.Classified {
		t.Errorf("expected Classified = true, got false")
	}
	expected := "The configured model is not available for this organization."
	if described.Text != expected {
		t.Errorf("expected '%s', got '%s'", expected, described.Text)
	}
}

func TestDescribePipelineError_PrefersClassifiedErrorWrapper(t *testing.T) {
	wrapped := &pipeline.ClassifiedError{
		Err:             errors.New("dial tcp 10.0.1.5:443: i/o timeout"),
		Category:        "NETWORK_TIMEOUT",
		FriendlyMessage: "Connection to the AI provider timed out during prompt inference.",
	}

	described := pipeline.DescribePipelineError(wrapped, nil)

	if !described.Classified {
		t.Errorf("expected Classified = true, got false")
	}
	expected := "Connection to the AI provider timed out during prompt inference."
	if described.Text != expected {
		t.Errorf("expected '%s', got '%s'", expected, described.Text)
	}
}

func TestDescribePipelineError_DoesNotReclassifyUnattachedError(t *testing.T) {
	// A GitHub API rate limit error should NOT be converted into "Rate limit reached on AI provider"
	githubErr := errors.New("GitHub API rate limit exceeded for token")
	described := pipeline.DescribePipelineError(githubErr, nil)

	if described.Classified {
		t.Errorf("expected Classified = false for unattached error")
	}
	if described.Text != "GitHub API rate limit exceeded for token" {
		t.Errorf("expected exact raw message, got '%s'", described.Text)
	}
}

func TestDescribePipelineError_FallsBackToRawMessage(t *testing.T) {
	err := errors.New("disk volume /var/data out of space")
	described := pipeline.DescribePipelineError(err, nil)

	if described.Classified {
		t.Errorf("expected Classified = false")
	}
	if described.Text != "disk volume /var/data out of space" {
		t.Errorf("expected raw message, got '%s'", described.Text)
	}
}

func TestDescribePipelineError_CutsMultiSentenceAtSentenceBoundary(t *testing.T) {
	longRationale := "Drixy Rules could not be evaluated: all 3 rule checks failed to run. " +
		"Reporting 0 findings would green-wash a review that evaluated nothing, " +
		"so we fail loudly instead and mark the execution degraded for the operator."

	err := errors.New(longRationale)
	described := pipeline.DescribePipelineError(err, nil)

	expected := "Drixy Rules could not be evaluated: all 3 rule checks failed to run."
	if described.Text != expected {
		t.Errorf("expected sentence boundary cut:\nwant: '%s'\ngot:  '%s'", expected, described.Text)
	}
}

func TestDescribePipelineError_CollapsesNewlinesAndWhitespace(t *testing.T) {
	err := errors.New("first line\n\n   second line \t with  multiple    spaces")
	described := pipeline.DescribePipelineError(err, nil)

	expected := "first line second line with multiple spaces"
	if described.Text != expected {
		t.Errorf("expected collapsed whitespace, got '%s'", described.Text)
	}
}

func TestDescribePipelineError_MissingErrorReturnsEmpty(t *testing.T) {
	described := pipeline.DescribePipelineError(nil, nil)

	if described.Text != "" {
		t.Errorf("expected empty string, got '%s'", described.Text)
	}
	if described.Classified {
		t.Errorf("expected Classified = false")
	}
}

func TestDescribePipelineError_LongSentenceTruncationWithEllipsis(t *testing.T) {
	// A very long single sentence with no early period
	longMessage := "This is a single continuous uninterrupted sentence describing an extensive failure in the internal parser subsystem that lacks punctuation and spans past one hundred and sixty characters in length"
	err := errors.New(longMessage)
	described := pipeline.DescribePipelineError(err, nil)

	if len(described.Text) > 165 {
		t.Errorf("expected capped length around 160, got %d", len(described.Text))
	}
	if !strings.HasSuffix(described.Text, "…") {
		t.Errorf("expected ellipsis suffix, got '%s'", described.Text)
	}
}

func TestToOneLine_CustomLengthAndBoundaries(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		maxLen    int
		expected  string
	}{
		{
			name:     "Short string stays intact",
			input:    "All systems operational.",
			maxLen:   50,
			expected: "All systems operational.",
		},
		{
			name:     "Sentence boundary honored when > 40 chars",
			input:    "Database connection pool exhausted after 30 attempts. Additional diagnostic details follow below.",
			maxLen:   70,
			expected: "Database connection pool exhausted after 30 attempts.",
		},
		{
			name:     "No punctuation falls back to ellipsis",
			input:    "abcdefghijklmnopqrstuvwxyz 0123456789 ABCDEFGHIJKLMNOPQRSTUVWXYZ",
			maxLen:   30,
			expected: "abcdefghijklmnopqrstuvwxyz 012…",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := pipeline.ToOneLine(tc.input, tc.maxLen)
			if res != tc.expected {
				t.Errorf("ToOneLine: want '%s', got '%s'", tc.expected, res)
			}
		})
	}
}

func TestDescribePipelineError_ConcurrencyRaceSafety(t *testing.T) {
	var wg sync.WaitGroup
	workers := 25
	iterations := 100

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				var err error
				var info *pipeline.ReviewErrorInfo

				if (id+i)%3 == 0 {
					info = &pipeline.ReviewErrorInfo{
						Category:        "RATE_LIMIT",
						FriendlyMessage: fmt.Sprintf("Rate limit hit on worker %d", id),
					}
				} else if (id+i)%3 == 1 {
					err = &pipeline.ClassifiedError{
						Category:        "TIMEOUT",
						FriendlyMessage: fmt.Sprintf("Timeout on worker %d", id),
					}
				} else {
					err = fmt.Errorf("Raw error on worker %d iteration %d with newline\nand extra text", id, i)
				}

				res := pipeline.DescribePipelineError(err, info)
				if res.Text == "" {
					t.Errorf("unexpected empty error description")
				}
			}
		}(w)
	}

	wg.Wait()
}
