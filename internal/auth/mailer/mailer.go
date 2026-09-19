package mailer

import (
	"context"
	"crypto/tls"
	"fmt"
	"html"
	"log/slog"
	"net"
	"net/smtp"
	"strings"

	"github.com/scandrix/backend/internal/notifications/templates"
)

// EmailSender defines the contract for sending transactional and commercial emails.
type EmailSender interface {
	SendPasswordResetEmail(ctx context.Context, recipientEmail, resetURL string) error
	SendEmailConfirmation(ctx context.Context, recipientEmail, confirmURL string) error
	SendSubscriptionWelcomeEmail(ctx context.Context, recipientEmail, subscriberName, orgName, planTier string, monthlyTokens int64, allocatedModels []string, dashboardURL string) error
	SendPaymentInvoiceEmail(ctx context.Context, recipientEmail string, invoice templates.InvoiceDetails) error
	SendPaymentFailedEmail(ctx context.Context, recipientEmail, subscriberName, orgName, planTier, orderID, failureReason, retryURL string) error
	SendSpendLimitAlertEmail(ctx context.Context, recipientEmail, subscriberName, orgName string, percent int, usedTokens, limitTokens int64, upgradeURL string) error
	SendTeamInviteEmail(ctx context.Context, recipientEmail, inviterName, orgName, role, inviteURL string) error
}

// SMTPConfig holds credentials and server connection details for SMTP.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// SMTPSender implements EmailSender using standard SMTP transport.
type SMTPSender struct {
	cfg SMTPConfig
}

// NewSMTPSender creates a sender connected to an SMTP server.
func NewSMTPSender(cfg SMTPConfig) *SMTPSender {
	if cfg.Port <= 0 {
		cfg.Port = 587
	}
	if cfg.From == "" {
		cfg.From = "no-reply@scandrix.dev"
	}
	return &SMTPSender{cfg: cfg}
}

// sanitizeHeader strips carriage returns and line feeds to prevent CRLF injection / SMTP smuggling.
func sanitizeHeader(val string) string {
	val = strings.ReplaceAll(val, "\r", "")
	val = strings.ReplaceAll(val, "\n", "")
	return strings.TrimSpace(val)
}

func (s *SMTPSender) sendHTML(ctx context.Context, recipientEmail, subject, htmlBody string) error {
	if s.cfg.Host == "" {
		slog.Info("SMTP host not configured; skipping email dispatch", "recipient", recipientEmail, "subject", subject)
		return nil
	}

	cleanSubj := sanitizeHeader(subject)
	cleanFrom := sanitizeHeader(s.cfg.From)
	cleanTo := sanitizeHeader(recipientEmail)

	if cleanTo == "" || !strings.Contains(cleanTo, "@") {
		return fmt.Errorf("invalid recipient email address: %q", recipientEmail)
	}

	subjectHeader := fmt.Sprintf("Subject: %s\r\n", cleanSubj)
	fromHeader := fmt.Sprintf("From: ScanDrix Platform <%s>\r\n", cleanFrom)
	toHeader := fmt.Sprintf("To: %s\r\n", cleanTo)
	mimeHeader := "MIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n"

	msg := []byte(fromHeader + toHeader + subjectHeader + mimeHeader + htmlBody)
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)

	// Context-aware dialer supporting cancellation and timeouts
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("failed connecting to smtp server: %w", err)
	}

	var c *smtp.Client
	if s.cfg.Port == 465 {
		// Implicit TLS (SMTPS)
		tlsConfig := &tls.Config{ServerName: s.cfg.Host}
		tlsConn := tls.Client(conn, tlsConfig)
		c, err = smtp.NewClient(tlsConn, s.cfg.Host)
	} else {
		// Plain connection with explicit STARTTLS upgrade if supported
		c, err = smtp.NewClient(conn, s.cfg.Host)
		if err == nil {
			if ok, _ := c.Extension("STARTTLS"); ok {
				tlsConfig := &tls.Config{ServerName: s.cfg.Host}
				if err := c.StartTLS(tlsConfig); err != nil {
					c.Close()
					return fmt.Errorf("smtp starttls upgrade failed: %w", err)
				}
			}
		}
	}
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed creating smtp client: %w", err)
	}
	defer c.Close()

	if s.cfg.Username != "" && s.cfg.Password != "" {
		auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
		if ok, _ := c.Extension("AUTH"); ok {
			if err := c.Auth(auth); err != nil {
				return fmt.Errorf("smtp authentication failed: %w", err)
			}
		}
	}

	if err := c.Mail(cleanFrom); err != nil {
		return fmt.Errorf("smtp mail from failed: %w", err)
	}
	if err := c.Rcpt(cleanTo); err != nil {
		return fmt.Errorf("smtp rcpt to failed: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp data initiation failed: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return fmt.Errorf("smtp write data failed: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close data failed: %w", err)
	}
	_ = c.Quit()

	slog.Info("Email dispatched successfully", "recipient", cleanTo, "subject", cleanSubj)
	return nil
}

