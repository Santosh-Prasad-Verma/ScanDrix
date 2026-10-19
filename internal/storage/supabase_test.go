package storage

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSupabaseStorageDriver_UploadAndDownload(t *testing.T) {
	storedFiles := make(map[string][]byte)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		if r.Method == http.MethodPost && r.URL.Path == "/storage/v1/bucket" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"message":"bucket created"}`))
			return
		}

		if r.Method == http.MethodPost {
			// Upload object
			buf := new(bytes.Buffer)
			buf.ReadFrom(r.Body)
			storedFiles[r.URL.Path] = buf.Bytes()
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"Key":"uploaded"}`))
			return
		}

		if r.Method == http.MethodGet {
			// Download object
			data, ok := storedFiles[r.URL.Path]
			if !ok {
				// try mapping authenticated path to upload path
				uploadPath := bytes.Replace([]byte(r.URL.Path), []byte("/storage/v1/object/authenticated/"), []byte("/storage/v1/object/"), 1)
				data, ok = storedFiles[string(uploadPath)]
			}
			if !ok {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write(data)
			return
		}

		http.Error(w, "unsupported method", http.StatusBadRequest)
	}))
	defer ts.Close()

	driver := NewSupabaseStorageDriver(ts.URL, "test-key")

	ctx := context.Background()
	payload := []byte("diff --git a/main.go b/main.go\n+package main")

	// 1. Upload
	uploadedID, err := driver.UploadArtifact(ctx, "test-bucket", "artifacts/diff-01.patch", "diff.patch", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("unexpected upload error: %v", err)
	}
	if uploadedID != "artifacts/diff-01.patch" {
		t.Errorf("expected uploadedID 'artifacts/diff-01.patch', got %q", uploadedID)
	}

	// 2. Download
	downloaded, err := driver.DownloadArtifact(ctx, "test-bucket", "artifacts/diff-01.patch")
	if err != nil {
		t.Fatalf("unexpected download error: %v", err)
	}
	if string(downloaded) != string(payload) {
		t.Errorf("expected downloaded content %q, got %q", string(payload), string(downloaded))
	}
}

func TestSupabaseStorageDriver_AutoEnsureBucketOn404(t *testing.T) {
	bucketCreated := false
	uploadAttempts := 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/storage/v1/bucket" {
			bucketCreated = true
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"id":"auto-bucket"}`))
			return
		}

		if r.Method == http.MethodPost {
			uploadAttempts++
			if !bucketCreated {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"statusCode":"404","error":"Bucket not found","message":"Bucket not found"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"Key":"auto-bucket/file.txt"}`))
			return
		}
	}))
	defer ts.Close()

	driver := NewSupabaseStorageDriver(ts.URL, "test-key")
	ctx := context.Background()

	id, err := driver.UploadArtifact(ctx, "auto-bucket", "file.txt", "file.txt", bytes.NewReader([]byte("auto content")))
	if err != nil {
		t.Fatalf("expected successful upload after auto-bucket creation, got: %v", err)
	}
	if id != "file.txt" {
		t.Errorf("expected id 'file.txt', got %q", id)
	}
	if !bucketCreated {
		t.Errorf("expected bucket to be automatically created")
	}
	if uploadAttempts != 2 {
		t.Errorf("expected 2 upload attempts (1 failed with 404, 1 succeeded), got %d", uploadAttempts)
	}
}
