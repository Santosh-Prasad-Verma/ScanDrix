// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	uuidRegex    = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	deviceMu     sync.Mutex
	cachedDevice *DeviceData
)

// DeviceData stores local device identity and persistent token.
type DeviceData struct {
	DeviceID       string `json:"deviceId"`
	CreatedAt      string `json:"createdAt"`
	DeviceToken    string `json:"deviceToken,omitempty"`
	TokenUpdatedAt string `json:"tokenUpdatedAt,omitempty"`
}

// DeviceIdentity represents the public device identity tuple.
type DeviceIdentity struct {
	DeviceID    string `json:"deviceId"`
	DeviceToken string `json:"deviceToken,omitempty"`
}

func getScandrixDeviceFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".scandrix")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "device.json"), nil
}

func generateSecureUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant 10
	h := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32])
}

// ReadStoredDeviceData loads device metadata from ~/.scandrix/device.json.
func ReadStoredDeviceData() (*DeviceData, error) {
	path, err := getScandrixDeviceFilePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var d DeviceData
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, err
	}

	if !uuidRegex.MatchString(strings.ToLower(d.DeviceID)) {
		return nil, nil
	}

	return &d, nil
}

// WriteStoredDeviceData atomically saves device identity with POSIX 0600 permissions.
func WriteStoredDeviceData(d *DeviceData) error {
	path, err := getScandrixDeviceFilePath()
	if err != nil {
		return err
	}

	payload, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := fmt.Sprintf("%s.%d.%d.tmp", path, os.Getpid(), time.Now().UnixNano())
	if err := os.WriteFile(tmpFile, payload, 0600); err != nil {
		return err
	}

	return os.Rename(tmpFile, path)
}

// GetDeviceIdentity returns the active device UUID and cached token.
func GetDeviceIdentity() (*DeviceIdentity, error) {
	deviceMu.Lock()
	defer deviceMu.Unlock()

	if cachedDevice != nil {
		return &DeviceIdentity{
			DeviceID:    cachedDevice.DeviceID,
			DeviceToken: cachedDevice.DeviceToken,
		}, nil
	}

	stored, err := ReadStoredDeviceData()
	if err == nil && stored != nil {
		cachedDevice = stored
		return &DeviceIdentity{
			DeviceID:    stored.DeviceID,
			DeviceToken: stored.DeviceToken,
		}, nil
	}

	created := &DeviceData{
		DeviceID:  generateSecureUUID(),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	_ = WriteStoredDeviceData(created)
	cachedDevice = created

	return &DeviceIdentity{
		DeviceID:    created.DeviceID,
		DeviceToken: created.DeviceToken,
	}, nil
}

// GetOrCreateDeviceID retrieves or creates a persistent device UUID.
func GetOrCreateDeviceID() (string, error) {
	identity, err := GetDeviceIdentity()
	if err != nil {
		return "", err
	}
	return identity.DeviceID, nil
}

// UpdateDeviceToken updates and persists the session device token.
func UpdateDeviceToken(deviceToken string) error {
	token := strings.TrimSpace(deviceToken)
	if token == "" {
		return fmt.Errorf("device token cannot be empty")
	}

	deviceMu.Lock()
	defer deviceMu.Unlock()

	var current *DeviceData
	if cachedDevice != nil {
		current = cachedDevice
	} else {
		stored, _ := ReadStoredDeviceData()
		if stored != nil {
			current = stored
		} else {
			current = &DeviceData{
				DeviceID:  generateSecureUUID(),
				CreatedAt: time.Now().UTC().Format(time.RFC3339),
			}
		}
	}

	current.DeviceToken = token
	current.TokenUpdatedAt = time.Now().UTC().Format(time.RFC3339)
	cachedDevice = current

	return WriteStoredDeviceData(current)
}
