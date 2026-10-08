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
		id      string
		name    string
		subject string
		html    string
	}

	emails := []emailItem{}

	s1, h1 := templates.RenderEmailVerification("Alex Rivera", "https://scandrix.dev/confirm-email?token=sec_preview_demo_token_12345")
	emails = append(emails, emailItem{"email_verification", "Email Verification", s1, h1})

	s2, h2 := templates.RenderNewUserWelcome("Alex Rivera", "https://scandrix.dev/cockpit")
	emails = append(emails, emailItem{"user_welcome", "New User Welcome", s2, h2})

	s3, h3 := templates.RenderSubscriptionWelcome("Alex Rivera", "Acme Engineering", "Pro Team", 50000000, []string{"claude-sonnet-5", "gpt-5.6-turbo", "gemini-3.7-flash"}, "https://scandrix.dev/cockpit")
	emails = append(emails, emailItem{"subscription_welcome", "Subscription Welcome", s3, h3})

	s4, h4 := templates.RenderTeamInvite("Sarah Chen", "dev@acmecorp.dev", "Acme Engineering", "Senior Security Engineer", "https://scandrix.dev/invite/accept?token=invite_demo_token")
	emails = append(emails, emailItem{"team_invite", "Team Invitation", s4, h4})

	s5, h5 := templates.RenderPasswordReset("Alex Rivera", "https://scandrix.dev/reset-password?token=pwd_reset_demo_token")
	emails = append(emails, emailItem{"password_reset", "Password Reset", s5, h5})

	s6, h6 := templates.RenderInvoiceReceipt(sampleInvoice)
	emails = append(emails, emailItem{"invoice_receipt", "Invoice Receipt", s6, h6})

	s7, h7 := templates.RenderPaymentFailed("Alex Rivera", "Acme Engineering", "Pro Team", "ord_99881122", "Card declined by issuing bank (insufficient funds)", "https://scandrix.dev/cockpit/billing?retry=1")
	emails = append(emails, emailItem{"payment_failed", "Payment Failed Alert", s7, h7})

	s8, h8 := templates.RenderSpendLimitAlert("Alex Rivera", "Acme Engineering", 85, 42500000, 50000000, "https://scandrix.dev/cockpit/billing/upgrade")
	emails = append(emails, emailItem{"spend_limit_alert", "Spend Limit Alert (85%)", s8, h8})

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
    #sidebar { width: 320px; background: #ffffff; border-right: 1px solid #e5e7eb; display: flex; flex-direction: column; }
    .header { padding: 20px 24px; border-bottom: 1px solid #e5e7eb; display: flex; align-items: center; justify-content: space-between; }
    .logo-text { font-size: 18px; font-weight: 800; color: #000000; letter-spacing: -0.5px; }
    .badge { font-size: 11px; padding: 2px 8px; border-radius: 9999px; background: #f3f4f6; color: #374151; font-weight: 600; border: 1px solid #e5e7eb; }
    .nav-list { list-style: none; overflow-y: auto; flex: 1; padding: 12px; }
    .nav-item { padding: 12px 16px; border-radius: 8px; margin-bottom: 4px; cursor: pointer; transition: all 0.15s ease; border: 1px solid transparent; }
    .nav-item:hover { background: #f9fafb; border-color: #e5e7eb; }
    .nav-item.active { background: #000000; border-color: #000000; }
    .nav-item.active .nav-title { color: #ffffff; }
    .nav-item.active .nav-subject { color: #9ca3af; }
    .nav-title { font-size: 14px; font-weight: 600; color: #111827; margin-bottom: 3px; }
    .nav-subject { font-size: 12px; color: #6b7280; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
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
      <div class="badge">Clean White &amp; Black</div>
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
			indexHTML += fmt.Sprintf("      { id: %q, name: %q, subject: %q, file: %q },\n", e.id, e.name, e.subject, fmt.Sprintf("%s.html", e.id))
		}
		indexHTML += `    ];

    let current = emailData[0];
    const navList = document.getElementById("nav-list");
    const frame = document.getElementById("preview-frame");
    const subjectEl = document.getElementById("active-subject");

    emailData.forEach((item, idx) => {
      const li = document.createElement("li");
      li.className = "nav-item" + (idx === 0 ? " active" : "");
      li.innerHTML = '<div class="nav-title">' + item.name + '</div><div class="nav-subject">' + item.subject + '</div>';
      li.onclick = () => selectEmail(item, li);
      navList.appendChild(li);
    });

    function selectEmail(item, el) {
      current = item;
      document.querySelectorAll(".nav-item").forEach(i => i.classList.remove("active"));
      el.classList.add("active");
      subjectEl.textContent = item.subject;
      frame.src = item.file;
    }

    function setMode(mode) {
      document.querySelectorAll(".viewport-btn").forEach(b => b.classList.remove("active"));
      if (mode === "desktop") {
        frame.style.maxWidth = "680px";
        event.target.classList.add("active");
      } else if (mode === "mobile") {
        frame.style.maxWidth = "390px";
        event.target.classList.add("active");
      }
    }

    function openRaw() {
      if (current) window.open(current.file, "_blank");
    }

    selectEmail(emailData[0], navList.firstChild);
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
