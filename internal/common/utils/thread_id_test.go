package utils

import (
	"strings"
	"testing"
)

func TestThreadID(t *testing.T) {
	ids := ThreadIdentifiers{
		"repo": "backend",
		"pr":   42,
	}

	thread, err := CreateThreadID(ids, ThreadOptions{Prefix: "PR"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(thread.ID, "TR-PR-") {
		t.Errorf("unexpected thread id format: %s", thread.ID)
	}
	if len(thread.ID) > 32 {
		t.Errorf("thread id exceeds 32 chars: %d", len(thread.ID))
	}
	if thread.Metadata["repo"] != "backend" || thread.Metadata["pr"] != 42 {
		t.Errorf("metadata missing identifiers: %+v", thread.Metadata)
	}

	// Determinism test
	thread2, _ := CreateThreadID(ids, ThreadOptions{Prefix: "PR"})
	if thread.ID != thread2.ID {
		t.Errorf("thread ID generation is not deterministic")
	}

	// Validation tests: empty
	_, err = CreateThreadID(ThreadIdentifiers{}, ThreadOptions{})
	if err == nil {
		t.Errorf("expected error on empty identifiers")
	}

	// Validation tests: too long prefix
	_, err = CreateThreadID(ids, ThreadOptions{Prefix: "LONG"})
	if err == nil {
		t.Errorf("expected error on prefix > 3 chars")
	}
}
