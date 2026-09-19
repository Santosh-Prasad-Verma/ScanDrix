// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGetDeviceFingerprint(t *testing.T) {
	fp1 := GetDeviceFingerprint()
	fp2 := GetDeviceFingerprint()

	if len(fp1) != 32 {
		t.Fatalf("expected 32-character fingerprint, got %d: %q", len(fp1), fp1)
	}

	if fp1 != fp2 {
		t.Fatalf("fingerprint should be deterministic across consecutive calls: %s != %s", fp1, fp2)
	}
}

func TestFormatTrialCompletionMessage(t *testing.T) {
	msg1 := FormatTrialCompletionMessage(nil)
	if msg1 != "Review complete! (Trial mode)" {
		t.Errorf("unexpected message for nil status: %s", msg1)
	}

	msg2 := FormatTrialCompletionMessage(&TrialStatus{
		ReviewsUsed:  2,
		ReviewsLimit: 5,
	})
	if !strings.Contains(msg2, "2/5") {
		t.Errorf("expected 2/5 in message, got: %s", msg2)
	}
}

func TestTrialStatusUnmarshal(t *testing.T) {
	camelJSON := `{"fingerprint":"abc123","reviewsUsed":3,"reviewsLimit":5,"filesLimit":10,"linesLimit":1000,"resetsAt":"2026-09-13T00:00:00Z","isLimited":false}`
	var status1 TrialStatus
	if err := json.Unmarshal([]byte(camelJSON), &status1); err != nil {
		t.Fatalf("failed unmarshaling camelCase: %v", err)
	}
	if status1.ReviewsUsed != 3 || status1.ReviewsLimit != 5 || status1.FilesLimit != 10 || status1.IsLimited != false {
		t.Errorf("unexpected status from camelCase: %+v", status1)
	}

	snakeJSON := `{"reviews_used":4,"reviews_limit":5,"files_limit":8,"lines_limit":500,"reset_at":"2026-09-13T00:00:00Z","is_limited":true}`
	var status2 TrialStatus
	if err := json.Unmarshal([]byte(snakeJSON), &status2); err != nil {
		t.Fatalf("failed unmarshaling snake_case: %v", err)
	}
	if status2.ReviewsUsed != 4 || status2.ReviewsLimit != 5 || status2.FilesLimit != 8 || status2.IsLimited != true {
		t.Errorf("unexpected status from snake_case: %+v", status2)
	}
}

