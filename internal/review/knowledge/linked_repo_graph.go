// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package knowledge

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// DependencyKind specifies the coupling between repositories or packages.
type DependencyKind string

const (
	DepKindInternalMonorepo DependencyKind = "INTERNAL_MONOREPO"
	DepKindLinkedRepository DependencyKind = "LINKED_REPOSITORY"
	DepKindSharedProtobuf   DependencyKind = "SHARED_PROTOBUF"
	DepKindOpenAPIContract  DependencyKind = "OPENAPI_CONTRACT"
)

// ContractSignature represents an exported programmatic interface or schema contract.
type ContractSignature struct {
	Name        string   `json:"name"`
	Package     string   `json:"package"`
	Kind        string   `json:"kind"` // "STRUCT", "INTERFACE", "FUNCTION", "ENDPOINT"
	Fields      []string `json:"fields,omitempty"`
	Params      []string `json:"params,omitempty"`
	Returns     []string `json:"returns,omitempty"`
	IsExported  bool     `json:"is_exported"`
	Deprecate   bool     `json:"deprecate"`
}

// PackageNode models a module or package in a monorepo or linked repo.
type PackageNode struct {
	ID          string              `json:"id"`
	RepoName    string              `json:"repo_name"`
	PackagePath string              `json:"package_path"`
	Contracts   []ContractSignature `json:"contracts"`
	DependsOn   []string            `json:"depends_on"` // List of package IDs
}

// BreakingChangeKind categorizes the severity of a cross-package contract violation.
type BreakingChangeKind string

const (
	BreakingDeletedSymbol    BreakingChangeKind = "DELETED_SYMBOL"
	BreakingFieldRemoval     BreakingChangeKind = "FIELD_REMOVAL"
	BreakingParamMismatch    BreakingChangeKind = "PARAM_MISMATCH"
	BreakingReturnMismatch   BreakingChangeKind = "RETURN_MISMATCH"
)

// CrossRepoBreakingChange details an incompatibility introduced between modules.
type CrossRepoBreakingChange struct {
	SourceRepo      string             `json:"source_repo"`
	SourcePackage   string             `json:"source_package"`
	ImpactedRepo    string             `json:"impacted_repo"`
	ImpactedPackage string             `json:"impacted_package"`
	SymbolName      string             `json:"symbol_name"`
	Kind            BreakingChangeKind `json:"kind"`
	Description     string             `json:"description"`
	Remediation     string             `json:"remediation"`
}

// CrossRepoBlastRadiusReport summarizes the upstream/downstream impact across repositories.
type CrossRepoBlastRadiusReport struct {
	ChangedRepo            string                    `json:"changed_repo"`
	ChangedPackages        []string                  `json:"changed_packages"`
	DirectDependents       []string                  `json:"direct_dependents"`
	TransitiveDependents   []string                  `json:"transitive_dependents"`
	BreakingChanges        []CrossRepoBreakingChange `json:"breaking_changes"`
	TopologicalOrder       []string                  `json:"topological_order"`
	HasCyclicDependency   bool                      `json:"has_cyclic_dependency"`
	CycleTrace             []string                  `json:"cycle_trace,omitempty"`
	CrossRepoRiskScore     float64                   `json:"cross_repo_risk_score"` // 0.0 to 1.0
	GeneratedAt            time.Time                 `json:"generated_at"`
}

// LinkedRepoGraphEngine manages multi-repository package dependency DAGs, contract validations,
// and cross-repository blast-radius calculations.
type LinkedRepoGraphEngine struct {
	mu           sync.RWMutex
	packages     map[string]*PackageNode // packageID -> PackageNode
	dependents   map[string][]string     // packageID -> []dependentPackageIDs
}

// NewLinkedRepoGraphEngine constructs a new graph engine.
func NewLinkedRepoGraphEngine() *LinkedRepoGraphEngine {
	return &LinkedRepoGraphEngine{
		packages:   make(map[string]*PackageNode),
		dependents: make(map[string][]string),
	}
}

