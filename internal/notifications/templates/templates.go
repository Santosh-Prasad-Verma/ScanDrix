package templates

import (
	"fmt"
	"time"
)

// Brand Color Palette (matching Kodus AI / ScanDrix enterprise brand system)
const (
	ColorPrimaryLight = "#f8b76d" // Brand primary light accent / buttons
	ColorPrimaryDark  = "#443024" // Button text contrast on primary-light
	ColorHeaderBG     = "#101019" // Dark header banner background
	ColorPageBG       = "#f4f4f5" // Neutral light page background
	ColorCardBG       = "#ffffff" // White card container background
	ColorTextPrimary  = "#1f2937" // Dark slate primary text
	ColorTextMuted    = "#6b7280" // Muted secondary text
	ColorBorder       = "#e5e7eb" // Subtle divider and table borders
	ColorSuccess      = "#10b981" // Green status accent
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
