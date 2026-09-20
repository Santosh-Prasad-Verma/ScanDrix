package domain

// CockpitSource identifies the data source providing cockpit analytics.
type CockpitSource string

const (
	CockpitSourceInternal CockpitSource = "internal"
	CockpitSourceLegacyBQ CockpitSource = "legacy-bq"
)
