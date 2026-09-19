// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"strings"
	"testing"
	"time"
)

func TestReadStreamPayload_NonEmpty(t *testing.T) {
	input := "diff --git a/main.go b/main.go\n+ func main() {}"
	r := strings.NewReader(input)

	got, err := ReadStreamPayload(r, ReadStreamPayloadOptions{
		NoDataTimeout:       100 * time.Millisecond,
		BrokenStreamTimeout: 500 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("ReadStreamPayload failed: %v", err)
	}
	if got != input {
		t.Errorf("expected %q, got %q", input, got)
	}
}

func TestReadStreamPayload_Empty(t *testing.T) {
	r := strings.NewReader("")

	got, err := ReadStreamPayload(r, ReadStreamPayloadOptions{
		NoDataTimeout:       50 * time.Millisecond,
		BrokenStreamTimeout: 200 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("ReadStreamPayload failed: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}
