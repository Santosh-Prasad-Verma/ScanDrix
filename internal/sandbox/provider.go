package sandbox

import (
	"github.com/scandrix/backend/internal/sandbox/contracts"
)

type (
	SandboxTier         = contracts.SandboxTier
	ProviderType        = contracts.ProviderType
	CreateSandboxParams = contracts.CreateSandboxParams
	SandboxRunResult    = contracts.SandboxRunResult
	RemoteCommands      = contracts.RemoteCommands
	SandboxInstance     = contracts.SandboxInstance
	ISandboxProvider    = contracts.ISandboxProvider
	CommandRequest      = contracts.CommandRequest
	CommandResult       = contracts.CommandResult
	ISandbox            = contracts.ISandbox

	// Lease management
	ISandboxLeaseManager = contracts.ISandboxLeaseManager
	AcquireResult        = contracts.AcquireResult
	ReleaseOptions       = contracts.ReleaseOptions
	DecomposedPrKey      = contracts.DecomposedPrKey
)

type (
	SandboxInvalidatedError     = contracts.SandboxInvalidatedError
	SandboxCreateTimeoutError   = contracts.SandboxCreateTimeoutError
	SandboxStaleConnectionError = contracts.SandboxStaleConnectionError
)

const (
	TierWorktree = contracts.TierWorktree
	TierMicroVM  = contracts.TierMicroVM
	TierNull     = contracts.TierNull

	ProviderAuto  = contracts.ProviderAuto
	ProviderE2B   = contracts.ProviderE2B
	ProviderLocal = contracts.ProviderLocal
	ProviderNull  = contracts.ProviderNull

	RoutingKeySandboxInvalidate = contracts.RoutingKeySandboxInvalidate
	ReasonPRClosed              = contracts.ReasonPRClosed
	ReasonForcePushed           = contracts.ReasonForcePushed
)

var (
	ResolveRepoPath = contracts.ResolveRepoPath
	ShSingleQuote   = contracts.ShSingleQuote
	BuildAuthHeader = contracts.BuildAuthHeader
	GetPRRefspec    = contracts.GetPRRefspec

	BuildPrKey      = contracts.BuildPrKey
	AssertValidPrKey = contracts.AssertValidPrKey
	DecomposePrKey   = contracts.DecomposePrKey
)

