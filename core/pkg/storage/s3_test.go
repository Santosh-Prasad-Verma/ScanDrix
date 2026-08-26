package storage_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/codehound/codehound/core/pkg/config"
	"github.com/codehound/codehound/core/pkg/storage"
)

func TestS3ArtifactsStorageAndPresigning(t *testing.T) {
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := storage.NewS3Client(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to initialize s3 client: %v", err)
	}

	// 1. Ensure bucket exists
	if err := client.EnsureBucketExists(ctx); err != nil {
		t.Fatalf("failed to ensure bucket exists: %v", err)
	}

	// 2. Put Object (SARIF report payload)
	testKey := "test-reports/sarif-001.json"
	testContent := []byte(`{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"CodeHound"}}}]}`)
	uri, err := client.PutObject(ctx, testKey, testContent, "application/json")
	if err != nil {
		t.Fatalf("failed to put object to s3: %v", err)
	}

	if !strings.HasPrefix(uri, "s3://") {
		t.Fatalf("expected s3:// URI prefix, got: %s", uri)
	}

	// 3. Get Object
	data, err := client.GetObject(ctx, testKey)
	if err != nil {
		t.Fatalf("failed to get object from s3: %v", err)
	}

	if string(data) != string(testContent) {
		t.Fatalf("downloaded content mismatch: expected %s, got %s", string(testContent), string(data))
	}

	// 4. Generate Presigned URL
	presignedURL, err := client.GeneratePresignedDownloadURL(ctx, testKey, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate presigned download URL: %v", err)
	}

	if !strings.Contains(presignedURL, cfg.ArtifactsBucket) && !strings.Contains(presignedURL, testKey) {
		t.Fatalf("presigned URL does not contain bucket or key: %s", presignedURL)
	}
}
