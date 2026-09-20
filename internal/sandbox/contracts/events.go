package contracts

// RoutingKeySandboxInvalidate is the AMQP routing key for invalidating sandbox instances.
const RoutingKeySandboxInvalidate = "sandbox.invalidate"

// SandboxInvalidateReason represents the event triggering the sandbox invalidation.
type SandboxInvalidateReason string

const (
	ReasonPRClosed    SandboxInvalidateReason = "pr_closed"
	ReasonForcePushed SandboxInvalidateReason = "force_pushed"
)

// SandboxInvalidatePayload is the event payload dispatched when a PR is closed or force-pushed.
type SandboxInvalidatePayload struct {
	PrKey  string                  `json:"pr_key"` // Canonical "{organizationId}:{repositoryId}:{prNumber}"
	Reason SandboxInvalidateReason `json:"reason"`
}