// SendPasswordResetEmail sends a password reset link to the recipient via SMTP.
func (s *SMTPSender) SendPasswordResetEmail(ctx context.Context, recipientEmail, resetURL string) error {
	subj, body := templates.RenderPasswordReset("", resetURL)
	return s.sendHTML(ctx, recipientEmail, subj, body)
}

// SendEmailConfirmation sends an email confirmation link via SMTP.
func (s *SMTPSender) SendEmailConfirmation(ctx context.Context, recipientEmail, confirmURL string) error {
	subj := "Confirm your ScanDrix Account"
	safeURL := html.EscapeString(confirmURL)
	body := fmt.Sprintf(`<html><body><h2>Confirm your Email</h2><p>Please confirm your email by clicking <a href="%s">here</a>.</p></body></html>`, safeURL)
	return s.sendHTML(ctx, recipientEmail, subj, body)
}

// SendSubscriptionWelcomeEmail sends an onboarding email with unlocked frontier models and quotas.
func (s *SMTPSender) SendSubscriptionWelcomeEmail(ctx context.Context, recipientEmail, subscriberName, orgName, planTier string, monthlyTokens int64, allocatedModels []string, dashboardURL string) error {
	subj, body := templates.RenderSubscriptionWelcome(subscriberName, orgName, planTier, monthlyTokens, allocatedModels, dashboardURL)
	return s.sendHTML(ctx, recipientEmail, subj, body)
}

// SendPaymentInvoiceEmail sends an itemized payment receipt and invoice to the subscriber.
func (s *SMTPSender) SendPaymentInvoiceEmail(ctx context.Context, recipientEmail string, invoice templates.InvoiceDetails) error {
	subj, body := templates.RenderInvoiceReceipt(invoice)
	return s.sendHTML(ctx, recipientEmail, subj, body)
}

// SendPaymentFailedEmail notifies the subscriber of a declined payment with retry instructions.
func (s *SMTPSender) SendPaymentFailedEmail(ctx context.Context, recipientEmail, subscriberName, orgName, planTier, orderID, failureReason, retryURL string) error {
	subj, body := templates.RenderPaymentFailed(subscriberName, orgName, planTier, orderID, failureReason, retryURL)
	return s.sendHTML(ctx, recipientEmail, subj, body)
}

// SendSpendLimitAlertEmail alerts admins when token consumption reaches threshold caps (e.g. 80% or 100%).
func (s *SMTPSender) SendSpendLimitAlertEmail(ctx context.Context, recipientEmail, subscriberName, orgName string, percent int, usedTokens, limitTokens int64, upgradeURL string) error {
	subj, body := templates.RenderSpendLimitAlert(subscriberName, orgName, percent, usedTokens, limitTokens, upgradeURL)
	return s.sendHTML(ctx, recipientEmail, subj, body)
}

