// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the Apache License, Version 2.0 (the "License").

package scan

import (
	"path/filepath"
	"sort"
)

// PartitionIntoBatches groups scanned files by directory/subsystem while strictly respecting the token budget.
func PartitionIntoBatches(files []ScannedFile, tokenBudget int) []Batch {
	if len(files) == 0 {
		return nil
	}
	if tokenBudget <= 0 {
		tokenBudget = 32000
	}

	// Sort files by directory to keep related code together
	sort.Slice(files, func(i, j int) bool {
		dirI := filepath.Dir(files[i].RelativePath)
		dirJ := filepath.Dir(files[j].RelativePath)
		if dirI == dirJ {
			return files[i].RelativePath < files[j].RelativePath
		}
		return dirI < dirJ
	})

	var batches []Batch
	var currentBatchFiles []ScannedFile
	currentTokens := 0
	batchID := 1

	for _, f := range files {
		// If a single file exceeds budget by itself, give it an isolated batch
		if f.EstimatedTokens >= tokenBudget {
			if len(currentBatchFiles) > 0 {
				batches = append(batches, Batch{
					BatchID:          batchID,
					Files:            currentBatchFiles,
					TotalTokens:      currentTokens,
					SubsystemContext: deriveSubsystemContext(currentBatchFiles),
				})
				batchID++
				currentBatchFiles = nil
				currentTokens = 0
			}

			batches = append(batches, Batch{
				BatchID:          batchID,
				Files:            []ScannedFile{f},
				TotalTokens:      f.EstimatedTokens,
				SubsystemContext: filepath.Dir(f.RelativePath),
			})
			batchID++
			continue
		}

		// If adding this file exceeds the budget, flush current batch
		if currentTokens+f.EstimatedTokens > tokenBudget && len(currentBatchFiles) > 0 {
			batches = append(batches, Batch{
				BatchID:          batchID,
				Files:            currentBatchFiles,
				TotalTokens:      currentTokens,
				SubsystemContext: deriveSubsystemContext(currentBatchFiles),
			})
			batchID++
			currentBatchFiles = nil
			currentTokens = 0
		}

		currentBatchFiles = append(currentBatchFiles, f)
		currentTokens += f.EstimatedTokens
	}

	if len(currentBatchFiles) > 0 {
		batches = append(batches, Batch{
			BatchID:          batchID,
			Files:            currentBatchFiles,
			TotalTokens:      currentTokens,
			SubsystemContext: deriveSubsystemContext(currentBatchFiles),
		})
	}

	return batches
}

func deriveSubsystemContext(files []ScannedFile) string {
	if len(files) == 0 {
		return ""
	}
	commonDir := filepath.Dir(files[0].RelativePath)
	return filepath.ToSlash(commonDir)
}
