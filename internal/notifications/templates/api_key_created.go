package templates

import (
	"fmt"
	"html"
	"strings"
	"time"
)

// TeamAPIKeyCreatedDetails holds details for an API or CI/CD credential creation event.
type TeamAPIKeyCreatedDetails struct {
	RecipientName string
	OrgName       string
	KeyName       string    // e.g. "github-actions-ci-pipeline"
	KeyPrefix     string    // Truncated token, e.g. "scandrix_live_9f82...3e71"
	CreatedBy     string    // e.g. "Alex Rivera (alex@acme.dev)"
	Scopes        []string  // e.g. ["ci:scan", "pr:review", "rules:read"]
	CreatedAt     time.Time
	ManageKeysURL string    // Dashboard link to manage/revoke team API keys
}

// RenderTeamAPIKeyCreated renders a confirmation email when a new team or CLI key is created.
func RenderTeamAPIKeyCreated(d TeamAPIKeyCreatedDetails) (subject, htmlBody string) {
	name := d.RecipientName
	if name == "" {
		name = "there"
	}
	safeName := html.EscapeString(name)
	safeOrg := html.EscapeString(d.OrgName)
	safeKeyName := html.EscapeString(d.KeyName)

	keyPrefix := d.KeyPrefix
	if keyPrefix == "" {
		keyPrefix = "scandrix_live_..."
	}

	subject = fmt.Sprintf("New Team API Key Created for %s", safeOrg)
	preview := fmt.Sprintf("A new API key (%s) was generated for workspace %s.", safeKeyName, safeOrg)

	badge := RenderBadge("Security Credential", "#f3f4f6", "#111827", "#e5e7eb")

	scopesList := "Full Access"
	if len(d.Scopes) > 0 {
		var escapedScopes []string
		for _, s := range d.Scopes {
			escapedScopes = append(escapedScopes, fmt.Sprintf("<code>%s</code>", html.EscapeString(s)))
		}
		scopesList = strings.Join(escapedScopes, ", ")
	}

	rows := []InfoRow{
		{Label: "Key Name", Value: fmt.Sprintf("<strong>%s</strong>", safeKeyName)},
		{Label: "Workspace", Value: safeOrg},
		{Label: "Created By", Value: html.EscapeString(d.CreatedBy)},
		{Label: "Scopes", Value: scopesList},
	}
	if !d.CreatedAt.IsZero() {
		rows = append(rows, InfoRow{Label: "Created At", Value: d.CreatedAt.UTC().Format("Jan 02, 2006 · 15:04 MST")})
	}

	callout := `
		<strong>Security Reminder:</strong><br>
		For your protection, the full secret token was only displayed once in the dashboard and cannot be viewed again. Never share API keys in public repositories or client-side bundles.<br><br>
		If you did not authorize this key generation, revoke it immediately in your team settings.
	`
	calloutHTML := RenderCallout(callout, "#e5e7eb", "#f9fafb", "#4b5563")

	content := fmt.Sprintf(`
		%s
		<p style="margin: 0 0 14px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 16px 0;">A new team API key has been created for your ScanDrix workspace <strong>%s</strong>:</p>

		<div style="margin: 16px 0 8px 0;">
			<span style="font-size: 11px; font-weight: 700; text-transform: uppercase; letter-spacing: 0.05em; color: #6b7280;">Token Identifier</span>
			%s
		</div>

		%s
		%s
	`, badge, safeName, safeOrg, RenderCodeSnippet(keyPrefix), RenderInfoTable(rows), calloutHTML)

	htmlBody = RenderBrandLayout(preview, "Team API Key Created", content, "Manage Workspace API Keys", d.ManageKeysURL)
	return subject, htmlBody
}
