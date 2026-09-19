// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"strings"
	"testing"
)

func TestParseOptionalNumber(t *testing.T) {
	num, err := ParseOptionalNumber("", "--limit")
	if err != nil || num != nil {
		t.Fatalf("expected nil, nil for empty, got %v, %v", num, err)
	}

	num, err = ParseOptionalNumber("42", "--limit")
	if err != nil || num == nil || *num != 42 {
		t.Fatalf("expected 42, got %v, %v", num, err)
	}

	_, err = ParseOptionalNumber("abc", "--limit")
	if err == nil {
		t.Fatalf("expected error for non-number")
	}

	_, err = ParseOptionalNumber("-5", "--limit")
	if err == nil {
		t.Fatalf("expected error for <= 0")
	}
}

func TestParseCsvEnumList(t *testing.T) {
	list, err := ParseCsvEnumList("", "--format", []string{"json", "terminal"})
	if err != nil || list != nil {
		t.Fatalf("expected nil for empty, got %v, %v", list, err)
	}

	list, err = ParseCsvEnumList("json, terminal", "--format", []string{"json", "terminal", "sarif"})
	if err != nil || len(list) != 2 {
		t.Fatalf("expected 2 items, got %v, %v", list, err)
	}

	_, err = ParseCsvEnumList("json, xml", "--format", []string{"json", "terminal"})
	if err == nil {
		t.Fatalf("expected error for invalid enum value xml")
	}
}

func TestValidateHTTPURL(t *testing.T) {
	valid, err := ValidateHTTPURL("https://scandrix.dev/api", "--url")
	if err != nil || valid != "https://scandrix.dev/api" {
		t.Fatalf("expected valid url, got %v, %v", valid, err)
	}

	_, err = ValidateHTTPURL("ftp://scandrix.dev", "--url")
	if err == nil {
		t.Fatalf("expected error for non-http/https")
	}

	_, err = ValidateHTTPURL("not-a-url", "--url")
	if err == nil {
		t.Fatalf("expected error for malformed url")
	}
}

func TestParseFieldList(t *testing.T) {
	fields := ParseFieldList("a, b, c, a,  b ")
	if len(fields) != 3 {
		t.Fatalf("expected 3 unique fields, got %v", fields)
	}
	if fields[0] != "a" || fields[1] != "b" || fields[2] != "c" {
		t.Fatalf("unexpected order: %v", fields)
	}
}

func TestAssertStructuredOutputForFields(t *testing.T) {
	if err := AssertStructuredOutputForFields("", "terminal", false); err != nil {
		t.Fatalf("unexpected error when fields empty: %v", err)
	}

	if err := AssertStructuredOutputForFields("a,b", "json", false); err != nil {
		t.Fatalf("unexpected error with json format: %v", err)
	}

	if err := AssertStructuredOutputForFields("a,b", "terminal", true); err != nil {
		t.Fatalf("unexpected error in agent mode: %v", err)
	}

	if err := AssertStructuredOutputForFields("a,b", "terminal", false); err == nil {
		t.Fatalf("expected error when neither json nor agent")
	}
}

func TestResolveRemoteInstallInstructions(t *testing.T) {
	unix := ResolveRemoteInstallInstructions("linux")
	if !strings.Contains(unix.Primary, "curl -fsSL") {
		t.Errorf("expected curl in unix installer, got %s", unix.Primary)
	}

	win := ResolveRemoteInstallInstructions("windows")
	if !strings.Contains(win.Primary, "powershell") {
		t.Errorf("expected powershell in windows installer, got %s", win.Primary)
	}
}
