package mailer_test

import (
	"context"
	"errors"
	"testing"

	"github.com/scandrix/backend/internal/auth/mailer"
	"github.com/scandrix/backend/internal/notifications/templates"
)

// TestUnconfiguredSenderFailsEveryPath pins AUDIT_REMEDIATION.md F-04.
//
// With SMTP_HOST unset, NewSender used to return a NoopSender whose methods all
// returned nil. Every email therefore reported success while being discarded:
// a user requesting a password reset saw "check your email" and was locked out
// permanently, and declined-payment and quota alerts vanished with no signal.
//
// Every method must now return ErrSMTPNotConfigured.
func TestUnconfiguredSenderFailsEveryPath(t *testing.T) {
	ctx := context.Background()
	s := mailer.NewSender(mailer.SMTPConfig{}) // no host

	cases := map[string]func() error{
		"password reset": func() error {
			return s.SendPasswordResetEmail(ctx, "u@example.test", "https://x/reset?t=1")
		},
		"email confirmation": func() error {
			return s.SendEmailConfirmation(ctx, "u@example.test", "https://x/confirm?t=1")
		},
		"new user welcome": func() error {
			return s.SendNewUserWelcomeEmail(ctx, "u@example.test", "Alice", "https://x/dash")
		},
		"subscription welcome": func() error {
			return s.SendSubscriptionWelcomeEmail(ctx, "u@example.test", "U", "Org", "pro", 1000, nil, "https://x")
		},
		"payment invoice": func() error {
			return s.SendPaymentInvoiceEmail(ctx, "u@example.test", templates.InvoiceDetails{})
		},
		"payment failed": func() error {
			return s.SendPaymentFailedEmail(ctx, "u@example.test", "U", "Org", "pro", "order-1", "card declined", "https://x/retry")
		},
		"spend limit alert": func() error {
			return s.SendSpendLimitAlertEmail(ctx, "u@example.test", "U", "Org", 90, 900, 1000, "https://x/upgrade")
		},
		"team invite": func() error {
			return s.SendTeamInviteEmail(ctx, "u@example.test", "Inviter", "Org", "member", "https://x/invite")
		},
	}

	for name, send := range cases {
		t.Run(name, func(t *testing.T) {
			err := send()
			if err == nil {
				t.Fatalf("%s reported success with no SMTP configured", name)
			}
			if !errors.Is(err, mailer.ErrSMTPNotConfigured) {
				t.Fatalf("%s: expected ErrSMTPNotConfigured, got %v", name, err)
			}
		})
	}
}

// TestNewSenderIsNeverTheRecordingSender asserts the factory can never return a
// sender that swallows messages in a live deployment.
func TestNewSenderIsNeverTheRecordingSender(t *testing.T) {
	s := mailer.NewSender(mailer.SMTPConfig{})
	if _, isNoop := s.(*mailer.NoopSender); isNoop {
		t.Fatalf("NewSender returned a recording sender for an unconfigured host; " +
			"that silently discards transactional email (AUDIT F-04)")
	}

	configured := mailer.NewSender(mailer.SMTPConfig{Host: "smtp.example.test", Port: 587, From: "a@b.test"})
	if _, isNoop := configured.(*mailer.NoopSender); isNoop {
		t.Fatalf("NewSender returned a recording sender even with a host configured")
	}
}
