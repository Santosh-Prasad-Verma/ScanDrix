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

func TestLinkedRepoGraph_BlastRadiusTraversal(t *testing.T) {
	engine := NewLinkedRepoGraphEngine()

	// Setup a 4-package pipeline across 2 repos:
	// proto-schema (repo-proto)
	//   -> auth-service (repo-backend, depends on proto-schema)
	//   -> order-service (repo-backend, depends on proto-schema)
	//   -> web-client (repo-frontend, depends on auth-service and order-service)

	engine.RegisterPackage(PackageNode{
		ID:          "proto-schema",
		RepoName:    "repo-proto",
		PackagePath: "proto/v1",
		Contracts: []ContractSignature{
			{Name: "UserCredentials", Fields: []string{"username", "password", "mfa_token"}},
			{Name: "Authenticate", Params: []string{"ctx", "req"}},
		},
	})

	engine.RegisterPackage(PackageNode{
		ID:          "auth-service",
		RepoName:    "repo-backend",
		PackagePath: "services/auth",
		DependsOn:   []string{"proto-schema"},
	})

	engine.RegisterPackage(PackageNode{
		ID:          "order-service",
		RepoName:    "repo-backend",
		PackagePath: "services/order",
		DependsOn:   []string{"proto-schema"},
	})

	engine.RegisterPackage(PackageNode{
		ID:          "web-client",
		RepoName:    "repo-frontend",
		PackagePath: "apps/web",
		DependsOn:   []string{"auth-service", "order-service"},
	})

	// Case 1: Modify proto-schema
	report := engine.AnalyzeCrossRepoBlastRadius(
		context.Background(),
		"repo-proto",
		[]string{"proto-schema"},
		nil,
	)

	// Direct dependents should be auth-service and order-service
	if len(report.DirectDependents) != 2 {
		t.Fatalf("expected 2 direct dependents, got %d", len(report.DirectDependents))
	}
	// Transitive dependent should be web-client
	if len(report.TransitiveDependents) != 1 || report.TransitiveDependents[0] != "web-client" {
		t.Fatalf("expected web-client as transitive dependent, got %+v", report.TransitiveDependents)
	}

	if report.HasCyclicDependency {
		t.Errorf("expected acyclic graph")
	}
}

func TestLinkedRepoGraph_BreakingContractChanges(t *testing.T) {
	engine := NewLinkedRepoGraphEngine()

	engine.RegisterPackage(PackageNode{
		ID:          "auth-contract",
		RepoName:    "repo-shared",
		PackagePath: "contracts/auth",
		Contracts: []ContractSignature{
			{
				Name:   "TokenExchangeRequest",
				Fields: []string{"grant_type", "client_id", "client_secret", "scope"},
			},
			{
				Name:   "ValidateSession",
				Params: []string{"ctx", "session_token"},
			},
			{
				Name: "DeprecatedLegacyLogin",
			},
		},
	})

	engine.RegisterPackage(PackageNode{
		ID:          "api-gateway",
		RepoName:    "repo-backend",
		PackagePath: "gateway",
		DependsOn:   []string{"auth-contract"},
	})

	// PR modifies contracts:
	// 1. Removes field "client_secret" from TokenExchangeRequest
	// 2. Adds extra parameter to ValidateSession without fallback
	// 3. Deletes DeprecatedLegacyLogin entirely
	modifiedContracts := []ContractSignature{
		{
			Name:   "TokenExchangeRequest",
			Fields: []string{"grant_type", "client_id", "scope"}, // client_secret REMOVED!
		},
		{
			Name:   "ValidateSession",
			Params: []string{"ctx", "session_token", "mandatory_tenant_id"}, // Added parameter!
		},
	}

	report := engine.AnalyzeCrossRepoBlastRadius(
		context.Background(),
		"repo-shared",
		[]string{"auth-contract"},
		modifiedContracts,
	)

	if len(report.BreakingChanges) != 3 {
		t.Fatalf("expected 3 breaking changes, got %d: %+v", len(report.BreakingChanges), report.BreakingChanges)
	}

	breakKinds := make(map[BreakingChangeKind]bool)
	for _, bc := range report.BreakingChanges {
		breakKinds[bc.Kind] = true
	}

	if !breakKinds[BreakingFieldRemoval] {
		t.Errorf("expected FIELD_REMOVAL breaking change")
	}
	if !breakKinds[BreakingParamMismatch] {
		t.Errorf("expected PARAM_MISMATCH breaking change")
	}
	if !breakKinds[BreakingDeletedSymbol] {
		t.Errorf("expected DELETED_SYMBOL breaking change")
	}

	if report.CrossRepoRiskScore <= 0.5 {
		t.Errorf("expected high risk score for breaking changes, got %f", report.CrossRepoRiskScore)
	}
}

func TestLinkedRepoGraph_CycleDetection(t *testing.T) {
	engine := NewLinkedRepoGraphEngine()

	// A -> B -> C -> A (cyclic)
	engine.RegisterPackage(PackageNode{
		ID:        "pkg-a",
		DependsOn: []string{"pkg-b"},
	})
	engine.RegisterPackage(PackageNode{
		ID:        "pkg-b",
		DependsOn: []string{"pkg-c"},
	})
	engine.RegisterPackage(PackageNode{
		ID:        "pkg-c",
		DependsOn: []string{"pkg-a"},
	})

	report := engine.AnalyzeCrossRepoBlastRadius(context.Background(), "repo-test", []string{"pkg-a"}, nil)

	if !report.HasCyclicDependency {
		t.Fatalf("expected cycle to be detected")
	}
	if len(report.CycleTrace) == 0 {
		t.Fatalf("expected non-empty cycle trace")
	}
}

func TestLinkedRepoGraph_ConcurrentStress(t *testing.T) {
	engine := NewLinkedRepoGraphEngine()

	var wg sync.WaitGroup
	workers := 25

	for w := 0; w < workers; w++ {
		wg.Add(2)

		// Worker A: Register package
		go func(id int) {
			defer wg.Done()
			engine.RegisterPackage(PackageNode{
				ID:          fmt.Sprintf("pkg-%d", id),
				RepoName:    "repo-multi",
				PackagePath: fmt.Sprintf("pkg/%d", id),
				DependsOn:   []string{fmt.Sprintf("pkg-%d", (id+1)%workers)},
				Contracts: []ContractSignature{
					{Name: fmt.Sprintf("Type%d", id), Fields: []string{"a", "b"}},
				},
			})
		}(w)

		// Worker B: Read and analyze blast radius
		go func(id int) {
			defer wg.Done()
			_ = engine.AnalyzeCrossRepoBlastRadius(
				context.Background(),
				"repo-multi",
				[]string{fmt.Sprintf("pkg-%d", id)},
				nil,
			)
		}(w)
	}

	wg.Wait()
}