// RegisterPackage adds or updates a package node in the multi-repository graph.
func (g *LinkedRepoGraphEngine) RegisterPackage(pkg PackageNode) {
	g.mu.Lock()
	defer g.mu.Unlock()

	cp := pkg
	g.packages[pkg.ID] = &cp

	// Rebuild reverse dependency index
	g.rebuildDependents()
}

func (g *LinkedRepoGraphEngine) rebuildDependents() {
	g.dependents = make(map[string][]string)
	for pkgID, node := range g.packages {
		for _, depID := range node.DependsOn {
			g.dependents[depID] = append(g.dependents[depID], pkgID)
		}
	}
}

// AnalyzeCrossRepoBlastRadius evaluates modified packages against dependent services.
func (g *LinkedRepoGraphEngine) AnalyzeCrossRepoBlastRadius(
	ctx context.Context,
	changedRepo string,
	changedPkgIDs []string,
	modifiedContracts []ContractSignature,
) CrossRepoBlastRadiusReport {
	g.mu.RLock()
	defer g.mu.RUnlock()

	report := CrossRepoBlastRadiusReport{
		ChangedRepo:          changedRepo,
		ChangedPackages:      changedPkgIDs,
		DirectDependents:     make([]string, 0),
		TransitiveDependents: make([]string, 0),
		BreakingChanges:      make([]CrossRepoBreakingChange, 0),
		GeneratedAt:          time.Now().UTC(),
	}

	directSet := make(map[string]struct{})
	transitiveSet := make(map[string]struct{})

	// 1. Direct dependents traversal
	for _, pkgID := range changedPkgIDs {
		deps := g.dependents[pkgID]
		for _, dep := range deps {
			directSet[dep] = struct{}{}
		}
	}

	// 2. Transitive dependents traversal (BFS)
	queue := make([]string, 0, len(directSet))
	for dep := range directSet {
		report.DirectDependents = append(report.DirectDependents, dep)
		queue = append(queue, dep)
	}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		downstreams := g.dependents[curr]
		for _, down := range downstreams {
			if _, seenDirect := directSet[down]; seenDirect {
				continue
			}
			if _, seenTrans := transitiveSet[down]; !seenTrans {
				transitiveSet[down] = struct{}{}
				queue = append(queue, down)
			}
		}
	}

	for trans := range transitiveSet {
		report.TransitiveDependents = append(report.TransitiveDependents, trans)
	}

	sort.Strings(report.DirectDependents)
	sort.Strings(report.TransitiveDependents)

	// 3. Breaking changes evaluation against dependent contracts
	report.BreakingChanges = g.detectContractBreakingChanges(changedRepo, changedPkgIDs, modifiedContracts)

	// 4. Topological sort & cycle detection
	topo, hasCycle, cycle := g.computeTopologicalSort()
	report.TopologicalOrder = topo
	report.HasCyclicDependency = hasCycle
	report.CycleTrace = cycle

	// 5. Composite Risk Score calculation
	totalImpacted := len(report.DirectDependents) + len(report.TransitiveDependents)
	riskScore := 0.20 + (float64(totalImpacted) * 0.15) + (float64(len(report.BreakingChanges)) * 0.25)
	if hasCycle {
		riskScore += 0.30
	}
	if riskScore > 1.0 {
		riskScore = 1.0
	}
	report.CrossRepoRiskScore = riskScore

	return report
}

