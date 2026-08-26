package sast

import (
	"testing"

	"github.com/codehound/codehound/shared/domain"
	"github.com/google/uuid"
)

func TestSASTScanGoFile(t *testing.T) {
	engine := NewEngine()

	goCode := []byte(`package main

import (
	"crypto/md5"
	"database/sql"
	"fmt"
)

func Handler(db *sql.DB, user string) {
	// Vulnerability 1: SQL Injection
	query := fmt.Sprintf("SELECT * FROM users WHERE username = '%s'", user)
	db.Query(query)

	// Vulnerability 2: Weak Crypto
	h := md5.New()
	h.Write([]byte(user))
}
`)

	tenantID := uuid.New()
	projectID := uuid.New()
	scanID := uuid.New()

	findings, evidences := engine.ScanFile(tenantID, projectID, scanID, "server.go", "go", goCode)

	if len(findings) < 2 {
		t.Fatalf("expected at least 2 SAST findings, got %d", len(findings))
	}

	hasSQLi := false
	hasMD5 := false

	for _, f := range findings {
		if f.CWEID != nil && *f.CWEID == "CWE-89" {
			hasSQLi = true
			if f.Severity != domain.FindingSeverityCritical {
				t.Errorf("expected SQLi severity CRITICAL, got %s", f.Severity)
			}
		}
		if f.CWEID != nil && *f.CWEID == "CWE-327" {
			hasMD5 = true
			if f.Severity != domain.FindingSeverityHigh {
				t.Errorf("expected MD5 severity HIGH, got %s", f.Severity)
			}
		}
	}

	if !hasSQLi {
		t.Errorf("expected SQLi finding")
	}
	if !hasMD5 {
		t.Errorf("expected MD5 weak crypto finding")
	}
	if len(evidences) != len(findings) {
		t.Errorf("expected %d evidences, got %d", len(findings), len(evidences))
	}
}

func TestSASTScanTypeScriptFile(t *testing.T) {
	engine := NewEngine()

	tsCode := []byte(`import React from 'react';

export function UserProfile({ bio }: { bio: string }) {
  // Vulnerability: XSS via dangerouslySetInnerHTML
  return <div dangerouslySetInnerHTML={{ __html: bio }} />;
}
`)

	tenantID := uuid.New()
	projectID := uuid.New()
	scanID := uuid.New()

	findings, _ := engine.ScanFile(tenantID, projectID, scanID, "profile.tsx", "tsx", tsCode)

	if len(findings) == 0 {
		t.Fatalf("expected XSS finding in TypeScript/React code")
	}

	if *findings[0].CWEID != "CWE-79" {
		t.Errorf("expected CWE-79, got %s", *findings[0].CWEID)
	}
}

func TestParseSARIFOutput(t *testing.T) {
	sarifSample := []byte(`{
  "version": "2.1.0",
  "$schema": "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json",
  "runs": [
    {
      "tool": {
        "driver": {
          "name": "Semgrep OSS",
          "version": "1.80.0",
          "informationUri": "https://semgrep.dev"
        }
      },
      "results": [
        {
          "ruleId": "go.lang.security.audit.sqli",
          "level": "error",
          "message": {
            "text": "Detected SQL injection risk in query construction"
          },
          "locations": [
            {
              "physicalLocation": {
                "artifactLocation": {
                  "uri": "api/handlers/users.go"
                },
                "region": {
                  "startLine": 45
                }
              }
            }
          ]
        }
      ]
    }
  ]
}`)

	tenantID := uuid.New()
	projectID := uuid.New()
	scanID := uuid.New()

	findings, evidences, err := ParseSARIF(tenantID, projectID, scanID, sarifSample)
	if err != nil {
		t.Fatalf("failed to parse SARIF: %v", err)
	}

	if len(findings) != 1 {
		t.Fatalf("expected 1 finding from SARIF, got %d", len(findings))
	}

	f := findings[0]
	if f.PrimaryFile != "api/handlers/users.go" || f.PrimaryLine != 45 {
		t.Errorf("unexpected file/line: %s:%d", f.PrimaryFile, f.PrimaryLine)
	}
	if f.Severity != domain.FindingSeverityHigh {
		t.Errorf("expected HIGH severity for error level, got %s", f.Severity)
	}
	if len(evidences) != 1 {
		t.Errorf("expected 1 evidence item")
	}
}
