package riskvector_test

import (
	"testing"

	"github.com/scandrix/backend/internal/scandrix/riskvector"
)

func Test6DRiskVectorCalculationAndAttenuation(t *testing.T) {
	engine := riskvector.NewRiskEngine()

	// High risk across security and supply chain
	v := riskvector.Vector6D{
		Security:    90.0,
		Reliability: 40.0,
		Architecture: 20.0,
		SupplyChain: 80.0,
		Performance: 10.0,
		Compliance:  50.0,
	}

	// 1. Standard Assessment
	ctxStandard := riskvector.DefaultContext()
	scoreStandard := engine.ComputeComposite(v, ctxStandard)

	// Weighted calculation:
	// 90*0.35 + 40*0.20 + 20*0.10 + 80*0.15 + 10*0.10 + 50*0.10
	// = 31.5 + 8 + 2 + 12 + 1 + 5 = 59.5
	if scoreStandard != 59.5 {
		t.Fatalf("expected standard score 59.5, got %f", scoreStandard)
	}

	// 2. High Criticality & Degraded Completeness
	ctxHighCrit := riskvector.AssessmentContext{
		Completeness: 0.5, // 50% unanalyzed files -> penalty factor: 1 + 0.5*(1-0.5) = 1.25
		AlphaPenalty: 0.5,
		Criticality:  1.5, // Tier 1 financial payments
		Exposure:     1.2, // Public egress
	}
	scoreHigh := engine.ComputeComposite(v, ctxHighCrit)
	// 59.5 * 1.25 * 1.5 * 1.2 = 133.88
	if scoreHigh <= scoreStandard {
		t.Fatalf("expected high criticality degraded score to exceed standard score: got %f vs %f", scoreHigh, scoreStandard)
	}

	// 3. Compensating Control Attenuation (WAF SQLi rule efficacy 0.80, NetworkPolicy 0.90)
	attenuated := riskvector.AttenuateWithControls(90.0, []float64{0.80, 0.90})
	// 90.0 * 0.20 * 0.10 = 1.80
	if attenuated != 1.8 {
		t.Fatalf("expected attenuated score 1.8, got %f", attenuated)
	}

	// 4. Severity Grading
	if riskvector.SeverityGrade(85.0) != "CRITICAL" || riskvector.SeverityGrade(65.0) != "HIGH" {
		t.Fatal("unexpected severity grade mapping")
	}
}
