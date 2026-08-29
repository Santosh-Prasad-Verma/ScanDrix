# Risk Engine — Exact Mathematical Formulation & Architecture

**Classification:** AUTHORITATIVE SPECIFICATION  
**Status:** APPROVED  
**Version:** 2.0.0  
**Engine Package:** `github.com/scandrix/scandrix/internal/risk`

---

## 1. Executive Summary & Core Invariants

The Scandrix Risk Engine provides a mathematically rigorous, multi-dimensional risk assessment framework for continuous software assurance. Unlike simplistic single-scalar severity scores, Scandrix models risk as a continuous vector space spanning six orthogonal engineering disciplines. 

### Core Architectural Invariants:
1. **Deterministic Grounding**: Finding risk scores are derived strictly from deterministic evidence (AST paths, reachability graph distance, CVSS base vectors, EPSS percentiles, and lockfile provenance).
2. **Precision Over Heuristics**: If analysis completeness is degraded (e.g., partial repository scan or unparsed AST branches), the engine applies a calibrated uncertainty penalty rather than assuming unanalyzed code is safe.
3. **Compensating Control Attenuation**: Static severity is attenuated dynamically by proven defensive runtime controls (WAF rules, network policies, memory-safe runtimes, and authenticated ingress boundaries).
4. **Reproducibility**: Given identical finding parameters, dependency graphs, and environment metadata, the risk engine outputs bit-for-bit identical floating-point scores.

---

## 2. The 6D Risk Vector Formulation ($\vec{R}$)

Software risk across any analyzed artifact (PR, commit, container image, or microservice) is represented as a 6-dimensional column vector $\vec{R} \in \mathbb{R}^6$:

$$\vec{R} = \begin{bmatrix} 
R_{\text{security}} \\ 
R_{\text{reliability}} \\ 
R_{\text{architecture}} \\ 
R_{\text{supply\_chain}} \\ 
R_{\text{performance}} \\ 
R_{\text{compliance}} 
\end{bmatrix}, \quad \forall d \in D, \; R_d \in [0.0, 100.0]$$

Where the domain set $D$ is defined as:
$$D = \{ \text{security}, \text{reliability}, \text{architecture}, \text{supply\_chain}, \text{performance}, \text{compliance} \}$$

### Dimension Definitions & Sub-Metrics

| Dimension ($d$) | Description | Underlying Primary Signals |
| :--- | :--- | :--- |
| **$R_{\text{security}}$** | Vulnerability exploitability & blast radius | CVSS v3.1/v4.0, EPSS probability, SAST/DAST taint flows, ingress reachability |
| **$R_{\text{reliability}}$** | Defect likelihood & runtime stability risks | Cyclic dependencies, unhandled exception paths, concurrency race potential, error-swallowing |
| **$R_{\text{architecture}}$** | Structural entropy & technical debt | Module coupling index (Afferent/Efferent), CBO (Coupling Between Objects), God-classes |
| **$R_{\text{supply\_chain}}$** | Third-party package & transitive risk | Deprecated dependencies, known CVEs in transitive trees, malicious packages, license conflicts |
| **$R_{\text{performance}}$** | Latency, throughput, & algorithmic complexity | $O(N^2)$ loops over I/O, unindexed ORM queries, N+1 query patterns, memory leaks |
| **$R_{\text{compliance}}$** | Regulatory & governance non-conformance | PII leakage without encryption, missing audit logging, SOC2/HIPAA/PCI-DSS policy breaches |

---

## 3. Composite Risk Score Equation ($R_{\text{composite}}$)

The scalar **Composite Risk Score** $R_{\text{composite}} \in [0.0, \infty)$ synthesizes the multidimensional vector into a prioritized organizational metric:

$$R_{\text{composite}} = \left( \sum_{d \in D} w_d \cdot R_d \right) \times \left( 1 + \alpha \cdot (1 - C) \right) \times A_{\text{criticality}} \times E_{\text{exposure}}$$

### 3.1 Normalized Domain Weights ($w_d$)

