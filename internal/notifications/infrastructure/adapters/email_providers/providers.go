package email_providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/smtp"
	"strings"
	"time"
)

// EmailMessage holds all data for dispatching an email through a provider.
type EmailMessage struct {
	From    string   `json:"from"`
	To      string   `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
	Text    string   `json:"text,omitempty"`
	ReplyTo string   `json:"reply_to,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// EmailProvider abstracts transactional email vendors (Resend, SMTP, etc.).
type EmailProvider interface {
	Send(ctx context.Context, msg EmailMessage) error
}

// ResendEmailProvider delivers transactional messages using the Resend REST API.
type ResendEmailProvider struct {
	apiKey     string
	httpClient *http.Client
	apiBaseURL string
}

// NewResendEmailProvider creates an authenticated Resend email client.
func NewResendEmailProvider(apiKey string) *ResendEmailProvider {
	return &ResendEmailProvider{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		apiBaseURL: "https://api.resend.com",
	}
}

// Send submits the email payload to the Resend API.
func (p *ResendEmailProvider) Send(ctx context.Context, msg EmailMessage) error {
	if p.apiKey == "" {
		return fmt.Errorf("resend API key is missing")
	}

	payload := map[string]interface{}{
		"from":    msg.From,
		"to":      []string{msg.To},
		"subject": msg.Subject,
		"html":    msg.HTML,
	}
	if msg.Text != "" {
		payload["text"] = msg.Text
	}
	if msg.ReplyTo != "" {
		payload["reply_to"] = msg.ReplyTo
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal resend email body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiBaseURL+"/emails", bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create resend request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("resend request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("resend API returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// SmtpConfig contains network parameters for an SMTP relay server.
type SmtpConfig struct {
	Host     string
	Port     int
	Username string
	Password string
}

// SmtpEmailProvider delivers transactional messages over standard SMTP.
type SmtpEmailProvider struct {
	config SmtpConfig
}

// NewSmtpEmailProvider creates an SMTP email provider.
func NewSmtpEmailProvider(config SmtpConfig) *SmtpEmailProvider {
	return &SmtpEmailProvider{config: config}
}

// Send connects to the configured SMTP server and sends the message.
func (p *SmtpEmailProvider) Send(ctx context.Context, msg EmailMessage) error {
	addr := fmt.Sprintf("%s:%d", p.config.Host, p.config.Port)
	var auth smtp.Auth
	if p.config.Username != "" {
		auth = smtp.PlainAuth("", p.config.Username, p.config.Password, p.config.Host)
	}

	header := make(map[string]string)
	header["From"] = msg.From
	header["To"] = msg.To
	header["Subject"] = msg.Subject
	header["MIME-Version"] = "1.0"
	header["Content-Type"] = "text/html; charset=UTF-8"
	if msg.ReplyTo != "" {
		header["Reply-To"] = msg.ReplyTo
	}

	var message strings.Builder
	for k, v := range header {
		message.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	message.WriteString("\r\n" + msg.HTML)

	return smtp.SendMail(addr, auth, msg.From, []string{msg.To}, []byte(message.String()))
}

// MemoryEmailProvider records sent messages in memory for unit testing.
type MemoryEmailProvider struct {
	Sent []EmailMessage
}

// NewMemoryEmailProvider creates an in-memory email test provider.
func NewMemoryEmailProvider() *MemoryEmailProvider {
	return &MemoryEmailProvider{Sent: make([]EmailMessage, 0)}
}

// Send records the email message in the memory slice.
func (p *MemoryEmailProvider) Send(ctx context.Context, msg EmailMessage) error {
	p.Sent = append(p.Sent, msg)
	return nil
}
