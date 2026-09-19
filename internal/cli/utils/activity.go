// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ACTIVITY AUDIT TRAIL DATA STRUCTURES

// ActivityEntry records a command invocation.
type ActivityEntry struct {
	Command   string    `json:"command"`
	Timestamp time.Time `json:"timestamp"`
}

// ActivityLog stores the history of recent CLI commands.
type ActivityLog struct {
	Recent []ActivityEntry `json:"recent"`
}

// PERSISTENCE & HISTORY TRUNCATION (0600 File Permissions)

// ActivityPath returns the path to ~/.scandrix/activity.json.
func ActivityPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".scandrix", "activity.json")
}

var sensitiveFlags = map[string]bool{
	"--key":           true,
	"--team-key":      true,
	"--token":         true,
	"--access-token":  true,
	"--refresh-token": true,
	"--password":      true,
	"--github-pat":    true,
}

// SanitizeCommandLine replaces sensitive credentials with [REDACTED].
func SanitizeCommandLine(cmdLine string) string {
	parts := strings.Fields(cmdLine)
	if len(parts) == 0 {
		return ""
	}

	sanitized := make([]string, 0, len(parts))
	for i := 0; i < len(parts); i++ {
		part := parts[i]
		if strings.HasPrefix(part, "--") {
			subParts := strings.SplitN(part, "=", 2)
			flagName := subParts[0]
			if sensitiveFlags[flagName] {
				if len(subParts) == 2 {
					sanitized = append(sanitized, flagName+"=[REDACTED]")
				} else {
					sanitized = append(sanitized, flagName)
					if i+1 < len(parts) && !strings.HasPrefix(parts[i+1], "-") {
						sanitized = append(sanitized, "[REDACTED]")
						i++
					}
				}
				continue
			}
		}
		sanitized = append(sanitized, part)
	}
	return strings.Join(sanitized, " ")
}

// RecordRecentActivity records a CLI command run in the recent activity log.
func RecordRecentActivity(cmdLine string) error {
	path := ActivityPath()
	_ = os.MkdirAll(filepath.Dir(path), 0700)

	var log ActivityLog
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &log)
	}

	entry := ActivityEntry{
		Command:   SanitizeCommandLine(cmdLine),
		Timestamp: time.Now().UTC(),
	}

	// Keep last 50 entries
	log.Recent = append([]ActivityEntry{entry}, log.Recent...)
	if len(log.Recent) > 50 {
		log.Recent = log.Recent[:50]
	}

	data, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// FormatRelativeTime converts a time.Time into a human-readable duration like '10s ago'.
func FormatRelativeTime(t time.Time, now ...time.Time) string {
	curr := time.Now().UTC()
	if len(now) > 0 {
		curr = now[0]
	}

	diff := curr.Sub(t)
	if diff < 0 {
		diff = 0
	}

	if diff < time.Minute {
		return fmt.Sprintf("%ds ago", int(diff.Seconds()))
	}
	if diff < time.Hour {
		return fmt.Sprintf("%dm ago", int(diff.Minutes()))
	}
	if diff < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(diff.Hours()/24))
}

// GetRecentActivityLines loads formatted activity lines for dashboard/status display.
func GetRecentActivityLines(maxItems int) ([]string, error) {
	path := ActivityPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{"No recent activity yet"}, nil
		}
		return nil, err
	}

	var log ActivityLog
	if err := json.Unmarshal(data, &log); err != nil {
		return []string{"No recent activity yet"}, nil
	}

	if len(log.Recent) == 0 {
		return []string{"No recent activity yet"}, nil
	}

	limit := maxItems
	if limit <= 0 || limit > len(log.Recent) {
		limit = len(log.Recent)
	}

	lines := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		e := log.Recent[i]
		lines = append(lines, fmt.Sprintf("%s - %s", e.Command, FormatRelativeTime(e.Timestamp)))
	}

	return lines, nil
}
