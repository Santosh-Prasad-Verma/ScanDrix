// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package formatters

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"

	"github.com/scandrix/backend/internal/cli/types"
)

// FormatCSV serializes review issues into standard comma-separated values (CSV).
func FormatCSV(w io.Writer, result *types.ReviewResult) error {
	if result == nil {
		result = &types.ReviewResult{}
	}

	writer := csv.NewWriter(w)
	defer writer.Flush()

	header := []string{
		"ID",
		"Severity",
		"Category",
		"File",
		"Line",
		"EndLine",
		"Message",
		"Recommendation",
		"Suggestion",
		"Fixable",
	}
	if err := writer.Write(header); err != nil {
		return err
	}

	for _, issue := range result.Issues {
		record := []string{
			issue.ID,
			string(issue.Severity),
			issue.Category,
			issue.File,
			strconv.Itoa(issue.Line),
			strconv.Itoa(issue.EndLine),
			issue.Message,
			issue.Recommendation,
			issue.Suggestion,
			fmt.Sprintf("%t", issue.Fixable),
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}
