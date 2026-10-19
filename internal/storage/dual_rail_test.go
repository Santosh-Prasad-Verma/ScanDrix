package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

type mockDriver struct {
	uploadErr   error
	downloadErr error
	uploadedID  string
	downloadRes []byte
	calls       int
}

func (m *mockDriver) UploadArtifact(ctx context.Context, bucketID, fileID, filename string, data io.Reader) (string, error) {
	m.calls++
	if m.uploadErr != nil {
		return "", m.uploadErr
	}
	return m.uploadedID, nil
}

func (m *mockDriver) DownloadArtifact(ctx context.Context, bucketID, fileID string) ([]byte, error) {
	m.calls++
	if m.downloadErr != nil {
		return nil, m.downloadErr
	}
	return m.downloadRes, nil
}

func TestDualRailStorage_Upload_PrimarySuccess(t *testing.T) {
	primary := &mockDriver{uploadedID: "file-appwrite-123"}
	fallback := &mockDriver{uploadedID: "file-supabase-123"}

	dual := NewDualRailStorage(primary, fallback)

	data := bytes.NewReader([]byte("test diff content"))
	id, err := dual.UploadArtifact(context.Background(), "bucket-1", "file-123", "diff.patch", data)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if id != "file-appwrite-123" {
		t.Errorf("expected id 'file-appwrite-123', got %s", id)
	}
	if primary.calls != 1 {
		t.Errorf("expected primary calls == 1, got %d", primary.calls)
	}
	if fallback.calls != 0 {
		t.Errorf("expected fallback calls == 0, got %d", fallback.calls)
	}
}

func TestDualRailStorage_Upload_FallbackOnPrimaryFailure(t *testing.T) {
	primary := &mockDriver{uploadErr: errors.New("appwrite 403 quota exceeded")}
	fallback := &mockDriver{uploadedID: "file-supabase-456"}

	dual := NewDualRailStorage(primary, fallback)

	data := bytes.NewReader([]byte("test diff content"))
	id, err := dual.UploadArtifact(context.Background(), "bucket-1", "file-456", "diff.patch", data)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if id != "file-supabase-456" {
		t.Errorf("expected id 'file-supabase-456', got %s", id)
	}
	if primary.calls != 1 {
		t.Errorf("expected primary calls == 1, got %d", primary.calls)
	}
	if fallback.calls != 1 {
		t.Errorf("expected fallback calls == 1, got %d", fallback.calls)
	}
}

func TestDualRailStorage_Download_Fallback(t *testing.T) {
	primary := &mockDriver{downloadErr: errors.New("appwrite timeout")}
	fallback := &mockDriver{downloadRes: []byte("diff data from supabase")}

	dual := NewDualRailStorage(primary, fallback)

	res, err := dual.DownloadArtifact(context.Background(), "bucket-1", "file-789")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if string(res) != "diff data from supabase" {
		t.Errorf("unexpected content: %s", string(res))
	}
}
