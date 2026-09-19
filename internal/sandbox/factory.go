package sandbox

import (
	"strings"

	"github.com/scandrix/backend/internal/config"
	"github.com/scandrix/backend/internal/sandbox/e2b"
	"github.com/scandrix/backend/internal/sandbox/lease"
	"github.com/scandrix/backend/internal/sandbox/local"
	"github.com/scandrix/backend/internal/sandbox/null"
)


// FactoryConfig contains all configuration parameters required to instantiate the sandbox subsystem.
type FactoryConfig struct {
	ProviderType ProviderType
	E2B          e2b.Config
}

// NewSandboxProvider instantiates the appropriate ISandboxProvider based on configuration.
// This implements the ScanDrix SandboxModule factory logic:
// - Explicit "null"  -> NullSandboxProvider
// - Explicit "local" -> LocalSandboxProvider
// - Explicit "e2b"   -> E2BProvider
// - "auto" (default) -> Prefers E2B if APIKey is present; otherwise falls back to LocalSandboxProvider.
func NewSandboxProvider(cfg FactoryConfig) ISandboxProvider {
	normalized := ProviderType(strings.ToLower(strings.TrimSpace(string(cfg.ProviderType))))
	if normalized == "" {
		normalized = ProviderAuto
	}

	switch normalized {
	case ProviderNull:
		return null.NewNullSandboxProvider()
	case ProviderLocal:
		return local.NewLocalSandboxProvider()
	case ProviderE2B:
		return e2b.NewE2BProvider(cfg.E2B)
	case ProviderAuto:
		fallthrough
	default:
		if cfg.E2B.APIKey != "" {
			return e2b.NewE2BProvider(cfg.E2B)
		}
		return local.NewLocalSandboxProvider()
	}
}

// NewSandboxProviderFromConfig initializes an ISandboxProvider directly from application configuration.
func NewSandboxProviderFromConfig(cfg *config.Config) ISandboxProvider {
	if cfg == nil {
		return null.NewNullSandboxProvider()
	}
	fCfg := FactoryConfig{
		ProviderType: ProviderType(cfg.SandboxProvider),
		E2B: e2b.Config{
			APIKey:          cfg.E2BAPIKey,
			Domain:          cfg.E2BDomain,
			Endpoint:        cfg.E2BEndpoint,
			TemplateID:      cfg.E2BTemplateID,
			TemplateGraphID: cfg.E2BTemplateGraphID,
			ProxyHost:       cfg.E2BProxyHost,
			ProxyPort:       cfg.E2BProxyPort,
			ProxyPassword:   cfg.E2BProxyPassword,
			ProxyMethod:     cfg.E2BProxyMethod,
		},
	}
	return NewSandboxProvider(fCfg)
}

// NewSandboxLeaseManager creates an initialized lease manager adhering to ScanDrix architecture.
func NewSandboxLeaseManager(
	provider ISandboxProvider,
	repo lease.ISandboxLeaseRepository,
	cfg *config.Config,
) *lease.SandboxLeaseManager {
	return lease.NewSandboxLeaseManager(provider, repo, cfg)
}

// NewSandboxLeaseReaper creates an initialized lease reaper adhering to ScanDrix architecture.
func NewSandboxLeaseReaper(
	repo lease.ISandboxLeaseRepository,
	cfg *config.Config,
) *lease.SandboxLeaseReaper {
	return lease.NewSandboxLeaseReaper(repo, cfg)
}

