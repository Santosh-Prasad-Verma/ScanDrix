package graph_test

import (
	"testing"

	"github.com/codehound/codehound/core/pkg/graph"
	"github.com/codehound/codehound/shared/domain"
	"github.com/google/uuid"
)

func TestCallGraphAndBlastRadius(t *testing.T) {
	commitID := uuid.New()
	cg := graph.NewCallGraph(commitID)

	sym1 := domain.CodeSymbol{ID: uuid.New(), Name: "ValidateAuth", SymbolKey: "pkg.auth.ValidateAuth", Kind: "FUNCTION"}
	sym2 := domain.CodeSymbol{ID: uuid.New(), Name: "HandleLogin", SymbolKey: "pkg.api.HandleLogin", Kind: "FUNCTION"}
	sym3 := domain.CodeSymbol{ID: uuid.New(), Name: "AdminDashboard", SymbolKey: "pkg.web.AdminDashboard", Kind: "FUNCTION"}

	cg.AddSymbol(sym1)
	cg.AddSymbol(sym2)
	cg.AddSymbol(sym3)

	// AdminDashboard -> HandleLogin -> ValidateAuth
	cg.AddEdge(sym3.ID, sym2.ID, "CALLS", 1.0)
	cg.AddEdge(sym2.ID, sym1.ID, "CALLS", 1.0)

	callers := cg.GetCallers(sym1.ID)
	if len(callers) != 1 || callers[0].Name != "HandleLogin" {
		t.Errorf("expected direct caller to be HandleLogin, got %+v", callers)
	}

	blast := cg.CalculateBlastRadius(sym1.ID)
	if blast <= 0 {
		t.Errorf("expected positive blast radius score for shared auth symbol, got %d", blast)
	}
}

func TestTaintAnalysisSQLInjection(t *testing.T) {
	engine := graph.NewTaintEngine()

	vulnCode := []byte(`
package handlers

import (
	"fmt"
	"net/http"
	"github.com/gin-gonic/gin"
)

func SearchUserHandler(c *gin.Context) {
	username := c.Query("username")
	query := fmt.Sprintf("SELECT * FROM users WHERE name = '%s'", username)
	db.Query(query)
}
`)

	findings := engine.AnalyzeSource("pkg/handlers/search.go", vulnCode)
	if len(findings) == 0 {
		t.Fatalf("expected TaintEngine to detect SQL injection, got 0 findings")
	}

	f := findings[0]
	if f.CWEID != "CWE-89" {
		t.Errorf("expected CWE-89, got %s", f.CWEID)
	}
	if f.Severity != domain.FindingSeverityCritical {
		t.Errorf("expected CRITICAL severity, got %s", f.Severity)
	}
}
