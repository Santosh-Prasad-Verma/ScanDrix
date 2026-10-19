package storage

import (
	"context"
	"fmt"
	"io"
)

// Driver defines the contract for object storage operations.
type Driver interface {
	UploadArtifact(ctx context.Context, bucketID, fileID, filename string, data io.Reader) (string, error)
	DownloadArtifact(ctx context.Context, bucketID, fileID string) ([]byte, error)
}

// DualRailStorage wraps a primary storage driver (e.g. Appwrite) and automatically
// fails over to a secondary storage driver (e.g. Supabase S3) if the primary fails.
type DualRailStorage struct {
	Primary  Driver
	Fallback Driver
}

// NewDualRailStorage creates a dual-rail storage instance.
func NewDualRailStorage(primary, fallback Driver) *DualRailStorage {
	return &DualRailStorage{
		Primary:  primary,
		Fallback: fallback,
	}
}

// UploadArtifact attempts to upload to the primary driver, failing over to fallback on error.
func (d *DualRailStorage) UploadArtifact(ctx context.Context, bucketID, fileID, filename string, data io.Reader) (string, error) {
	if d.Primary == nil && d.Fallback == nil {
		return "", fmt.Errorf("no storage driver configured")
	}
	if d.Primary == nil {
		return d.Fallback.UploadArtifact(ctx, bucketID, fileID, filename, data)
	}

	uploadedID, err := d.Primary.UploadArtifact(ctx, bucketID, fileID, filename, data)
	if err == nil {
		return uploadedID, nil
	}

	if d.Fallback != nil {
		return d.Fallback.UploadArtifact(ctx, bucketID, fileID, filename, data)
	}

	return "", err
}

// DownloadArtifact attempts to download from primary, failing over to fallback on error.
func (d *DualRailStorage) DownloadArtifact(ctx context.Context, bucketID, fileID string) ([]byte, error) {
	if d.Primary == nil && d.Fallback == nil {
		return nil, fmt.Errorf("no storage driver configured")
	}
	if d.Primary == nil {
		return d.Fallback.DownloadArtifact(ctx, bucketID, fileID)
	}

	payload, err := d.Primary.DownloadArtifact(ctx, bucketID, fileID)
	if err == nil {
		return payload, nil
	}

	if d.Fallback != nil {
		return d.Fallback.DownloadArtifact(ctx, bucketID, fileID)
	}

	return nil, err
}
