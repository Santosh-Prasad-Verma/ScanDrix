// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: send_rules_notification.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
)

// IRulesNotifier abstracts event and notification dispatching.
type IRulesNotifier interface {
	NotifyRulesGenerated(ctx context.Context, organizationID string, ruleTitles []string) error
}

// SendRulesNotificationUseCase informs organization members when new Drixy rules have been synthesized.
type SendRulesNotificationUseCase struct {
	notifier IRulesNotifier
}

// NewSendRulesNotificationUseCase constructs the notification use case.
func NewSendRulesNotificationUseCase(notifier IRulesNotifier) *SendRulesNotificationUseCase {
	return &SendRulesNotificationUseCase{notifier: notifier}
}

// Execute broadcasts rule generation notifications to eligible organization users.
func (uc *SendRulesNotificationUseCase) Execute(
	ctx context.Context,
	organizationID string,
	rules []string,
) error {
	if len(rules) == 0 || organizationID == "" || uc.notifier == nil {
		return nil
	}

	return uc.notifier.NotifyRulesGenerated(ctx, organizationID, rules)
}