The domain weights $w_d$ are strictly normalized such that $\sum_{d \in D} w_d = 1.0$.

$$\mathbf{w} = \begin{bmatrix} w_{\text{sec}} \\ w_{\text{rel}} \\ w_{\text{arch}} \\ w_{\text{sc}} \\ w_{\text{perf}} \\ w_{\text{comp}} \end{bmatrix} = \begin{bmatrix} 0.35 \\ 0.20 \\ 0.15 \\ 0.15 \\ 0.10 \\ 0.05 \end{bmatrix}$$

*Note: Enterprise administrators can override default weights via tenant-level security policy, provided $\sum w_d = 1.0$ is maintained.*

### 3.2 Data Completeness Index ($C$) & Uncertainty Penalty ($\alpha$)

To prevent false confidence when scanners timeout or codebases are partially analyzed, the Data Completeness Index $C \in (0.0, 1.0]$ measures analysis coverage:

$$C = \frac{\sum_{i=1}^{N} \text{Artifacts Analyzed}_i}{\sum_{i=1}^{N} \text{Total Artifacts In Scope}_i}$$

- $\alpha = 0.5$ is the **Uncertainty Penalty Multiplier**.
- When $C = 1.0$ (complete scan), $(1 - C) = 0 \implies \text{Multiplier} = 1.00$ (no penalty).
- When $C = 0.5$ (half of files analyzed), $\text{Multiplier} = 1 + 0.5(0.5) = 1.25$ (+25% risk inflation).
- When $C \to 0.0$ (severe scan truncation), $\text{Multiplier} \to 1.50$ (+50% maximum uncertainty penalty).

### 3.3 Asset Criticality Multiplier ($A_{\text{criticality}}$)

Asset tiering reflects the business impact of the affected repository or service:

$$A_{\text{criticality}} \in [0.40, 2.50]$$

| Asset Tier | Classification | Multiplier ($A$) | Examples |
| :--- | :--- | :--- | :--- |
| **Tier-0** | Crown Jewels | $2.50$ | Payment gateway, KMS key manager, Auth/IAM provider, PII vault |
| **Tier-1** | High Criticality | $1.80$ | Core checkout API, primary customer database models, routing proxy |
| **Tier-2** | Medium Criticality | $1.00$ | Internal reporting API, analytics pipeline, notification service |
| **Tier-3** | Low Criticality | $0.40$ | Dev tooling, test fixtures, documentation generators, sandbox scripts |

### 3.4 Runtime Exposure Coefficient ($E_{\text{exposure}}$)

Network reachability and authentication boundaries determine runtime exploitability:

$$E_{\text{exposure}} \in [0.10, 2.00]$$

| Exposure Tier | Boundary Definition | Coefficient ($E$) |
| :--- | :--- | :--- |
| **Internet Ingress (Unauthenticated)** | Publicly accessible endpoint with zero auth guard | $2.00$ |
| **Internet Ingress (Authenticated)** | Publicly accessible endpoint protected by JWT/OAuth/mTLS | $1.50$ |
| **Internal VPC / Cluster Ingress** | Service accessible only within private VPC subnet / mesh | $0.80$ |
| **Air-Gapped / Isolated Worker** | Batch worker with no inbound network listeners | $0.40$ |
| **Dead / Unreachable Code** | Code not reachable from any entrypoint in the Call Graph | $0.10$ |

---

## 4. Individual Finding Risk Scoring Formulation

For any specific security finding $f_i$, its individual score $S(f_i)$ is evaluated as:

$$S(f_i) = \frac{\text{CVSS}_{\text{base}}(f_i) \times \text{Conf}(f_i) \times \text{Exploit}(f_i) \times A_{\text{criticality}}}{1.0 + \sum_{k \in K(f_i)} \text{CompensatingControl}_k}$$

### 4.1 Parameters & Boundaries

