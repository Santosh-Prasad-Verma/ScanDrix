package mailer_test

import (
	"context"
	"errors"
	"testing"

	"github.com/scandrix/backend/internal/auth/mailer"
	"github.com/scandrix/backend/internal/notifications/templates"
)

func TestNoopSender(t *testing.T) {
	ctx := context.Background()
	sender := mailer.NewNoopSender()

	err := sender.SendPasswordResetEmail(ctx, "dev@scandrix.dev", "https://app.scandrix.dev/reset-password?token=secret123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sender.LastRecipient != "dev@scandrix.dev" {
		t.Errorf("expected recipient dev@scandrix.dev, got %s", sender.LastRecipient)
	}
	if sender.LastResetURL != "https://app.scandrix.dev/reset-password?token=secret123" {
		t.Errorf("unexpected reset URL: %s", sender.LastResetURL)
	}

	// Test new user welcome email
	err = sender.SendNewUserWelcomeEmail(ctx, "dev@scandrix.dev", "Alice", "https://app.scandrix.dev/dashboard")
	if err != nil {
		t.Fatalf("unexpected new user welcome error: %v", err)
	}
	if sender.LastRecipient != "dev@scandrix.dev" || sender.LastSubject != "Welcome to ScanDrix" {
		t.Errorf("unexpected new user welcome state: recipient=%s subject=%s", sender.LastRecipient, sender.LastSubject)
	}

	// Test subscription welcome email
	err = sender.SendSubscriptionWelcomeEmail(ctx, "dev@scandrix.dev", "Alice", "Acme", "TEAM", 10000000, []string{"claude-sonnet-5"}, "https://app.scandrix.dev")
	if err != nil {
		t.Fatalf("unexpected welcome error: %v", err)
	}
	if sender.LastPlanTier != "TEAM" {
		t.Errorf("expected plan tier TEAM, got %s", sender.LastPlanTier)
	}

	// Test invoice email
	err = sender.SendPaymentInvoiceEmail(ctx, "dev@scandrix.dev", templates.InvoiceDetails{
		InvoiceNumber:   "INV-2026-001",
		AmountFormatted: "₹2,499.00 INR",
	})
	if err != nil {
		t.Fatalf("unexpected invoice error: %v", err)
	}
	if sender.LastInvoice == nil || sender.LastInvoice.InvoiceNumber != "INV-2026-001" {
		t.Errorf("expected invoice INV-2026-001")
	}

	// Test payment failed
	_ = sender.SendPaymentFailedEmail(ctx, "dev@scandrix.dev", "Alice", "Acme", "TEAM", "ord_1", "Card declined", "https://app.scandrix.dev")
	if sender.LastSubject != "Payment failed for ScanDrix" {
		t.Errorf("unexpected subject: %s", sender.LastSubject)
	}

	// Test spend limit alert
	_ = sender.SendSpendLimitAlertEmail(ctx, "dev@scandrix.dev", "Alice", "Acme", 80, 8000000, 10000000, "https://app.scandrix.dev")
	if sender.LastRecipient != "dev@scandrix.dev" {
		t.Errorf("unexpected recipient: %s", sender.LastRecipient)
	}

	// Test team invite
	_ = sender.SendTeamInviteEmail(ctx, "colleague@scandrix.dev", "Alice", "Acme", "Engineer", "https://app.scandrix.dev/invite")
	if sender.LastRecipient != "colleague@scandrix.dev" {
		t.Errorf("unexpected recipient: %s", sender.LastRecipient)
	}
}

func TestNewSenderFactory(t *testing.T) {
	ctx := context.Background()

	// 1. Without host -> returns UnconfiguredSender, whose sends all fail.
	//
	// It used to return a NoopSender, which returned nil from every send, so a
	// deployment with no SMTP silently discarded password resets and email
	// confirmations while reporting success (AUDIT_REMEDIATION.md F-04).
	s1 := mailer.NewSender(mailer.SMTPConfig{})
	if _, ok := s1.(mailer.UnconfiguredSender); !ok {
		t.Fatalf("expected mailer.UnconfiguredSender when host is empty, got %T", s1)
	}
	if err := s1.SendPasswordResetEmail(ctx, "a@b.test", "https://x"); !errors.Is(err, mailer.ErrSMTPNotConfigured) {
		t.Fatalf("expected ErrSMTPNotConfigured, got %v", err)
	}

	// 2. With host -> returns SMTPSender
	s2 := mailer.NewSender(mailer.SMTPConfig{
		Host: "smtp.sendgrid.net",
		Port: 587,
		From: "security@scandrix.dev",
	})
	if _, ok := s2.(*mailer.SMTPSender); !ok {
		t.Errorf("expected *mailer.SMTPSender when host is set, got %T", s2)
	}
}
