package graph_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/codeanalysis/graph"
)

func TestGraphIndexerSymbolExtractionAndCallGraph(t *testing.T) {
	ctx := context.Background()
	indexer := graph.NewGraphIndexer()
	repoID := uuid.New()

	sampleGoCode := `package payment

func ValidateCard(number string) bool {
	return len(number) == 16
}

func ChargeCustomer(amount int) error {
	if !ValidateCard("4111111111111111") {
		return nil
	}
	return nil
}

func ExecuteCheckout() {
	ChargeCustomer(100)
}
`

	nodes, err := indexer.IndexGoFile(ctx, repoID, "payment.go", sampleGoCode)
	if err != nil {
		t.Fatalf("failed indexing Go file: %v", err)
	}

	if len(nodes) != 3 {
		t.Fatalf("expected 3 function nodes, got %d", len(nodes))
	}

	// 1. Verify symbol names
	symbols := make(map[string]uuid.UUID)
	for _, n := range nodes {
		symbols[n.SymbolName] = n.ID
	}

	validateID, ok1 := symbols["ValidateCard"]
	chargeID, ok2 := symbols["ChargeCustomer"]
	checkoutID, ok3 := symbols["ExecuteCheckout"]

	if !ok1 || !ok2 || !ok3 {
		t.Fatalf("missing expected symbols: ValidateCard=%v, ChargeCustomer=%v, ExecuteCheckout=%v", ok1, ok2, ok3)
	}

	// 2. Query callers of ValidateCard -> Should be ChargeCustomer
	callersOfValidate := indexer.QueryCallers(validateID)
	if len(callersOfValidate) != 1 {
		t.Fatalf("expected 1 caller of ValidateCard, got %d", len(callersOfValidate))
	}
	if callersOfValidate[0].SymbolName != "ChargeCustomer" {
		t.Errorf("expected caller to be ChargeCustomer, got %s", callersOfValidate[0].SymbolName)
	}

	// 3. Query callers of ChargeCustomer -> Should be ExecuteCheckout
	callersOfCharge := indexer.QueryCallers(chargeID)
	if len(callersOfCharge) != 1 {
		t.Fatalf("expected 1 caller of ChargeCustomer, got %d", len(callersOfCharge))
	}
	if callersOfCharge[0].SymbolName != "ExecuteCheckout" {
		t.Errorf("expected caller to be ExecuteCheckout, got %s", callersOfCharge[0].SymbolName)
	}

	// 4. Query callers of ExecuteCheckout -> Should be 0
	callersOfCheckout := indexer.QueryCallers(checkoutID)
	if len(callersOfCheckout) != 0 {
		t.Errorf("expected 0 callers of ExecuteCheckout, got %d", len(callersOfCheckout))
	}
}