1. **$\text{CVSS}_{\text{base}}(f_i) \in [0.1, 10.0]$**: Standard CVSS v3.1 / v4.0 base score.
2. **$\text{Conf}(f_i) \in [0.5, 1.0]$** (Analysis Confidence):
   - Deterministic AST / Exact Compiler Analysis: $\text{Conf} = 1.00$
   - Verified Dependency Lockfile Match: $\text{Conf} = 1.00$
   - Static Taint Propagation (Cross-file): $\text{Conf} = 0.85$
   - Heuristic / Pattern Matching: $\text{Conf} = 0.70$
   - AI Speculative Inference: $\text{Conf} = 0.50$
3. **$\text{Exploit}(f_i) \in [0.2, 1.5]$** (Real-world Exploitability):
   $$\text{Exploit}(f_i) = \max\left(0.20, \min\left(1.50, \text{EPSS}(f_i) \times 1.2 + \mathbb{I}_{\text{InTheWildProof}} \times 0.5\right)\right)$$
   - Where $\text{EPSS} \in [0.0, 1.0]$ is the FIRST.org Exploit Prediction Scoring System score.
   - $\mathbb{I}_{\text{InTheWildProof}} = 1$ if Metasploit/CISA KEV proof exists, else $0$.
4. **Compensating Controls $\sum \text{CompensatingControl}_k$**:

| Compensating Control ($k$) | Attenuation Weight | Verification Method |
| :--- | :--- | :--- |
| **WAF Active Rule** | $+0.40$ | Live Cloudflare / AWS WAF rule targeting CWE sink |
| **Kubernetes NetworkPolicy** | $+0.30$ | Strict egress/ingress isolation verified in IaC |
| **Memory-Safe Runtime** | $+0.50$ | Rust / Go execution environment (negates buffer overflows) |
| **mTLS Service Mesh** | $+0.35$ | Istio / Linkerd PeerAuthentication STRICT mode |
| **Read-Only Root Filesystem** | $+0.25$ | Container securityContext `readOnlyRootFilesystem: true` |

---

## 5. Production Go 1.24+ Implementation

