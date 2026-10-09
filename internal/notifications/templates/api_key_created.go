package templates

import (
	"fmt"
	"html"
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

	content := fmt.Sprintf(`
		<p style="margin: 0 0 14px 0;">Hi <strong>%s</strong>,</p>
		<p style="margin: 0 0 14px 0; color: #cbd5e1; line-height: 1.6;">A new team API key (<strong>%s</strong>) was created for workspace <strong>%s</strong>:</p>
		<p style="margin: 0 0 16px 0; font-family: monospace; font-size: 14px; color: #38bdf8;"><code>%s</code></p>
		<p style="margin: 0; font-size: 13px; color: #94a3b8; line-height: 1.5;">If you did not authorize this key, you can revoke it immediately in your workspace settings.</p>
	`, safeName, safeKeyName, safeOrg, html.EscapeString(keyPrefix))

	htmlBody = RenderBrandLayoutWithHero(preview, "Team API Key Created", content, "Manage Workspace API Keys", d.ManageKeysURL, "", DrixyMascotIdeaLightbulb, "Drixy")
	return subject, htmlBody
}
