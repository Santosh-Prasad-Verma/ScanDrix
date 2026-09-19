package worker

import (
	"fmt"
	"strings"
)

// WorkerRole defines the execution mode of the asynchronous worker daemon.
type WorkerRole string

const (
	// RoleCodeReview runs RabbitMQ consumers, code review execution pipeline,
	// outbox relay, and code review monitoring crons.
	RoleCodeReview WorkerRole = "code-review"

	// RoleAnalytics runs analytics ingestion crons and DORA metric aggregators.
	// RabbitMQ consumers are disabled in this mode.
	RoleAnalytics WorkerRole = "analytics"

	// RoleAll runs both code review queues and analytics/maintenance crons (default).
	RoleAll WorkerRole = "all"
)

// ResolveWorkerRole validates and normalizes the WORKER_ROLE environment variable.
func ResolveWorkerRole(raw string) (WorkerRole, error) {
	clean := strings.ToLower(strings.TrimSpace(raw))
	if clean == "" {
		return RoleAll, nil
	}

	switch WorkerRole(clean) {
	case RoleCodeReview:
		return RoleCodeReview, nil
	case RoleAnalytics:
		return RoleAnalytics, nil
	case RoleAll:
		return RoleAll, nil
	default:
		return "", fmt.Errorf("WORKER_ROLE must be set to 'code-review', 'analytics', or 'all'. Got %q", raw)
	}
}
