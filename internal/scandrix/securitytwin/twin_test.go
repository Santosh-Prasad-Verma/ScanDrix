package securitytwin_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/scandrix/securitytwin"
)

func TestSecurityTwinContinuumAndAttenuation(t *testing.T) {
	wsID := uuid.New()
	twin := securitytwin.NewSecurityTwin(wsID)

	// 1. Build 7-Hop Continuum Chain
	_ = twin.AddNode(&securitytwin.ContinuumNode{
		ID:   "ast_sql_sink",
		Hop:  securitytwin.HopCodeAST,
		Name: "db.Query(userInput)",
	})
	_ = twin.AddNode(&securitytwin.ContinuumNode{
		ID:       "service_package",
		Hop:      securitytwin.HopBuildPackage,
		Name:     "orders-api",
		ParentID: "ast_sql_sink",
	})
	_ = twin.AddNode(&securitytwin.ContinuumNode{
		ID:       "public_ingress",
		Hop:      securitytwin.HopRuntimeIngress,
		Name:     "api.acme.com:443",
		ParentID: "service_package",
		IsPublic: true,
	})

	isReachable, path := twin.TraceReachability("ast_sql_sink")
	if !isReachable || len(path) != 3 {
		t.Fatalf("expected reachability to public ingress through continuum: reachable=%v, path=%v", isReachable, path)
	}

	// 2. Register Runtime Mitigations
	_ = twin.RegisterMitigation(&securitytwin.RuntimeMitigation{
		ID:          "waf_sqli_rule",
		Category:    securitytwin.ControlWAF,
		Description: "AWS WAF SQLi Core Rule Set active on ALB",
		Efficacy:    0.80, // Blocks 80% of SQLi attacks at edge
		Active:      true,
		VerifiedAt:  time.Now(),
	})
	_ = twin.RegisterMitigation(&securitytwin.RuntimeMitigation{
		ID:          "netpol_db_isolation",
		Category:    securitytwin.ControlNetworkPolicy,
		Description: "Calico NetworkPolicy restricting egress to internal RDS port only",
		Efficacy:    0.90,
		Active:      true,
		VerifiedAt:  time.Now(),
	})

	// 3. CVSS Attenuation
	baseCVSS := 9.8 // Critical unauthenticated SQLi
	attenuated := twin.AttenuateRisk(baseCVSS, []string{"waf_sqli_rule", "netpol_db_isolation"}, 0.0)

	// 9.8 * (1 - 0.80) * (1 - 0.90) = 9.8 * 0.20 * 0.10 = 0.20
	if attenuated != 0.20 {
		t.Fatalf("expected attenuated score 0.20, got %f", attenuated)
	}
}
