package pipeline

import (
	"fmt"
)

// StageMessageHelper generates user-facing commit check run status and timeline messages.
type StageMessageHelper struct{}

// SkippedWithReason formats a message with recommended user action and tech details.
func (h StageMessageHelper) SkippedWithReason(reason PipelineReason, techDetail ...string) string {
	res := reason.Message
	if reason.Action != "" {
		res += " — " + reason.Action
	}
	if len(techDetail) > 0 && techDetail[0] != "" {
		res += fmt.Sprintf(" (%s)", techDetail[0])
	}
	return res
}

// Skipped formats a simple skipped message with optional technical reason.
func (h StageMessageHelper) Skipped(userMessage, technicalReason string) string {
	if technicalReason == "" {
		return userMessage
	}
	return fmt.Sprintf("%s (Tech: %s)", userMessage, technicalReason)
}

// Error formats an execution failure message with structured error details.
func (h StageMessageHelper) Error(userMessage string, err error) string {
	if err == nil {
		return userMessage
	}
	return fmt.Sprintf("%s (Error: %s)", userMessage, err.Error())
}
