// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Platform Data Subsystem
// Package: enums
// File: enums.go
// ═══════════════════════════════════════════════════════════════

package enums

// DeliveryStatus tracks whether a generated suggestion reached the SCM pull request.
type DeliveryStatus string

const (
	DeliveryStatusSent                DeliveryStatus = "sent"
	DeliveryStatusNotSent             DeliveryStatus = "not_sent"
	DeliveryStatusFailedLinesMismatch DeliveryStatus = "failed_lines_mismatch"
	DeliveryStatusFailed              DeliveryStatus = "failed"
	DeliveryStatusReplaced            DeliveryStatus = "replaced"
)

// ImplementationStatus captures whether the developer accepted or addressed the suggestion.
type ImplementationStatus string

const (
	ImplementationStatusImplemented          ImplementationStatus = "implemented"
	ImplementationStatusPartiallyImplemented ImplementationStatus = "partially_implemented"
	ImplementationStatusNotImplemented       ImplementationStatus = "not_implemented"
)

// PriorityStatus identifies how a suggestion was filtered, ranked, or suppressed during review.
type PriorityStatus string

const (
	PriorityStatusPrioritized             PriorityStatus = "prioritized"
	PriorityStatusPrioritizedByClustering PriorityStatus = "prioritized-by-clustering"
	PriorityStatusReprioritized           PriorityStatus = "repriorized"
	PriorityStatusDiscardedBySeverity     PriorityStatus = "discarded-by-severity"
	PriorityStatusDiscardedByQuantity     PriorityStatus = "discarded-by-quantity"
	PriorityStatusDiscardedByClustering   PriorityStatus = "discarded-by-clustering"
	PriorityStatusDiscardedBySafeguard    PriorityStatus = "discarded-by-safeguard"
	PriorityStatusDiscardedByCodeDiff     PriorityStatus = "discarded-by-code-diff"
	PriorityStatusDiscardedByDrixyFineTuning PriorityStatus = "discarded-by-drixy-fine-tuning"
)
