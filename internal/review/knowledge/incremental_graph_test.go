// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package knowledge

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

func TestIncrementalGraphSynchronizer_Lifecycle(t *testing.T) {
	index := NewSymbolIndex()
	graph := NewCallGraph()
	syncEngine := NewIncrementalGraphSynchronizer(index, graph)

	// Baseline: 3 Go source files
	fileA := "pkg/auth/hash.go"
	codeA := `package auth

func HashPassword(raw string) string {
	return "hashed_" + raw
}
`

	fileB := "pkg/auth/login.go"
	codeB := `package auth

func AuthenticateUser(user string, pass string) bool {
	h := HashPassword(pass)
	return ValidateHash(h)
}

func ValidateHash(h string) bool {
	return len(h) > 0
}
`

	fileC := "pkg/api/handler.go"
	codeC := `package api

func LoginHandler(u string, p string) bool {
	return AuthenticateUser(u, p)
}
`

	// 1. Initial full baseline sync
	initReq := IncrementalSyncRequest{
		RepositoryID: "repo-auth-service",
		ChangedFiles: []string{fileA, fileB, fileC},
		FileContents: map[string]string{
			fileA: codeA,
			fileB: codeB,
			fileC: codeC,
		},
		NewSha: "sha-v1",
	}

	report1, err := syncEngine.Sync(context.Background(), initReq)
	if err != nil {
		t.Fatalf("unexpected error on baseline sync: %v", err)
	}

	if report1.ModifiedFilesCount != 3 {
		t.Errorf("expected 3 modified files in baseline, got %d", report1.ModifiedFilesCount)
	}
	if len(report1.AddedSymbols) != 4 { // HashPassword, AuthenticateUser, ValidateHash, LoginHandler
		t.Errorf("expected 4 added symbols in baseline, got %d (%v)", len(report1.AddedSymbols), report1.AddedSymbols)
	}

	// 2. Incremental update: Modify File B (Remove ValidateHash, add VerifyToken)
	codeB_v2 := `package auth

func AuthenticateUser(user string, pass string) bool {
	h := HashPassword(pass)
	return len(h) > 5
}

func VerifyToken(token string) bool {
	return token != ""
}
`

	incrReq := IncrementalSyncRequest{
		RepositoryID: "repo-auth-service",
		ChangedFiles: []string{fileB},
		FileContents: map[string]string{
			fileB: codeB_v2,
		},
		NewSha: "sha-v2",
	}

	report2, err := syncEngine.Sync(context.Background(), incrReq)
	if err != nil {
		t.Fatalf("unexpected error on incremental sync: %v", err)
	}

	if report2.ModifiedFilesCount != 1 {
		t.Errorf("expected 1 modified file, got %d", report2.ModifiedFilesCount)
	}

	// ValidateHash was removed
	foundRemoved := false
	for _, s := range report2.RemovedSymbols {
		if s == "ValidateHash" {
			foundRemoved = true
		}
	}
	if !foundRemoved {
		t.Errorf("expected ValidateHash to be marked as removed, got %v", report2.RemovedSymbols)
	}

	// VerifyToken was added
	foundAdded := false
	for _, s := range report2.AddedSymbols {
		if s == "VerifyToken" {
			foundAdded = true
		}
	}
	if !foundAdded {
		t.Errorf("expected VerifyToken to be marked as added, got %v", report2.AddedSymbols)
	}

	// File A and File C symbols should still be intact in index
	defsA := syncEngine.GetIndex().GetDefinitionsInFile(fileA)
	if len(defsA) != 1 || defsA[0].Name != "HashPassword" {
		t.Errorf("expected HashPassword in File A to be preserved, got %v", defsA)
	}
	defsC := syncEngine.GetIndex().GetDefinitionsInFile(fileC)
	if len(defsC) != 1 || defsC[0].Name != "LoginHandler" {
		t.Errorf("expected LoginHandler in File C to be preserved, got %v", defsC)
	}

	// 3. Delete File A
	delReq := IncrementalSyncRequest{
		RepositoryID: "repo-auth-service",
		ChangedFiles: []string{fileA},
		FileContents: map[string]string{
			fileA: "", // Empty indicates deleted
		},
		NewSha: "sha-v3",
	}

	report3, err := syncEngine.Sync(context.Background(), delReq)
	if err != nil {
		t.Fatalf("unexpected error on delete sync: %v", err)
	}

	if report3.DeletedFilesCount != 1 {
		t.Errorf("expected 1 deleted file, got %d", report3.DeletedFilesCount)
	}
	if len(report3.RemovedSymbols) != 1 || report3.RemovedSymbols[0] != "HashPassword" {
		t.Errorf("expected HashPassword to be removed, got %v", report3.RemovedSymbols)
	}

	// Callers of HashPassword (AuthenticateUser in File B) should be reported in ImpactedCallers!
	foundImpacted := false
	for _, caller := range report3.ImpactedCallers {
		if caller == "AuthenticateUser" {
			foundImpacted = true
		}
	}
	if !foundImpacted {
		t.Errorf("expected AuthenticateUser to be in ImpactedCallers, got %v", report3.ImpactedCallers)
	}
}

func TestIncrementalGraphSynchronizer_ConcurrentStress(t *testing.T) {
	index := NewSymbolIndex()
	graph := NewCallGraph()
	syncEngine := NewIncrementalGraphSynchronizer(index, graph)

	var wg sync.WaitGroup
	for i := 0; i < 15; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			fileName := fmt.Sprintf("pkg/mod_%d/file.go", workerID%4)
			code := fmt.Sprintf(`package mod
func WorkerFunc_%d() {
	Helper_%d()
}
func Helper_%d() {}
`, workerID, workerID, workerID)

			req := IncrementalSyncRequest{
				RepositoryID: "repo-concurrent",
				ChangedFiles: []string{fileName},
				FileContents: map[string]string{
					fileName: code,
				},
				NewSha: fmt.Sprintf("sha-%d", workerID),
			}

			_, _ = syncEngine.Sync(context.Background(), req)
		}(i)
	}
	wg.Wait()
}
