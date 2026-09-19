// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
)

// TrialStatus models the rate-limit status for unauthenticated CLI reviews.
type TrialStatus struct {
	Fingerprint  string `json:"fingerprint,omitempty"`
	ReviewsUsed  int    `json:"reviewsUsed"`
	ReviewsLimit int    `json:"reviewsLimit"`
	FilesLimit   int    `json:"filesLimit,omitempty"`
	LinesLimit   int    `json:"linesLimit,omitempty"`
	ResetsAt     string `json:"resetsAt,omitempty"`
	IsLimited    bool   `json:"isLimited"`

	// Compatibility aliases
	Allowed       bool   `json:"allowed,omitempty"`
	Remaining     int    `json:"remaining,omitempty"`
	RemainingUses int    `json:"remaining_uses,omitempty"`
	ResetAt       string `json:"reset_at,omitempty"`
	UpgradeURL    string `json:"upgrade_url,omitempty"`
}

// UnmarshalJSON handles both camelCase and snake_case payloads.
func (t *TrialStatus) UnmarshalJSON(data []byte) error {
	type Alias TrialStatus
	aux := struct {
		*Alias
		SnakeReviewsUsed  *int   `json:"reviews_used"`
		SnakeReviewsLimit *int   `json:"reviews_limit"`
		SnakeFilesLimit   *int   `json:"files_limit"`
		SnakeLinesLimit   *int   `json:"lines_limit"`
		SnakeResetsAt     string `json:"reset_at"`
		SnakeIsLimited    *bool  `json:"is_limited"`
	}{
		Alias: (*Alias)(t),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if aux.SnakeReviewsUsed != nil && t.ReviewsUsed == 0 {
		t.ReviewsUsed = *aux.SnakeReviewsUsed
	}
	if aux.SnakeReviewsLimit != nil && t.ReviewsLimit == 0 {
		t.ReviewsLimit = *aux.SnakeReviewsLimit
	}
	if aux.SnakeFilesLimit != nil && t.FilesLimit == 0 {
		t.FilesLimit = *aux.SnakeFilesLimit
	}
	if aux.SnakeLinesLimit != nil && t.LinesLimit == 0 {
		t.LinesLimit = *aux.SnakeLinesLimit
	}
	if aux.SnakeResetsAt != "" && t.ResetsAt == "" {
		t.ResetsAt = aux.SnakeResetsAt
	}
	if aux.SnakeIsLimited != nil {
		t.IsLimited = *aux.SnakeIsLimited
	}

	if t.ResetAt == "" && t.ResetsAt != "" {
		t.ResetAt = t.ResetsAt
	}
	if t.ResetsAt == "" && t.ResetAt != "" {
		t.ResetsAt = t.ResetAt
	}
	if t.ReviewsLimit > 0 && t.Remaining == 0 {
		rem := t.ReviewsLimit - t.ReviewsUsed
		if rem < 0 {
			rem = 0
		}
		t.Remaining = rem
		t.RemainingUses = rem
	}
	if !t.IsLimited && !t.Allowed && t.ReviewsLimit > 0 && t.ReviewsUsed >= t.ReviewsLimit {
		t.IsLimited = true
	}
	if t.IsLimited {
		t.Allowed = false
	} else {
		t.Allowed = true
	}
	return nil
}

// GetDeviceFingerprint returns a stable 32-character SHA-256 identifier for the current machine and repository.
func GetDeviceFingerprint() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown-host"
	}

	username := "unknown-user"
	if u, err := user.Current(); err == nil && u.Username != "" {
		username = u.Username
	} else if envUser := os.Getenv("USER"); envUser != "" {
		username = envUser
	} else if envUser := os.Getenv("USERNAME"); envUser != "" {
		username = envUser
	}

	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}

	raw := fmt.Sprintf("%s:%s:%s", hostname, username, cwd)
	h := sha256.Sum256([]byte(raw))
	hashHex := hex.EncodeToString(h[:])
	if len(hashHex) > 32 {
		return hashHex[:32]
	}
	return hashHex
}

// ShowTrialLimitPrompt displays a styled warning box when unauthenticated reviews reach the daily limit.
func ShowTrialLimitPrompt(status TrialStatus) {
	used := status.ReviewsUsed
	limit := status.ReviewsLimit
	if limit <= 0 {
		limit = 5
	}

	fmt.Println()
	fmt.Println("\033[33m╭──────────────────────────────────────────────────────────╮\033[0m")
	fmt.Printf("\033[33m│\033[0m  \033[1;33m⚡ Daily limit reached\033[0m (%d/%d reviews)                    \033[33m│\033[0m\n", used, limit)
	fmt.Println("\033[33m│\033[0m                                                          \033[33m│\033[0m")
	fmt.Println("\033[33m│\033[0m  Sign up for free to unlock:                             \033[33m│\033[0m")
	fmt.Println("\033[33m│\033[0m  \033[32m✓\033[0m Unlimited reviews                                      \033[33m│\033[0m")
	fmt.Println("\033[33m│\033[0m  \033[32m✓\033[0m Custom configurations                                  \033[33m│\033[0m")
	fmt.Println("\033[33m│\033[0m  \033[32m✓\033[0m Review history                                         \033[33m│\033[0m")
	fmt.Println("\033[33m│\033[0m  \033[32m✓\033[0m Team integration                                       \033[33m│\033[0m")
	fmt.Println("\033[33m│\033[0m                                                          \033[33m│\033[0m")
	fmt.Println("\033[33m│\033[0m  \033[90m→\033[0m \033[36mscandrix auth login\033[0m                                   \033[33m│\033[0m")
	fmt.Println("\033[33m╰──────────────────────────────────────────────────────────╯\033[0m")
	fmt.Println()
}

// FormatTrialCompletionMessage formats the status line for completed trial reviews.
func FormatTrialCompletionMessage(status *TrialStatus) string {
	if status == nil {
		return "Review complete! (Trial mode)"
	}
	if status.ReviewsLimit > 0 {
		return fmt.Sprintf("Review complete! (Trial: %d/%d reviews today)", status.ReviewsUsed, status.ReviewsLimit)
	}
	return "Review complete! (Trial mode)"
}
