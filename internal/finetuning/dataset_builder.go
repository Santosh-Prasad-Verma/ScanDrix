package finetuning

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
)

// FeedbackStatus indicates developer sentiment on an AI code review finding.
type FeedbackStatus string

const (
	FeedbackAccepted FeedbackStatus = "accepted"
	FeedbackRejected FeedbackStatus = "rejected"
	FeedbackIgnored  FeedbackStatus = "ignored"
)

// ReviewSuggestionTrainingSample represents a single fine-tuning data point.
type ReviewSuggestionTrainingSample struct {
	ID                 uuid.UUID      `json:"id"`
	WorkspaceID        uuid.UUID      `json:"workspace_id"`
	SystemPrompt       string         `json:"system_prompt"`
	InputCodeDiff      string         `json:"input_code_diff"`
	ExpectedSuggestion string         `json:"expected_suggestion"`
	Status             FeedbackStatus `json:"status"`
	CapturedAt         time.Time      `json:"captured_at"`
}

// OpenAIMessage represents the chat completion format for fine-tuning.
type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatFineTuningEntry represents an individual JSONL line for OpenAI/Gemini tuning.
type ChatFineTuningEntry struct {
	Messages []OpenAIMessage `json:"messages"`
}

// FineTuningDatasetBuilder accumulates verified developer reactions into training corpora.
type FineTuningDatasetBuilder struct {
	mu      sync.RWMutex
	samples []ReviewSuggestionTrainingSample
}

func NewFineTuningDatasetBuilder() *FineTuningDatasetBuilder {
	return &FineTuningDatasetBuilder{
		samples: make([]ReviewSuggestionTrainingSample, 0),
	}
}

// RecordSample adds a reviewed code suggestion with developer feedback.
func (b *FineTuningDatasetBuilder) RecordSample(sample ReviewSuggestionTrainingSample) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.samples = append(b.samples, sample)
}

// ExportJSONL filters accepted samples and writes OpenAI-compatible fine-tuning JSONL to a writer.
func (b *FineTuningDatasetBuilder) ExportJSONL(w io.Writer) (int, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	count := 0
	for _, s := range b.samples {
		// Only train models on accepted suggestions
		if s.Status != FeedbackAccepted {
			continue
		}

		entry := ChatFineTuningEntry{
			Messages: []OpenAIMessage{
				{Role: "system", Content: s.SystemPrompt},
				{Role: "user", Content: fmt.Sprintf("Review the following code diff:\n\n%s", s.InputCodeDiff)},
				{Role: "assistant", Content: s.ExpectedSuggestion},
			},
		}

		lineBytes, err := json.Marshal(entry)
		if err != nil {
			return count, fmt.Errorf("failed serializing fine-tuning entry: %w", err)
		}

		if _, err := w.Write(append(lineBytes, '\n')); err != nil {
			return count, fmt.Errorf("failed writing JSONL: %w", err)
		}
		count++
	}

	return count, nil
}