```go
package risk

import (
	"errors"
	"math"
)

// Domain represents the 6 orthogonal risk dimensions.
type Domain int

const (
	DomainSecurity Domain = iota
	DomainReliability
	DomainArchitecture
	DomainSupplyChain
	DomainPerformance
	DomainCompliance
	DomainCount = 6
)

// RiskVector holds the 6D risk scores [0.0, 100.0].
type RiskVector [DomainCount]float64

// DomainWeights defines normalized weights summing to 1.0.
type DomainWeights [DomainCount]float64

// DefaultDomainWeights returns standard enterprise weights.
func DefaultDomainWeights() DomainWeights {
	return DomainWeights{
		DomainSecurity:     0.35,
		DomainReliability:  0.20,
		DomainArchitecture: 0.15,
		DomainSupplyChain:  0.15,
		DomainPerformance:  0.10,
		DomainCompliance:   0.05,
	}
}

// AssetTier defines the criticality tier of an analyzed system.
type AssetTier float64

const (
	AssetTier0CrownJewel AssetTier = 2.50
	AssetTier1High       AssetTier = 1.80
	AssetTier2Medium     AssetTier = 1.00
	AssetTier3Low        AssetTier = 0.40
)

// ExposureTier defines the network reachability context.
type ExposureTier float64

const (
	ExposureInternetNoAuth ExposureTier = 2.00
	ExposureInternetAuth   ExposureTier = 1.50
	ExposureInternalVPC    ExposureTier = 0.80
	ExposureAirGapped      ExposureTier = 0.40
	ExposureDeadCode       ExposureTier = 0.10
)

// RiskEngine evaluates continuous multidimensional risk.
type RiskEngine struct {
	weights DomainWeights
	alpha   float64 // Uncertainty penalty multiplier (default 0.5)
}

func NewRiskEngine(weights DomainWeights) (*RiskEngine, error) {
	var sum float64
	for _, w := range weights {
		if w < 0 {
			return nil, errors.New("domain weight cannot be negative")
		}
		sum += w
	}
	if math.Abs(sum-1.0) > 1e-6 {
		return nil, errors.New("domain weights must sum to exactly 1.0")
	}
	return &RiskEngine{
		weights: weights,
		alpha:   0.5,
	}, nil
}

// CalculateCompositeScore evaluates the complete mathematical formulation.
func (e *RiskEngine) CalculateCompositeScore(
	vec RiskVector,
	completeness float64,
	assetTier AssetTier,
	exposure ExposureTier,
) (float64, error) {
	if completeness <= 0.0 || completeness > 1.0 {
		return 0, errors.New("data completeness index must be in range (0.0, 1.0]")
	}
	if assetTier < 0.40 || assetTier > 2.50 {
		return 0, errors.New("asset tier multiplier must be in range [0.40, 2.50]")
	}
	if exposure < 0.10 || exposure > 2.00 {
		return 0, errors.New("exposure coefficient must be in range [0.10, 2.00]")
	}

	// 1. Weighted sum across 6 dimensions
	var weightedSum float64
	for d := 0; d < DomainCount; d++ {
		score := math.Max(0.0, math.Min(100.0, vec[d]))
		weightedSum += e.weights[d] * score
	}

	// 2. Uncertainty penalty multiplier: (1 + alpha * (1 - C))
	uncertaintyFactor := 1.0 + e.alpha*(1.0-completeness)

	// 3. Composite score calculation
	composite := weightedSum * uncertaintyFactor * float64(assetTier) * float64(exposure)

	return composite, nil
}

// FindingContext contains finding-specific telemetry.
type FindingContext struct {
	CVSSBase             float64
	Confidence           float64 // 0.5 to 1.0
	EPSS                 float64 // 0.0 to 1.0
	HasInTheWildProof    bool
	AssetCriticality     AssetTier
	CompensatingControls []float64
}

// CalculateFindingScore evaluates single finding risk with compensating controls.
func (e *RiskEngine) CalculateFindingScore(fc FindingContext) (float64, error) {
	if fc.CVSSBase < 0.1 || fc.CVSSBase > 10.0 {
		return 0, errors.New("CVSS base must be in range [0.1, 10.0]")
	}
	conf := math.Max(0.5, math.Min(1.0, fc.Confidence))
	
	// Exploitability index calculation
	exploitProofBonus := 0.0
	if fc.HasInTheWildProof {
		exploitProofBonus = 0.5
	}
	exploitability := math.Max(0.20, math.Min(1.50, fc.EPSS*1.2+exploitProofBonus))

	// Compensating control attenuation sum
	var controlSum float64
	for _, c := range fc.CompensatingControls {
		if c > 0 {
			controlSum += c
		}
	}

	// S(f_i) calculation
	numerator := fc.CVSSBase * conf * exploitability * float64(fc.AssetCriticality)
	denominator := 1.0 + controlSum

	return numerator / denominator, nil
}
```

---

## 6. Mathematical Properties & Proofs

### Theorem 1 (Monotonicity with Respect to Vulnerability Severity):
$$\forall f_1, f_2, \; \text{CVSS}_{\text{base}}(f_1) > \text{CVSS}_{\text{base}}(f_2) \implies S(f_1) > S(f_2) \quad (\text{all other variables constant})$$

*Proof:* $\frac{\partial S(f)}{\partial \text{CVSS}_{\text{base}}} = \frac{\text{Conf} \cdot \text{Exploit} \cdot A_{\text{criticality}}}{1.0 + \sum \text{Controls}} > 0$ since all terms in the numerator and denominator are strictly positive. Thus $S(f)$ is strictly monotonically increasing with respect to $\text{CVSS}_{\text{base}}$. $\blacksquare$

### Theorem 2 (Non-Zero Completeness Safeguard):
As $C \to 0^+$, $R_{\text{composite}} \to 1.50 \times \left(\sum w_d R_d\right) \times A_{\text{criticality}} \times E_{\text{exposure}}$.  
The uncertainty penalty acts as an upper bound factor $\lim_{C \to 0^+} (1 + 0.5(1 - C)) = 1.50$, preventing score divergence while strictly penalizing incomplete telemetry. $\blacksquare$
