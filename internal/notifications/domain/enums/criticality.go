package enums

// Criticality categorizes the urgency and operational severity of a notification.
type Criticality string

const (
	CriticalitySystem        Criticality = "system"
	CriticalityCritical      Criticality = "critical"
	CriticalityTransactional Criticality = "transactional"
	CriticalityInformational Criticality = "informational"
)
