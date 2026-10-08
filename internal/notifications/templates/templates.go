package templates

import (
	"fmt"
	"time"
)

// Brand Color Palette (ScanDrix clean white and black minimalist system)
const (
	ColorPrimaryLight = "#000000" // Crisp black CTA button
	ColorPrimaryDark  = "#ffffff" // Pure white text on CTA button
	ColorHeaderBG     = "#ffffff" // Pure white header
	ColorPageBG       = "#f9fafb" // Crisp light page background
	ColorCardBG       = "#ffffff" // Pure white card container
	ColorTextPrimary  = "#111827" // Deep black headings and primary text
	ColorTextMuted    = "#6b7280" // Refined slate gray secondary text
	ColorBorder       = "#e5e7eb" // Subtle clean 1px border
	ColorSuccess      = "#10b981" // Emerald green status accent
	ColorDanger       = "#ef4444" // Red alert accent
	ColorWarning      = "#f59e0b" // Amber warning accent
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

// formatNumber formats an integer with comma separators (e.g. 10,000,000).
func formatNumber(n int64) string {
	in := fmt.Sprintf("%d", n)
	if len(in) <= 3 {
		return in
	}
	var out []byte
	rem := len(in) % 3
	if rem > 0 {
		out = append(out, in[:rem]...)
		if len(in) > rem {
			out = append(out, ',')
		}
	}
	for i := rem; i < len(in); i += 3 {
		out = append(out, in[i:i+3]...)
		if i+3 < len(in) {
			out = append(out, ',')
		}
	}
	return string(out)
}

