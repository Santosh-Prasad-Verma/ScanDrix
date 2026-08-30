package securitytwin

import (
	"errors"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ContinuumHop represents one of the 7 stages across code-to-cloud.
type ContinuumHop string

const (
	HopCodeAST         ContinuumHop = "1_CODE_AST"
	HopBuildPackage    ContinuumHop = "2_BUILD_PACKAGE"
	HopArtifactSBOM    ContinuumHop = "3_ARTIFACT_SBOM"
	HopContainerImage  ContinuumHop = "4_CONTAINER_IMAGE"
	HopCloudDeployment ContinuumHop = "5_CLOUD_DEPLOYMENT"
	HopRuntimeIngress  ContinuumHop = "6_RUNTIME_INGRESS"
	HopBusinessAsset   ContinuumHop = "7_BUSINESS_ASSET"
)

// ControlCategory identifies the layer of a defensive mitigation.
type ControlCategory string

const (
	ControlWAF           ControlCategory = "WAF_INSPECTION"
	ControlNetworkPolicy ControlCategory = "NETWORK_POLICY"
	ControlDeadCode      ControlCategory = "DEAD_CODE_PATH"
	ControlAuthBoundary  ControlCategory = "AUTHENTICATED_BOUNDARY"
	ControlMemorySafe    ControlCategory = "MEMORY_SAFE_RUNTIME"
)

// RuntimeMitigation represents an active verified defensive control.
type RuntimeMitigation struct {
	ID          string          `json:"id"`
	Category    ControlCategory `json:"category"`
	Description string          `json:"description"`
	Efficacy    float64         `json:"efficacy"` // [0.0, 1.0]
	Active      bool            `json:"active"`
	VerifiedAt  time.Time       `json:"verified_at"`
}

// ContinuumNode tracks an entity at a hop in the digital twin.
type ContinuumNode struct {
	ID         string       `json:"id"`
	Hop        ContinuumHop `json:"hop"`
	Name       string       `json:"name"`
	ParentID   string       `json:"parent_id,omitempty"`
	AssetTier  string       `json:"asset_tier,omitempty"`
	IsPublic   bool         `json:"is_public"`
	IsReachable bool        `json:"is_reachable"`
}

// SecurityTwin manages the 7-hop continuum graph and live risk attenuation.
type SecurityTwin struct {
	mu          sync.RWMutex
	workspaceID uuid.UUID
	nodes       map[string]*ContinuumNode
	mitigations map[string]*RuntimeMitigation
}

// NewSecurityTwin initializes a workspace security twin.
func NewSecurityTwin(workspaceID uuid.UUID) *SecurityTwin {
	return &SecurityTwin{
		workspaceID: workspaceID,
		nodes:       make(map[string]*ContinuumNode),
		mitigations: make(map[string]*RuntimeMitigation),
	}
}

// AddNode registers an asset or deployment node in the continuum.
func (t *SecurityTwin) AddNode(node *ContinuumNode) error {
	if node.ID == "" {
		return errors.New("node ID cannot be empty")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nodes[node.ID] = node
	return nil
}

// RegisterMitigation adds a verified runtime defensive control.
func (t *SecurityTwin) RegisterMitigation(m *RuntimeMitigation) error {
	if m.ID == "" {
		return errors.New("mitigation ID cannot be empty")
	}
	if m.Efficacy < 0.0 || m.Efficacy > 1.0 {
		return errors.New("efficacy must be between 0.0 and 1.0")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.mitigations[m.ID] = m
	return nil
}

// AttenuateRisk computes residual CVSS risk considering active mitigations and correlation discounts.
// R_attenuated = CVSS * (1.0 - TotalMitigationBenefit)
func (t *SecurityTwin) AttenuateRisk(baseCVSS float64, mitigationIDs []string, correlationDiscount float64) float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if len(mitigationIDs) == 0 {
		return baseCVSS
	}

	// Calculate unmitigated failure product
	residualMultiplier := 1.0
	var appliedControls []*RuntimeMitigation

	for _, id := range mitigationIDs {
		m, exists := t.mitigations[id]
		if !exists || !m.Active {
			continue
		}

		effectiveEfficacy := m.Efficacy

		// Cap at 0.99: no single control is 100% effective; 1.0 would
		// zero the residual multiplier and erase all remaining risk.
		if effectiveEfficacy > 0.99 {
			effectiveEfficacy = 0.99
		}

		// If duplicate/similar category control already applied, apply correlation discount
		for _, prev := range appliedControls {
			if prev.Category == m.Category {
				effectiveEfficacy *= (1.0 - correlationDiscount)
				break
			}
		}

		residualMultiplier *= (1.0 - effectiveEfficacy)
		appliedControls = append(appliedControls, m)
	}

	attenuated := baseCVSS * residualMultiplier
	return math.Round(attenuated*100) / 100
}

// TraceReachability checks if a code finding at HopCodeAST reaches public ingress or business assets.
func (t *SecurityTwin) TraceReachability(startNodeID string) (bool, []string) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	visited := make(map[string]bool)
	var path []string

	// Build directed adjacency list: parent → child only.
	// The continuum flows from code (Hop 1) toward public ingress (Hop 6) and
	// business assets (Hop 7). Bidirectional edges would cause a node at
	// HopRuntimeIngress to falsely "reach" back down to HopCodeAST.
	adj := make(map[string][]string)
	for id, node := range t.nodes {
		if node.ParentID != "" {
			adj[node.ParentID] = append(adj[node.ParentID], id)
		}
	}

	var dfs func(curr string) bool
	dfs = func(curr string) bool {
		visited[curr] = true
		path = append(path, curr)

		node := t.nodes[curr]
		if node != nil && (node.IsPublic || node.Hop == HopRuntimeIngress || node.Hop == HopBusinessAsset) {
			return true
		}

		for _, neighbor := range adj[curr] {
			if !visited[neighbor] {
				if dfs(neighbor) {
					return true
				}
			}
		}
		path = path[:len(path)-1]
		return false
	}

	reached := dfs(startNodeID)
	return reached, path
}
