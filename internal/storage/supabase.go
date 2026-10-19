package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"
)

// SupabaseStorageDriver interacts with Supabase Storage REST API as a resilient primary or fallback driver.
type SupabaseStorageDriver struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewSupabaseStorageDriver initializes a production-grade Supabase Storage driver.
func NewSupabaseStorageDriver(baseURL, apiKey string) *SupabaseStorageDriver {
	cleanURL := strings.TrimRight(baseURL, "/")
	return &SupabaseStorageDriver{
		baseURL: cleanURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
}

// EnsureBucket verifies that the given bucket exists, creating it as a private bucket if missing.
func (s *SupabaseStorageDriver) EnsureBucket(ctx context.Context, bucketID string) error {
	if s.baseURL == "" || s.apiKey == "" {
		return fmt.Errorf("supabase storage not configured")
	}

	url := fmt.Sprintf("%s/storage/v1/bucket", s.baseURL)
	payload := fmt.Sprintf(`{"id":"%s","name":"%s","public":false}`, bucketID, bucketID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed creating bucket request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("apiKey", s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("supabase ensure bucket failed: %w", err)
	}
	defer resp.Body.Close()

	// 200/201 = created, 400/409 = already exists (both valid in production)
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated ||
		resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusConflict {
		return nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("failed creating supabase bucket (HTTP %d): %s", resp.StatusCode, string(respBody))
}

// UploadArtifact stores an artifact payload into the Supabase storage bucket with idempotency (x-upsert).
func (s *SupabaseStorageDriver) UploadArtifact(ctx context.Context, bucketID, fileID, filename string, data io.Reader) (string, error) {
	if s.baseURL == "" || s.apiKey == "" {
		return "", fmt.Errorf("supabase storage credentials not configured")
	}

	buf, err := io.ReadAll(data)
	if err != nil {
		return "", fmt.Errorf("failed reading artifact data: %w", err)
	}

	cleanFileID := strings.TrimPrefix(path.Clean("/"+fileID), "/")
	url := fmt.Sprintf("%s/storage/v1/object/%s/%s", s.baseURL, bucketID, cleanFileID)

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(100*(1<<attempt)) * time.Millisecond):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
		if err != nil {
			return "", fmt.Errorf("failed creating upload request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+s.apiKey)
		req.Header.Set("apiKey", s.apiKey)
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("x-upsert", "true")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("supabase upload connection error: %w", err)
			continue
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
			return cleanFileID, nil
		}

		// If bucket was missing, try creating it and retry
		if resp.StatusCode == http.StatusNotFound && strings.Contains(string(respBody), "Bucket not found") {
			if bErr := s.EnsureBucket(ctx, bucketID); bErr == nil {
				continue
			}
		}

		// 4xx client errors (other than missing bucket) are permanent
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return "", fmt.Errorf("supabase storage client error (HTTP %d): %s", resp.StatusCode, string(respBody))
		}

		lastErr = fmt.Errorf("supabase storage server error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	return "", fmt.Errorf("supabase storage upload exhausted retries: %w", lastErr)
}

// DownloadArtifact retrieves file bytes from a Supabase storage bucket.
func (s *SupabaseStorageDriver) DownloadArtifact(ctx context.Context, bucketID, fileID string) ([]byte, error) {
	if s.baseURL == "" || s.apiKey == "" {
		return nil, fmt.Errorf("supabase storage credentials not configured")
	}

	cleanFileID := strings.TrimPrefix(path.Clean("/"+fileID), "/")
	url := fmt.Sprintf("%s/storage/v1/object/authenticated/%s/%s", s.baseURL, bucketID, cleanFileID)

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(100*(1<<attempt)) * time.Millisecond):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed creating download request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+s.apiKey)
		req.Header.Set("apiKey", s.apiKey)

		resp, err := s.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("supabase download connection error: %w", err)
			continue
		}

		if resp.StatusCode == http.StatusOK {
			data, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr != nil {
				return nil, fmt.Errorf("failed reading download body: %w", readErr)
			}
			return data, nil
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("artifact not found (HTTP 404): %s", cleanFileID)
		}

		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return nil, fmt.Errorf("supabase storage download client error (HTTP %d): %s", resp.StatusCode, string(respBody))
		}

		lastErr = fmt.Errorf("supabase storage download server error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	return nil, fmt.Errorf("supabase storage download exhausted retries: %w", lastErr)
}
