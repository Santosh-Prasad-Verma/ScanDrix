package finetuning

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/google/uuid"
)

type ExportFormat string

const (
	FormatChat       ExportFormat = "chat"
	FormatOpenAI     ExportFormat = "openai"
	FormatDPO        ExportFormat = "dpo"
	FormatCompletion ExportFormat = "completion"
)

type ExportOptions struct {
	WorkspaceID       uuid.UUID
	Format            ExportFormat
	IncludeDismissed  bool
	Limit             int
}

type DPOFineTuningEntry struct {
	Prompt   string `json:"prompt"`
	Chosen   string `json:"chosen"`
	Rejected string `json:"rejected"`
}

type CompletionFineTuningEntry struct {
	Prompt     string `json:"prompt"`
	Completion string `json:"completion"`
}

type FinetuningRepository interface {
	GetFineTuningSamples(ctx context.Context, workspaceID uuid.UUID, limit int) ([]ReviewSuggestionTrainingSample, error)
}

// ExportDataset queries the repository and writes formatted JSONL directly to the destination writer.
func ExportDataset(
	ctx context.Context,
	repo FinetuningRepository,
	opts ExportOptions,
	w io.Writer,
) (int, error) {
	if repo == nil {
		return 0, fmt.Errorf("finetuning repository is nil")
	}

	samples, err := repo.GetFineTuningSamples(ctx, opts.WorkspaceID, opts.Limit)
	if err != nil {
		return 0, err
	}

	encoder := json.NewEncoder(w)
	count := 0

	format := opts.Format
	if format == "" || format == FormatOpenAI {
		format = FormatChat
	}

	switch format {
	case FormatDPO:
		// Group samples into accepted (chosen) and rejected pairs
		var chosenList []ReviewSuggestionTrainingSample
		var rejectedList []ReviewSuggestionTrainingSample
		for _, s := range samples {
			if s.Status == FeedbackAccepted {
				chosenList = append(chosenList, s)
			} else if s.Status == FeedbackRejected {
				rejectedList = append(rejectedList, s)
			}
		}

		maxPairs := len(chosenList)
		if len(rejectedList) < maxPairs {
			maxPairs = len(rejectedList)
		}

		for i := 0; i < maxPairs; i++ {
			c := chosenList[i]
			r := rejectedList[i]
			dpoEntry := DPOFineTuningEntry{
				Prompt:   fmt.Sprintf("Review the following code diff for potential bugs or security defects:\n\n%s", c.InputCodeDiff),
				Chosen:   c.ExpectedSuggestion,
				Rejected: r.ExpectedSuggestion,
			}
			if err := encoder.Encode(dpoEntry); err != nil {
				return count, err
			}
			count++
		}

	case FormatCompletion:
		for _, s := range samples {
			if s.Status != FeedbackAccepted && !opts.IncludeDismissed {
				continue
			}
			entry := CompletionFineTuningEntry{
				Prompt:     fmt.Sprintf("%s\n\nCode Diff:\n%s\n\nRecommendation:\n", s.SystemPrompt, s.InputCodeDiff),
				Completion: s.ExpectedSuggestion,
			}
			if err := encoder.Encode(entry); err != nil {
				return count, err
			}
			count++
		}

	case FormatChat:
		fallthrough
	default:
		for _, s := range samples {
			if s.Status != FeedbackAccepted && !opts.IncludeDismissed {
				continue
			}
			entry := ChatFineTuningEntry{
				Messages: []OpenAIMessage{
					{Role: "system", Content: s.SystemPrompt},
					{Role: "user", Content: fmt.Sprintf("Analyze the following code diff and recommend necessary fixes:\n\n%s", s.InputCodeDiff)},
					{Role: "assistant", Content: s.ExpectedSuggestion},
				},
			}
			if err := encoder.Encode(entry); err != nil {
				return count, err
			}
			count++
		}
	}

	return count, nil
}
