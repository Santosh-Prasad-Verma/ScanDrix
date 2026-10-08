package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/scandrix/backend/internal/notifications/templates"
)

func main() {
	outDirs := []string{
		"/home/tarun/Videos/kodus-ai/ScanDrix/email_previews",
		"/home/tarun/Videos/kodus-ai/ScanDrix/scandrix-website/public/email-previews",
	}

	for _, dir := range outDirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			panic(err)
		}
	}

	sampleInvoice := templates.InvoiceDetails{
		InvoiceNumber:    "INV-2026-0042",
		RecipientName:    "Alex Rivera",
		RecipientEmail:   "alex@acmecorp.dev",
		OrganizationName: "Acme Engineering",
		PlanTier:         "Enterprise Pro",
		AmountFormatted:  "$149.00 USD",
		OrderID:          "ord_99881122",
		PaymentID:        "pay_77334411",
		PaymentProvider:  "Stripe",
		BillingDate:      time.Now(),
		NextBillingDate:  time.Now().AddDate(0, 1, 0),
		BillingPortalURL: "https://scandrix.dev/cockpit/billing",
	}

	type emailItem struct {
		id       string
		category string
		name     string
		subject  string
		html     string
	}

	emails := []emailItem{}

	// --- 1. Security & Authentication ---
	sReset, hReset := templates.RenderPasswordResetWithDetails(templates.PasswordResetDetails{
		RecipientName:  "Alex Rivera",
		ResetURL:       "https://scandrix.dev/reset-password?token=pwd_reset_live_tok_9918",
		Device:         "macOS 15.3 (Sequoia) · Chrome 124.0",
		IPAddress:      "198.51.100.42",
		Location:       "San Francisco, CA, United States",
		RequestedAt:    time.Now(),
		LockAccountURL: "https://scandrix.dev/account/lock?token=sec_lock_session_881",
	})
	emails = append(emails, emailItem{"password_reset", "Security & Auth", "Password Reset (15m Expiry)", sReset, hReset})

	sLogin, hLogin := templates.RenderNewDeviceLogin(templates.NewDeviceLoginDetails{
		RecipientName:  "Alex Rivera",
		Device:         "Linux x86_64 · Firefox 131.0",
		IPAddress:      "194.26.29.112",
		Location:       "Frankfurt am Main, Germany",
		LoginTime:      time.Now(),
		LockAccountURL: "https://scandrix.dev/account/lock?token=sec_lock_device_7721",
		ActivityURL:    "https://scandrix.dev/cockpit/security",
	})
	emails = append(emails, emailItem{"new_device_login", "Security & Auth", "New Device / Location Login", sLogin, hLogin})

	sAPIKey, hAPIKey := templates.RenderTeamAPIKeyCreated(templates.TeamAPIKeyCreatedDetails{
		RecipientName: "Alex Rivera",
		OrgName:       "Acme Engineering",
		KeyName:       "github-actions-ci-pipeline",
		KeyPrefix:     "scandrix_live_9f82...3e71",
		CreatedBy:     "Alex Rivera (alex@acme.dev)",
		Scopes:        []string{"ci:scan", "pr:review", "rules:read"},
		CreatedAt:     time.Now(),
		ManageKeysURL: "https://scandrix.dev/cockpit/settings/api-keys",
	})
	emails = append(emails, emailItem{"api_key_created", "Security & Auth", "Team API Key Created", sAPIKey, hAPIKey})

	sVerify, hVerify := templates.RenderEmailVerification("Alex Rivera", "https://scandrix.dev/confirm-email?token=sec_preview_demo_token_12345")
	emails = append(emails, emailItem{"email_verification", "Security & Auth", "Email Verification", sVerify, hVerify})

	// --- 2. Developer Workflow & Code Review ---
	sVuln, hVuln := templates.RenderCriticalVulnerabilityAlert(templates.CriticalVulnerabilityAlertDetails{
		RecipientName: "Alex Rivera",
		OrgName:       "Acme Engineering",
		Repository:    "acme/auth-service",
		BranchOrPR:    "PR #142 (fix/auth-tokens)",
		Severity:      "CRITICAL",
		FindingTitle:  "CWE-89: SQL Injection Vulnerability in User Query",
		RuleID:        "scandrix-sec-sql-004",
		FilePath:      "internal/db/users.go",
		LineNumber:    74,
		Description:   "Unsanitized user-supplied identifier is directly concatenated into a raw database query string, allowing arbitrary SQL execution.",
		Remediation:   "Replace raw string formatting with parameterized query placeholders ($1, $2) or use repository query builder.",
		FindingURL:    "https://scandrix.dev/cockpit/findings/fnd_sec_99182",
		SourceCodeURL: "https://github.com/acme/auth-service/blob/main/internal/db/users.go#L74",
	})
	emails = append(emails, emailItem{"critical_vulnerability_alert", "Developer Workflow", "Critical Vulnerability Alert", sVuln, hVuln})

	sWeekly, hWeekly := templates.RenderWeeklyScanDigest(templates.WeeklyScanDigestDetails{
		RecipientName:          "Alex Rivera",
		OrgName:                "Acme Engineering",
		WeekDateRange:          "Sep 30 - Oct 07, 2026",
		TotalPRsReviewed:       48,
		VulnerabilitiesBlocked: 14,
		CriticalBlocked:        3,
		HoursSaved:             18.5,
		HealthScore:            98,
		TopRepos: []templates.DigestRepoItem{
			{Repository: "acme/auth-service", PRsScanned: 24, IssuesBlocked: 7, HealthGrade: "A+"},
			{Repository: "acme/payment-api", PRsScanned: 16, IssuesBlocked: 5, HealthGrade: "A"},
			{Repository: "acme/web-frontend", PRsScanned: 8, IssuesBlocked: 2, HealthGrade: "A"},
		},
		DashboardURL: "https://scandrix.dev/cockpit",
	})
	emails = append(emails, emailItem{"weekly_digest", "Developer Workflow", "Weekly Scan Summary Digest", sWeekly, hWeekly})

	sPR, hPR := templates.RenderPRReviewCompleted(templates.PRReviewCompletedDetails{
		RecipientName:    "Sarah Chen",
		OrgName:          "Acme Engineering",
		RepoName:         "acme/payment-api",
		PRNumber:         189,
		PRTitle:          "feat: implement OAuth2 PKCE token exchange",
		Author:           "Sarah Chen",
		Verdict:          "Changes Requested",
		CriticalCount:    1,
		HighCount:        0,
		SuggestionsCount: 2,
		DrixyNotes:       "Drixy identified a potential authorization bypass due to unverified redirect_uri matching. 2 optimization opportunities identified in token cache.",
		ReviewURL:        "https://github.com/acme/payment-api/pull/189#pullrequestreview-88192",
	})
	emails = append(emails, emailItem{"pr_review_completed", "Developer Workflow", "PR Review Completed (Drixy)", sPR, hPR})

	// --- 3. Account & Lifecycle ---
	sTrial, hTrial := templates.RenderTrialExpiring(templates.TrialExpiringDetails{
		RecipientName: "Alex Rivera",
		OrgName:       "Acme Engineering",
		DaysRemaining: 3,
		TrialEndDate:  time.Now().AddDate(0, 0, 3),
		PRsScanned:    64,
		IssuesBlocked: 9,
		HoursSaved:    14.2,
		UpgradeURL:    "https://scandrix.dev/cockpit/billing/upgrade",
	})
	emails = append(emails, emailItem{"trial_expiring", "Account & Lifecycle", "Trial Expiring in 3 Days", sTrial, hTrial})

	s80, h80 := templates.RenderUsageThresholdWarning(templates.UsageThresholdDetails{
		RecipientName: "Alex Rivera",
		OrgName:       "Acme Engineering",
		PlanTier:      "Pro Team",
		PercentUsed:   80,
		UsedUnits:     40000000,
		LimitUnits:    50000000,
		UnitType:      "Tokens",
		ResetDate:     time.Now().AddDate(0, 0, 7),
		UpgradeURL:    "https://scandrix.dev/cockpit/billing",
	})
	emails = append(emails, emailItem{"usage_threshold_80", "Account & Lifecycle", "Usage Threshold Warning (80%)", s80, h80})

	s100, h100 := templates.RenderUsageThresholdWarning(templates.UsageThresholdDetails{
		RecipientName: "Alex Rivera",
		OrgName:       "Acme Engineering",
		PlanTier:      "Pro Team",
		PercentUsed:   100,
		UsedUnits:     50000000,
		LimitUnits:    50000000,
		UnitType:      "Tokens",
		ResetDate:     time.Now().AddDate(0, 0, 7),
		UpgradeURL:    "https://scandrix.dev/cockpit/billing",
	})
	emails = append(emails, emailItem{"usage_threshold_100", "Account & Lifecycle", "Usage Quota Exceeded (100%)", s100, h100})

	sWelcome, hWelcome := templates.RenderNewUserWelcome("Alex Rivera", "https://scandrix.dev/cockpit")
	emails = append(emails, emailItem{"user_welcome", "Account & Lifecycle", "New User Welcome", sWelcome, hWelcome})

	sSub, hSub := templates.RenderSubscriptionWelcome("Alex Rivera", "Acme Engineering", "Pro Team", 50000000, []string{"claude-sonnet-5", "gpt-5.6-turbo", "gemini-3.7-flash"}, "https://scandrix.dev/cockpit")
	emails = append(emails, emailItem{"subscription_welcome", "Account & Lifecycle", "Subscription Welcome", sSub, hSub})

	sInvite, hInvite := templates.RenderTeamInvite("Sarah Chen", "dev@acmecorp.dev", "Acme Engineering", "Senior Security Engineer", "https://scandrix.dev/invite/accept?token=invite_demo_token")
	emails = append(emails, emailItem{"team_invite", "Account & Lifecycle", "Team Invitation", sInvite, hInvite})

	// --- 4. Billing & Receipts ---
	sInv, hInv := templates.RenderInvoiceReceipt(sampleInvoice)
	emails = append(emails, emailItem{"invoice_receipt", "Billing & Payments", "Invoice Receipt", sInv, hInv})

	sFail, hFail := templates.RenderPaymentFailed("Alex Rivera", "Acme Engineering", "Pro Team", "ord_99881122", "Card declined by issuing bank (insufficient funds)", "https://scandrix.dev/cockpit/billing?retry=1")
	emails = append(emails, emailItem{"payment_failed", "Billing & Payments", "Payment Failed Alert", sFail, hFail})

	for _, dir := range outDirs {
		for _, e := range emails {
			filePath := filepath.Join(dir, fmt.Sprintf("%s.html", e.id))
			if err := os.WriteFile(filePath, []byte(e.html), 0644); err != nil {
				panic(err)
			}
			fmt.Printf("Wrote: %s\n", filePath)
		}

		// Also generate an interactive index.html viewer
		indexHTML := `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>ScanDrix Email Templates Previewer</title>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #f3f4f6; color: #111827; display: flex; height: 100vh; overflow: hidden; }
    #sidebar { width: 340px; background: #ffffff; border-right: 1px solid #e5e7eb; display: flex; flex-direction: column; }
    .header { padding: 20px 24px; border-bottom: 1px solid #e5e7eb; display: flex; align-items: center; justify-content: space-between; }
    .logo-text { font-size: 18px; font-weight: 800; color: #000000; letter-spacing: -0.5px; }
    .badge { font-size: 11px; padding: 2px 8px; border-radius: 9999px; background: #f3f4f6; color: #374151; font-weight: 600; border: 1px solid #e5e7eb; }
    .nav-list { list-style: none; overflow-y: auto; flex: 1; padding: 12px; }
    .category-header { font-size: 11px; font-weight: 700; text-transform: uppercase; letter-spacing: 0.05em; color: #9ca3af; padding: 12px 16px 4px 16px; }
    .nav-item { padding: 10px 16px; border-radius: 8px; margin-bottom: 4px; cursor: pointer; transition: all 0.15s ease; border: 1px solid transparent; }
    .nav-item:hover { background: #f9fafb; border-color: #e5e7eb; }
    .nav-item.active { background: #000000; border-color: #000000; }
    .nav-item.active .nav-title { color: #ffffff; }
    .nav-item.active .nav-subject { color: #9ca3af; }
    .nav-title { font-size: 13px; font-weight: 600; color: #111827; margin-bottom: 2px; }
    .nav-subject { font-size: 11px; color: #6b7280; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
    #main { flex: 1; display: flex; flex-direction: column; background: #f9fafb; }
    #topbar { padding: 12px 24px; background: #ffffff; border-bottom: 1px solid #e5e7eb; display: flex; align-items: center; justify-content: space-between; }
    .topbar-info { display: flex; flex-direction: column; }
    .active-subject { font-size: 14px; font-weight: 600; color: #111827; }
    .viewport-controls { display: flex; gap: 8px; }
    .viewport-btn { background: #ffffff; border: 1px solid #e5e7eb; color: #4b5563; padding: 6px 14px; border-radius: 6px; font-size: 12px; cursor: pointer; transition: all 0.2s; font-weight: 500; }
    .viewport-btn.active, .viewport-btn:hover { color: #ffffff; background: #000000; border-color: #000000; }
    #preview-container { flex: 1; display: flex; justify-content: center; align-items: center; padding: 24px; overflow: auto; background: #f3f4f6; }
    #preview-frame { border: 1px solid #e5e7eb; border-radius: 8px; box-shadow: 0 4px 20px -2px rgba(0, 0, 0, 0.08); background: #ffffff; transition: width 0.2s ease; height: 100%; width: 100%; max-width: 600px; }
  </style>
</head>
<body>
  <div id="sidebar">
    <div class="header">
      <div class="logo-text">ScanDrix</div>
      <div class="badge">Drixy Templates</div>
    </div>
    <ul class="nav-list" id="nav-list"></ul>
  </div>
  <div id="main">
    <div id="topbar">
      <div class="topbar-info">
        <span style="font-size: 11px; text-transform: uppercase; letter-spacing: 0.05em; color: #6b7280;">Subject Line</span>
        <span class="active-subject" id="active-subject">...</span>
      </div>
      <div class="viewport-controls">
        <button class="viewport-btn active" onclick="setMode('desktop')">Desktop (600px)</button>
        <button class="viewport-btn" onclick="setMode('mobile')">Mobile (390px)</button>
        <button class="viewport-btn" onclick="openRaw()">Open HTML ↗</button>
      </div>
    </div>
    <div id="preview-container">
      <iframe id="preview-frame" src=""></iframe>
    </div>
  </div>

  <script>
    const emailData = [
`
		for _, e := range emails {
			indexHTML += fmt.Sprintf("      { id: %q, category: %q, name: %q, subject: %q, file: %q },\n", e.id, e.category, e.name, e.subject, fmt.Sprintf("%s.html", e.id))
		}
		indexHTML += `    ];

    let current = emailData[0];
    const navList = document.getElementById("nav-list");
    const frame = document.getElementById("preview-frame");
    const subjectEl = document.getElementById("active-subject");

    let currentCat = "";
    emailData.forEach((item, idx) => {
      if (item.category !== currentCat) {
        currentCat = item.category;
        const catHeader = document.createElement("li");
        catHeader.className = "category-header";
        catHeader.textContent = currentCat;
        navList.appendChild(catHeader);
      }
      const li = document.createElement("li");
      li.className = "nav-item" + (idx === 0 ? " active" : "");
      li.innerHTML = '<div class="nav-title">' + item.name + '</div><div class="nav-subject">' + item.subject + '</div>';
      li.onclick = () => selectEmail(item, li);
      navList.appendChild(li);
    });

    function selectEmail(item, el) {
      current = item;
      document.querySelectorAll(".nav-item").forEach(i => i.classList.remove("active"));
      if (el) el.classList.add("active");
      subjectEl.textContent = item.subject;
      frame.src = item.file;
    }

    function setMode(mode) {
      document.querySelectorAll(".viewport-btn").forEach(b => b.classList.remove("active"));
      if (mode === "desktop") {
        frame.style.maxWidth = "600px";
        event.target.classList.add("active");
      } else if (mode === "mobile") {
        frame.style.maxWidth = "390px";
        event.target.classList.add("active");
      }
    }

    function openRaw() {
      if (current) window.open(current.file, "_blank");
    }

    // Initialize with first item
    const firstItem = document.querySelector(".nav-item");
    if (firstItem) selectEmail(emailData[0], firstItem);
  </script>
</body>
</html>`

		indexPath := filepath.Join(dir, "index.html")
		if err := os.WriteFile(indexPath, []byte(indexHTML), 0644); err != nil {
			panic(err)
		}
		fmt.Printf("Wrote Index: %s\n", indexPath)
	}
}