func (g *LinkedRepoGraphEngine) detectContractBreakingChanges(
	changedRepo string,
	changedPkgIDs []string,
	modifiedContracts []ContractSignature,
) []CrossRepoBreakingChange {
	if len(modifiedContracts) == 0 {
		return nil
	}

	var breaking []CrossRepoBreakingChange
	modMap := make(map[string]ContractSignature, len(modifiedContracts))
	for _, c := range modifiedContracts {
		modMap[c.Name] = c
	}

	for _, pkgID := range changedPkgIDs {
		originalPkg, exists := g.packages[pkgID]
		if !exists {
			continue
		}

		dependents := g.dependents[pkgID]
		for _, depID := range dependents {
			depPkg := g.packages[depID]
			depRepo := ""
			if depPkg != nil {
				depRepo = depPkg.RepoName
			}

			// Cross-examine original contracts
			for _, origContract := range originalPkg.Contracts {
				newContract, modified := modMap[origContract.Name]
				if !modified {
					// Contract was removed entirely!
					breaking = append(breaking, CrossRepoBreakingChange{
						SourceRepo:      changedRepo,
						SourcePackage:   pkgID,
						ImpactedRepo:    depRepo,
						ImpactedPackage: depID,
						SymbolName:      origContract.Name,
						Kind:            BreakingDeletedSymbol,
						Description:     fmt.Sprintf("Exported contract symbol `%s` was deleted or unexported, breaking dependent package `%s`.", origContract.Name, depID),
						Remediation:     "Restore symbol with a deprecation notice instead of immediate deletion.",
					})
					continue
				}

				// Check removed struct/interface fields
				removedFields := findRemovedElements(origContract.Fields, newContract.Fields)
				for _, rf := range removedFields {
					breaking = append(breaking, CrossRepoBreakingChange{
						SourceRepo:      changedRepo,
						SourcePackage:   pkgID,
						ImpactedRepo:    depRepo,
						ImpactedPackage: depID,
						SymbolName:      fmt.Sprintf("%s.%s", origContract.Name, rf),
						Kind:            BreakingFieldRemoval,
						Description:     fmt.Sprintf("Field `%s` was removed from contract `%s` consumed by `%s`.", rf, origContract.Name, depID),
						Remediation:     "Maintain field backwards compatibility or introduce a versioned schema.",
					})
				}

				// Check added function parameters (calling convention mismatch)
				if len(newContract.Params) > len(origContract.Params) {
					breaking = append(breaking, CrossRepoBreakingChange{
						SourceRepo:      changedRepo,
						SourcePackage:   pkgID,
						ImpactedRepo:    depRepo,
						ImpactedPackage: depID,
						SymbolName:      origContract.Name,
						Kind:            BreakingParamMismatch,
						Description:     fmt.Sprintf("Function signature `%s` added %d parameters without default fallback.", origContract.Name, len(newContract.Params)-len(origContract.Params)),
						Remediation:     "Use functional options or variadic parameters to avoid breaking callers.",
					})
				}
			}
		}
	}

	return breaking
}

// computeTopologicalSort runs Kahn's algorithm to determine build/validation order and cycles.
func (g *LinkedRepoGraphEngine) computeTopologicalSort() ([]string, bool, []string) {
	inDegree := make(map[string]int)
	for pkgID := range g.packages {
		inDegree[pkgID] = 0
	}

	for _, node := range g.packages {
		for _, dep := range node.DependsOn {
			if _, exists := g.packages[dep]; exists {
				inDegree[node.ID]++
			}
		}
	}

	var zeroQueue []string
	for pkgID, deg := range inDegree {
		if deg == 0 {
			zeroQueue = append(zeroQueue, pkgID)
		}
	}
	sort.Strings(zeroQueue)

	var topo []string
	for len(zeroQueue) > 0 {
		curr := zeroQueue[0]
		zeroQueue = zeroQueue[1:]
		topo = append(topo, curr)

		for _, dependent := range g.dependents[curr] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				zeroQueue = append(zeroQueue, dependent)
			}
		}
	}

	if len(topo) < len(g.packages) {
		// Cycle detected
		var cyclicNodes []string
		for pkgID, deg := range inDegree {
			if deg > 0 {
				cyclicNodes = append(cyclicNodes, pkgID)
			}
		}
		sort.Strings(cyclicNodes)
		return topo, true, cyclicNodes
	}

	return topo, false, nil
}

func findRemovedElements(original, updated []string) []string {
	upMap := make(map[string]struct{}, len(updated))
	for _, u := range updated {
		upMap[u] = struct{}{}
	}

	var removed []string
	for _, o := range original {
		if _, exists := upMap[o]; !exists {
			removed = append(removed, o)
		}
	}
	return removed
}
