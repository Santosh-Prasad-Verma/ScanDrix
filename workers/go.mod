module github.com/codehound/codehound/workers

go 1.26.0

replace github.com/codehound/codehound/shared => ../shared

require (
	github.com/codehound/codehound/shared v0.0.0-00010101000000-000000000000
	github.com/google/uuid v1.6.0
)
