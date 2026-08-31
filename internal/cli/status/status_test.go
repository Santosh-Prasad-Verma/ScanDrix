// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package status_test

import (
	"testing"

	"github.com/scandrix/backend/internal/cli/status"
)

func TestGetStatus(t *testing.T) {
	st, err := status.GetStatus(".")
	if err != nil {
		t.Fatalf("failed getting status: %v", err)
	}

	if st.Version == "" {
		t.Fatal("expected non-empty version")
	}

	if st.BundledSkills <= 0 {
		t.Fatalf("expected bundled skills count > 0, got %d", st.BundledSkills)
	}

	if err := status.PrintStatus("."); err != nil {
		t.Fatalf("failed printing status: %v", err)
	}
}
