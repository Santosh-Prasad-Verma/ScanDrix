package spendlimit

import (
	"fmt"
	"strings"
)

// SpendLimitConfigError is the base error for invalid spend limit configurations.
type SpendLimitConfigError struct {
	Message string
}

func (e *SpendLimitConfigError) Error() string {
	return e.Message
}

// SpendLimitPriceabilityError indicates that one or more models cannot be priced.
type SpendLimitPriceabilityError struct {
	UnpriceableModels []string
}

func (e *SpendLimitPriceabilityError) Error() string {
	return fmt.Sprintf(
		"Cannot enable spend alerts: no price found for %s. Enter pricing on the BYOK config to continue.",
		strings.Join(e.UnpriceableModels, ", "),
	)
}
