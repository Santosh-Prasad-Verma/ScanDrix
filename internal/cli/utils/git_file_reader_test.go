// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeRepoFileReader(t *testing.T) {
	tempDir := t.TempDir()

	// Create a text file
	textPath := filepath.Join(tempDir, "hello.txt")
	if err := os.WriteFile(textPath, []byte("hello world from scandrix\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a binary file (contains null byte)
	binPath := filepath.Join(tempDir, "data.bin")
	if err := os.WriteFile(binPath, []byte{0x00, 0x01, 0x02, 0xFF}, 0644); err != nil {
		t.Fatal(err)
	}

	reader, err := NewSafeRepoFileReader(tempDir)
	if err != nil {
		t.Fatalf("NewSafeRepoFileReader failed: %v", err)
	}

	// 1. Path traversal test
	_, err = reader.ValidateRelativePath("../escaped.txt")
	if err == nil {
		t.Error("expected error for path traversal escaping repo root")
	}

	// 2. Read text file
	content, err := reader.ReadFileContent("hello.txt")
	if err != nil {
		t.Fatalf("ReadFileContent(hello.txt) failed: %v", err)
	}
	if content != "hello world from scandrix\n" {
		t.Errorf("unexpected content: %q", content)
	}

	// 3. Binary detection
	isBin, err := reader.IsBinaryFile("data.bin")
	if err != nil {
		t.Fatalf("IsBinaryFile(data.bin) failed: %v", err)
	}
	if !isBin {
		t.Error("expected data.bin to be detected as binary")
	}

	// 4. Binary read rejection
	_, err = reader.ReadFileContent("data.bin")
	if err == nil {
		t.Error("expected ReadFileContent on binary file to return error")
	}
}

func TestIsBinaryContent(t *testing.T) {
	tests := []struct {
		data   []byte
		wantBin bool
	}{
		{[]byte("plain ascii string"), false},
		{[]byte("UTF-8 characters: Привет мир! 🚀"), false},
		{[]byte{0x48, 0x65, 0x6C, 0x6C, 0x6F, 0x00, 0x57}, true}, // Null byte
		{[]byte{0xFF, 0xFE, 0xFD}, true},                          // Invalid UTF-8 sequence
		{[]byte{}, false},
	}

	for i, tt := range tests {
		got := IsBinaryContent(tt.data)
		if got != tt.wantBin {
			t.Errorf("case %d: IsBinaryContent() = %v, want %v", i, got, tt.wantBin)
		}
	}
}
