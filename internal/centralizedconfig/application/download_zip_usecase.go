package application

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
)

// FullDownloadZipUseCase generates a ZIP archive of all virtual configuration and rule files with inheritance and scoping.
type FullDownloadZipUseCase struct {
	downloadUC *FullDownloadUseCase
}

// NewFullDownloadZipUseCase constructs a new FullDownloadZipUseCase.
func NewFullDownloadZipUseCase(downloadUC *FullDownloadUseCase) *FullDownloadZipUseCase {
	return &FullDownloadZipUseCase{downloadUC: downloadUC}
}

// Execute returns a byte slice containing the complete ZIP archive.
func (uc *FullDownloadZipUseCase) Execute(ctx context.Context, orgID, teamID string, opts DownloadOptions) ([]byte, error) {
	entries, err := uc.downloadUC.Execute(ctx, orgID, teamID, opts)
	if err != nil {
		return nil, fmt.Errorf("download zip: failed fetching entries: %w", err)
	}

	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	for _, entry := range entries {
		w, err := zipWriter.Create(entry.Path)
		if err != nil {
			return nil, fmt.Errorf("download zip: failed creating zip entry %q: %w", entry.Path, err)
		}
		if _, err := io.WriteString(w, entry.Content); err != nil {
			return nil, fmt.Errorf("download zip: failed writing zip entry %q: %w", entry.Path, err)
		}
	}

	if err := zipWriter.Close(); err != nil {
		return nil, fmt.Errorf("download zip: failed closing zip writer: %w", err)
	}

	return buf.Bytes(), nil
}
