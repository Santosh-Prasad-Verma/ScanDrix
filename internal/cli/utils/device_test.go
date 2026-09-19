// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetDeviceIdentity(t *testing.T) {
	tempHome := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tempHome)

	// Reset cached state
	deviceMu.Lock()
	cachedDevice = nil
	deviceMu.Unlock()

	id1, err := GetDeviceIdentity()
	if err != nil {
		t.Fatalf("GetDeviceIdentity() failed: %v", err)
	}
	if id1.DeviceID == "" {
		t.Fatal("expected non-empty DeviceID")
	}

	// Verify file was written with 0600 permissions
	path := filepath.Join(tempHome, ".scandrix", "device.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat failed on %s: %v", path, err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected 0600 permissions, got %v", info.Mode().Perm())
	}

	// Verify idempotency
	id2, err := GetDeviceIdentity()
	if err != nil {
		t.Fatalf("GetDeviceIdentity() second call failed: %v", err)
	}
	if id1.DeviceID != id2.DeviceID {
		t.Errorf("expected identical DeviceID, got %s and %s", id1.DeviceID, id2.DeviceID)
	}

	// Test UpdateDeviceToken
	testToken := "scandrix_dev_token_12345"
	if err := UpdateDeviceToken(testToken); err != nil {
		t.Fatalf("UpdateDeviceToken() failed: %v", err)
	}

	id3, err := GetDeviceIdentity()
	if err != nil {
		t.Fatalf("GetDeviceIdentity() after update failed: %v", err)
	}
	if id3.DeviceToken != testToken {
		t.Errorf("expected token %s, got %s", testToken, id3.DeviceToken)
	}
}
