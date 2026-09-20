package riskvector

import (
	"fmt"
	"math"
)

// Vector6D models the 6-dimensional risk vector space R^6 in [0.0, 100.0].
type Vector6D struct {
	Security     float64 `json:"security"`
	Reliability  float64 `json:"reliability"`
	Architecture float64 `json:"architecture"`
	SupplyChain  float64 `json:"supply_chain"`
	Performance  float64 `json:"performance"`
	Compliance   float64 `json:"compliance"`
}

// Weights contains normalized weights summing to 1.0 across the 6 dimensions.
type Weights struct {
	Security     float64
	Reliability  float64
	Architecture float64
	SupplyChain  float64
	Performance  float64
	Compliance   float64
}

// DefaultWeights defines standard enterprise risk domain weight distribution.
func DefaultWeights() Weights {
	return Weights{
		Security:     0.35,
		Reliability:  0.20,
		Architecture: 0.10,
		SupplyChain:  0.15,
		Performance:  0.10,
		Compliance:   0.10,
	}
}

// AssessmentContext contains environmental factors modulating composite risk.
type AssessmentContext struct {
	Completeness float64 // C in [0.0, 1.0] representing AST/reachability coverage
	AlphaPenalty float64 // Uncertainty scaling factor (typically 0.5)
	Criticality  float64 // Asset criticality multiplier (1.0 = standard, 1.5 = tier-1 payments)
	Exposure     float64 // Network exposure (0.8 = internal cron, 1.0 = staging, 1.3 = public 0.0.0.0/0)
}

// DefaultContext provides baseline standard assessment parameters.
func DefaultContext() AssessmentContext {
	return AssessmentContext{
		Completeness: 1.0,
		AlphaPenalty: 0.5,
		Criticality:  1.0,
		Exposure:     1.0,
	}
}

// RiskEngine evaluates 6D risk vectors and synthesizes composite scores.
type RiskEngine struct {
	weights Weights
}

// NewRiskEngine initializes the risk engine with domain weights.
func NewRiskEngine(weights ...Weights) *RiskEngine {
	w := DefaultWeights()
	if len(weights) > 0 {
		w = weights[0]
	}
	return &RiskEngine{weights: w}
}

// Clamp restricts a scalar to [0.0, 100.0].
func Clamp(v float64) float64 {
	if v < 0.0 {
		return 0.0
	}
	if v > 100.0 {
		return 100.0
	}
	return v
}

// ComputeComposite calculates the scalar composite risk score:
// R_composite = (sum(w_d * R_d)) * (1 + alpha * (1 - C)) * Criticality * Exposure
func (e *RiskEngine) ComputeComposite(v Vector6D, ctx AssessmentContext) float64 {
	// Clamp each dimension
	sec := Clamp(v.Security)
	rel := Clamp(v.Reliability)
	arc := Clamp(v.Architecture)
	sup := Clamp(v.SupplyChain)
	prf := Clamp(v.Performance)
	cmp := Clamp(v.Compliance)

	weightedSum := (sec * e.weights.Security) +
		(rel * e.weights.Reliability) +
		(arc * e.weights.Architecture) +
		(sup * e.weights.SupplyChain) +
		(prf * e.weights.Performance) +
		(cmp * e.weights.Compliance)

	c := ctx.Completeness
	if c < 0.0 {
		c = 0.0
	} else if c > 1.0 {
		c = 1.0
	}

	uncertaintyMult := 1.0 + (ctx.AlphaPenalty * (1.0 - c))
	critMult := ctx.Criticality
	if critMult <= 0 {
		critMult = 1.0
	}
	expoMult := ctx.Exposure
	if expoMult <= 0 {
		expoMult = 1.0
	}

	composite := weightedSum * uncertaintyMult * critMult * expoMult
	return math.Round(composite*100) / 100
}

// AttenuateWithControls applies compensating runtime defenses to reduce risk.
// R_attenuated = CVSS * product(1.0 - Efficacy(m))
func AttenuateWithControls(baseScore float64, controlEfficacies []float64) float64 {
	residual := baseScore
	for _, eff := range controlEfficacies {
		if eff >= 1.0 {
			return 0.0
		}
		if eff > 0 {
			residual *= (1.0 - eff)
		}
	}
	return math.Round(residual*100) / 100
}

// SeverityGrade categorizes composite scores into standard enterprise risk bands.
func SeverityGrade(score float64) string {
	switch {
	case score >= 80.0:
		return "CRITICAL"
	case score >= 60.0:
		return "HIGH"
	case score >= 40.0:
		return "MEDIUM"
	case score >= 20.0:
		return "LOW"
	default:
		return "INFORMATIONAL"
	}
}

// String prints the 6D vector in mathematical column notation.
func (v Vector6D) String() string {
	return fmt.Sprintf("[Sec:%.1f, Rel:%.1f, Arc:%.1f, Sup:%.1f, Prf:%.1f, Cmp:%.1f]",
		v.Security, v.Reliability, v.Architecture, v.SupplyChain, v.Performance, v.Compliance)
}
