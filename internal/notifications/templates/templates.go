package templates

import (
	"fmt"
	"time"
)

// Brand Color Palette (ScanDrix obsidian dark & neon lime system)
const (
	ColorPrimaryLight = "#c9f36b" // Brand neon lime accent / buttons
	ColorPrimaryDark  = "#080807" // Pitch black button text on neon lime
	ColorHeaderBG     = "#0a0a0d" // Deep obsidian header background
	ColorPageBG       = "#050505" // Pitch dark page background
	ColorCardBG       = "#0e0f12" // Sleek dark card container background
	ColorTextPrimary  = "#ffffff" // Crisp pure white headings & text
	ColorTextMuted    = "#9ca3af" // Clean muted gray text
	ColorBorder       = "#222428" // Sleek 1px dark border
	ColorSuccess      = "#c9f36b" // Neon lime status accent
	ColorDanger       = "#f87171" // Red alert accent
	ColorWarning      = "#fbbf24" // Amber warning accent
)

// InvoiceDetails encapsulates full tax and itemized subscription billing data.
type InvoiceDetails struct {
	InvoiceNumber    string    `json:"invoice_number"`
	RecipientName    string    `json:"recipient_name"`
	RecipientEmail   string    `json:"recipient_email"`
	OrganizationName string    `json:"organization_name"`
	PlanTier         string    `json:"plan_tier"`
	AmountFormatted  string    `json:"amount_formatted"` // e.g. "₹2,499.00 INR" or "$29.00 USD"
	AmountSubtotal   string    `json:"amount_subtotal"`
	TaxAmount        string    `json:"tax_amount"`
	OrderID          string    `json:"order_id"`
	PaymentID        string    `json:"payment_id"`
	PaymentProvider  string    `json:"payment_provider"` // "Razorpay"
	PaymentMethod    string    `json:"payment_method"`   // "UPI / NetBanking / Cards"
	BillingDate      time.Time `json:"billing_date"`
	NextBillingDate  time.Time `json:"next_billing_date"`
	BillingPortalURL string    `json:"billing_portal_url"`
}

// GenerateInvoiceNumber creates a standard sequential invoice identifier.
func GenerateInvoiceNumber(orderID string) string {
	suffix := orderID
	if len(orderID) > 8 {
		suffix = orderID[len(orderID)-8:]
	}
	return fmt.Sprintf("INV-%d-%s", time.Now().Year(), suffix)
}
