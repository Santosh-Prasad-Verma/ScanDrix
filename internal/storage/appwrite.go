package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

// ArtifactClient interacts with Appwrite Storage buckets to archive review diffs and reports.
type ArtifactClient struct {
	endpoint   string
	projectID  string
	apiKey     string
	httpClient *http.Client
}

// NewArtifactClient initializes an Appwrite Storage client.
func NewArtifactClient(endpoint, projectID, apiKey string) *ArtifactClient {
	return &ArtifactClient{
		endpoint:  endpoint,
		projectID: projectID,
		apiKey:    apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// UploadArtifact stores a file payload into the specified Appwrite storage bucket.
func (c *ArtifactClient) UploadArtifact(ctx context.Context, bucketID, fileID, filename string, data io.Reader) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// fileId field
	if err := writer.WriteField("fileId", fileID); err != nil {
		return "", fmt.Errorf("failed writing fileId field: %w", err)
	}

	// file field
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("failed creating form file: %w", err)
	}

	if _, err := io.Copy(part, data); err != nil {
		return "", fmt.Errorf("failed copying file data: %w", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("failed closing multipart writer: %w", err)
	}

	url := fmt.Sprintf("%s/storage/buckets/%s/files", c.endpoint, bucketID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return "", fmt.Errorf("failed creating http request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Appwrite-Project", c.projectID)
	req.Header.Set("X-Appwrite-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("appwrite storage upload failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("appwrite storage error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	return fileID, nil
}

// DownloadArtifact retrieves file bytes from an Appwrite storage bucket.
func (c *ArtifactClient) DownloadArtifact(ctx context.Context, bucketID, fileID string) ([]byte, error) {
	url := fmt.Sprintf("%s/storage/buckets/%s/files/%s/download", c.endpoint, bucketID, fileID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed creating download request: %w", err)
	}

	req.Header.Set("X-Appwrite-Project", c.projectID)
	req.Header.Set("X-Appwrite-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("appwrite storage download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("appwrite download error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	return io.ReadAll(resp.Body)
}
