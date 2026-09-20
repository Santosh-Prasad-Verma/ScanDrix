package enums

// DeliveryStatus tracks dispatch outcomes and lifecycle transitions.
type DeliveryStatus string

const (
	StatusPending     DeliveryStatus = "pending"
	StatusDelivered   DeliveryStatus = "delivered"
	StatusFailed      DeliveryStatus = "failed"
	StatusRateLimited DeliveryStatus = "rate_limited"
	StatusSkipped     DeliveryStatus = "skipped"
)