// SendTeamInviteEmail sends an invitation to join a workspace.
func (s *SMTPSender) SendTeamInviteEmail(ctx context.Context, recipientEmail, inviterName, orgName, role, inviteURL string) error {
	subj, body := templates.RenderTeamInvite(inviterName, recipientEmail, orgName, role, inviteURL)
	return s.sendHTML(ctx, recipientEmail, subj, body)
}

// NoopSender is a fallback that logs dispatch events without network requests (for testing/dev).
type NoopSender struct {
	LastRecipient string
	LastSubject   string
	LastResetURL  string
	LastPlanTier  string
	LastInvoice   *templates.InvoiceDetails
}

// NewNoopSender creates a no-op email sender.
func NewNoopSender() *NoopSender {
	return &NoopSender{}
}

func (n *NoopSender) SendPasswordResetEmail(_ context.Context, recipientEmail, resetURL string) error {
	n.LastRecipient = recipientEmail
	n.LastResetURL = resetURL
	n.LastSubject = "Reset your ScanDrix Password"
	slog.Info("Mock email dispatch", "recipient", recipientEmail, "action", "password_reset")
	return nil
}

func (n *NoopSender) SendEmailConfirmation(_ context.Context, recipientEmail, confirmURL string) error {
	n.LastRecipient = recipientEmail
	n.LastSubject = "Confirm your ScanDrix Account"
	slog.Info("Mock email dispatch", "recipient", recipientEmail, "action", "confirm_email")
	return nil
}

func (n *NoopSender) SendSubscriptionWelcomeEmail(_ context.Context, recipientEmail, subscriberName, orgName, planTier string, monthlyTokens int64, allocatedModels []string, dashboardURL string) error {
	n.LastRecipient = recipientEmail
	n.LastPlanTier = planTier
	n.LastSubject = fmt.Sprintf("Welcome to ScanDrix %s", planTier)
	slog.Info("Mock email dispatch", "recipient", recipientEmail, "action", "subscription_welcome", "plan", planTier)
	return nil
}

func (n *NoopSender) SendPaymentInvoiceEmail(_ context.Context, recipientEmail string, invoice templates.InvoiceDetails) error {
	n.LastRecipient = recipientEmail
	n.LastInvoice = &invoice
	n.LastSubject = fmt.Sprintf("Your ScanDrix Invoice %s", invoice.InvoiceNumber)
	slog.Info("Mock email dispatch", "recipient", recipientEmail, "action", "payment_invoice", "invoice", invoice.InvoiceNumber)
	return nil
}

func (n *NoopSender) SendPaymentFailedEmail(_ context.Context, recipientEmail, subscriberName, orgName, planTier, orderID, failureReason, retryURL string) error {
	n.LastRecipient = recipientEmail
	n.LastSubject = "Payment failed for ScanDrix"
	slog.Info("Mock email dispatch", "recipient", recipientEmail, "action", "payment_failed", "order", orderID)
	return nil
}

func (n *NoopSender) SendSpendLimitAlertEmail(_ context.Context, recipientEmail, subscriberName, orgName string, percent int, usedTokens, limitTokens int64, upgradeURL string) error {
	n.LastRecipient = recipientEmail
	n.LastSubject = fmt.Sprintf("Token quota alert (%d%%)", percent)
	slog.Info("Mock email dispatch", "recipient", recipientEmail, "action", "spend_limit_alert", "percent", percent)
	return nil
}

func (n *NoopSender) SendTeamInviteEmail(_ context.Context, recipientEmail, inviterName, orgName, role, inviteURL string) error {
	n.LastRecipient = recipientEmail
	n.LastSubject = fmt.Sprintf("Invitation to join %s", orgName)
	slog.Info("Mock email dispatch", "recipient", recipientEmail, "action", "team_invite", "org", orgName)
	return nil
}

// NewSender constructs the appropriate EmailSender based on configuration.
func NewSender(cfg SMTPConfig) EmailSender {
	if strings.TrimSpace(cfg.Host) != "" {
		return NewSMTPSender(cfg)
	}
	return NewNoopSender()
}
