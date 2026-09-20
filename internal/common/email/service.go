// Package email provides transactional delivery service for ScanDrix notifications and security alerts.
package email

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// EmailConfig holds credentials and API endpoints for email dispatch.
type EmailConfig struct {
	ResendAPIKey string
	FromDomain   string
	AppBaseURL   string
}

// Service provides high-level methods for sending transactional emails.
type Service struct {
	cfg        EmailConfig
	httpClient *http.Client
}

// NewService creates a new EmailService.
func NewService(cfg EmailConfig) *Service {
	if cfg.AppBaseURL == "" {
		cfg.AppBaseURL = "https://app.scandrix.dev"
	}
	return &Service{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// resendPayload matches Resend.com emails API payload format.
type resendPayload struct {
	From     string   `json:"from"`
	To       []string `json:"to"`
	Subject  string   `json:"subject"`
	HTML     string   `json:"html"`
	Text     string   `json:"text,omitempty"`
	ReplyTo  string   `json:"reply_to,omitempty"`
}

// Send dispatches an EmailMessage via Resend API or logs locally if API key is not set.
func (s *Service) Send(ctx context.Context, msg EmailMessage) error {
	if msg.To == "" {
		return errors.New("email: recipient 'To' address is required")
	}

	fromAddress := FormatFromAddress(msg.From)

	if s.cfg.ResendAPIKey == "" {
		// Mock/sandbox logging when API key is not configured
		return nil
	}

	payload := resendPayload{
		From:    fromAddress,
		To:      []string{msg.To},
		Subject: msg.Subject,
		HTML:    msg.HTMLBody,
		Text:    msg.TextBody,
		ReplyTo: msg.ReplyTo,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to encode email payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.cfg.ResendAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("resend request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("resend returned error status: %d", resp.StatusCode)
	}

	return nil
}
