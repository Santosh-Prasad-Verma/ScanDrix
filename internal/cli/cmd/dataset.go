// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/cli/services/api"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/spf13/cobra"
)

var (
	datasetFormat    string
	datasetOutput    string
	datasetWorkspace string
	datasetLimit     int
)

var datasetCmd = &cobra.Command{
	Use:   "dataset",
	Short: "Export and manage LLM fine-tuning datasets from human feedback",
	Long: `Export high-fidelity training datasets (JSONL) generated from real developer
feedback, accepted committable suggestions, and resolved review issues. Supports OpenAI Chat,
Completion, and Direct Preference Optimization (DPO) formats for LoRA / QLoRA training.`,
}

var datasetExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export instruction-tuning or DPO dataset in JSONL format",
	Example: `  scandrix dataset export --output=lora.jsonl
  scandrix dataset export --format=dpo --output=dpo_pairs.jsonl
  scandrix dataset export --format=chat --limit=500`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := api.NewClient("", "", "")

		path := "/api/v1/finetuning/export"
		if datasetWorkspace != "" {
			path = fmt.Sprintf("/api/v1/finetuning/%s/export", datasetWorkspace)
		}

		queryParts := []string{}
		if datasetFormat != "" {
			queryParts = append(queryParts, fmt.Sprintf("format=%s", datasetFormat))
		}
		if datasetLimit > 0 {
			queryParts = append(queryParts, fmt.Sprintf("limit=%d", datasetLimit))
		}
		if len(queryParts) > 0 {
			path += "?" + strings.Join(queryParts, "&")
		}

		ctx := cmd.Context()
		status, rawBytes, err := client.DoRaw(ctx, http.MethodGet, path, nil, nil)
		if err != nil {
			return utils.NewCommandError("API_ERROR", fmt.Sprintf("Failed exporting fine-tuning dataset: %v", err))
		}

		if status != http.StatusOK {
			return utils.NewCommandError("EXPORT_FAILED", fmt.Sprintf("API returned status %d: %s", status, string(rawBytes)))
		}

		lines := strings.Split(strings.TrimSpace(string(rawBytes)), "\n")
		sampleCount := 0
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				sampleCount++
			}
		}

		var out io.Writer = os.Stdout
		if datasetOutput != "" {
			f, err := os.Create(datasetOutput)
			if err != nil {
				return utils.NewCommandError("IO_ERROR", fmt.Sprintf("Failed to create output file: %v", err))
			}
			defer f.Close()
			out = f
		}

		if _, err := out.Write(rawBytes); err != nil {
			return utils.NewCommandError("IO_ERROR", fmt.Sprintf("Failed writing dataset: %v", err))
		}

		if agentFlag {
			env := utils.BuildAgentSuccessEnvelope("dataset.export", map[string]interface{}{
				"format":       datasetFormat,
				"sample_count": sampleCount,
				"output_file":  datasetOutput,
				"bytes":        len(rawBytes),
			}, time.Now())
			return utils.EmitAgentEnvelope(env, "")
		}

		if datasetOutput != "" {
			utils.Success("Successfully exported %d fine-tuning samples (%d bytes) in %s format to %s",
				sampleCount, len(rawBytes), strings.ToUpper(datasetFormat), datasetOutput)
		}

		return nil
	},
}

func init() {
	datasetExportCmd.Flags().StringVarP(&datasetFormat, "format", "f", "chat", "Format of dataset: chat, dpo, completion")
	datasetExportCmd.Flags().StringVarP(&datasetOutput, "output", "o", "", "Output file path (default stdout)")
	datasetExportCmd.Flags().StringVarP(&datasetWorkspace, "workspace", "w", "", "Workspace ID (defaults to active workspace)")
	datasetExportCmd.Flags().IntVarP(&datasetLimit, "limit", "l", 0, "Maximum number of samples to export (0 = unlimited)")

	datasetCmd.AddCommand(datasetExportCmd)
}
