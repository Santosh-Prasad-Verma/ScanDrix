package sandbox_test

import (
	"testing"

	"github.com/scandrix/backend/internal/sandbox"
	"github.com/scandrix/backend/internal/sandbox/e2b"
	"github.com/scandrix/backend/internal/sandbox/local"
	"github.com/scandrix/backend/internal/sandbox/null"
)

func TestNewSandboxProvider(t *testing.T) {
	// 1. Explicit Null provider
	pNull := sandbox.NewSandboxProvider(sandbox.FactoryConfig{
		ProviderType: sandbox.ProviderNull,
	})
	if _, ok := pNull.(*null.NullSandboxProvider); !ok {
		t.Fatalf("expected *null.NullSandboxProvider, got %T", pNull)
	}

	// 2. Explicit Local provider
	pLocal := sandbox.NewSandboxProvider(sandbox.FactoryConfig{
		ProviderType: sandbox.ProviderLocal,
	})
	if _, ok := pLocal.(*local.LocalSandboxProvider); !ok {
		t.Fatalf("expected *local.LocalSandboxProvider, got %T", pLocal)
	}

	// 3. Explicit E2B provider with API key
	pE2B := sandbox.NewSandboxProvider(sandbox.FactoryConfig{
		ProviderType: sandbox.ProviderE2B,
		E2B: e2b.Config{
			APIKey: "e2b_test_key",
		},
	})
	if _, ok := pE2B.(*e2b.E2BProvider); !ok {
		t.Fatalf("expected *e2b.E2BProvider, got %T", pE2B)
	}

	// 4. Auto mode with API key -> prefers E2B
	pAutoWithKey := sandbox.NewSandboxProvider(sandbox.FactoryConfig{
		ProviderType: sandbox.ProviderAuto,
		E2B: e2b.Config{
			APIKey: "e2b_test_key",
		},
	})
	if _, ok := pAutoWithKey.(*e2b.E2BProvider); !ok {
		t.Fatalf("expected *e2b.E2BProvider for auto with key, got %T", pAutoWithKey)
	}

	// 5. Auto mode without API key -> falls back to Local
	pAutoNoKey := sandbox.NewSandboxProvider(sandbox.FactoryConfig{
		ProviderType: sandbox.ProviderAuto,
		E2B: e2b.Config{
			APIKey: "",
		},
	})
	if _, ok := pAutoNoKey.(*local.LocalSandboxProvider); !ok {
		t.Fatalf("expected *local.LocalSandboxProvider for auto without key, got %T", pAutoNoKey)
	}

	// 6. Empty provider string defaults to Auto
	pEmpty := sandbox.NewSandboxProvider(sandbox.FactoryConfig{
		ProviderType: "",
	})
	if _, ok := pEmpty.(*local.LocalSandboxProvider); !ok {
		t.Fatalf("expected *local.LocalSandboxProvider for empty provider type, got %T", pEmpty)
	}

	// 7. Factory helpers for LeaseManager and LeaseReaper
	mgr := sandbox.NewSandboxLeaseManager(pNull, nil, nil)
	if mgr == nil {
		t.Fatal("expected non-nil SandboxLeaseManager")
	}

	reaper := sandbox.NewSandboxLeaseReaper(nil, nil)
	if reaper == nil {
		t.Fatal("expected non-nil SandboxLeaseReaper")
	}
}

