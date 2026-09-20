// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// CREDENTIAL STORE DATA STRUCTURES

var credsMu sync.RWMutex

// UserProfile stores basic identity of the authenticated user.
type UserProfile struct {
	ID            string   `json:"id,omitempty"`
	Email         string   `json:"email,omitempty"`
	Name          string   `json:"name,omitempty"`
	Role          string   `json:"role,omitempty"`
	Workspace     string   `json:"workspace,omitempty"`
	Organizations []string `json:"orgs,omitempty"`
}

// StoredCredentials stores access tokens, refresh tokens, and user profile.
type StoredCredentials struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token,omitempty"`
	ExpiresAt    int64        `json:"expires_at"` // Epoch milliseconds
	User         *UserProfile `json:"user,omitempty"`
	TeamKey      string       `json:"team_key,omitempty"`
}

// CREDENTIAL DISK STORAGE (Restricted 0600 File Permissions)

// CredentialsPath returns the path to ~/.scandrix/credentials.json.
func CredentialsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".scandrix", "credentials.json")
}

// LoadCredentials loads stored credentials from ~/.scandrix/credentials.json.
func LoadCredentials() (*StoredCredentials, error) {
	credsMu.RLock()
	defer credsMu.RUnlock()

	path := CredentialsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var creds StoredCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, err
	}

	return &creds, nil
}

// SaveCredentials writes credentials securely with 0600 permissions.
func SaveCredentials(creds *StoredCredentials) error {
	credsMu.Lock()
	defer credsMu.Unlock()

	path := CredentialsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// ClearCredentials removes the credentials file.
func ClearCredentials() error {
	credsMu.Lock()
	defer credsMu.Unlock()

	path := CredentialsPath()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
