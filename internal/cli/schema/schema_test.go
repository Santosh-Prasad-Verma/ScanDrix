// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package schema_test

import (
	"testing"

	"github.com/scandrix/backend/internal/cli/schema"
)

func TestGetMasterSchema(t *testing.T) {
	s := schema.GetMasterSchema()
	if s.Name != "scandrix" {
		t.Fatalf("expected root name scandrix, got %s", s.Name)
	}

	if len(s.Subcommands) == 0 {
		t.Fatal("expected subcommands in master schema")
	}

	foundReview := false
	foundSkills := false
	foundTrace := false
	for _, sub := range s.Subcommands {
		if sub.Name == "review" {
			foundReview = true
		}
		if sub.Name == "skills" {
			foundSkills = true
		}
		if sub.Name == "trace" {
			foundTrace = true
		}
	}

	if !foundReview || !foundSkills || !foundTrace {
		t.Fatalf("missing expected subcommands in schema: review=%v, skills=%v, trace=%v", foundReview, foundSkills, foundTrace)
	}
}
