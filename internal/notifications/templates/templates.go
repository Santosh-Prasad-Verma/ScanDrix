package templates

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Brand Color Palette (ScanDrix Orange-Blue Banner & Noise Theme)
const (
	ColorPrimaryCTA     = "#ef8557" // Warm coral brand CTA
	ColorPrimaryAccent  = "#f97316" // Vibrant orange accent
	ColorCoralAccent    = "#ef8557" // ScanDrix coral button accent
	ColorRoyalBlue      = "#2563eb" // Brand royal blue accent
	ColorPageBG         = "#060812" // Deep ambient dark page background with noise
	ColorCardBG         = "#0a0e1c" // Deep midnight card container
	ColorCardSurface    = "#0e1428" // Internal container surface
	ColorHeaderBG       = "#0b1a42" // Header background
	ColorPrimaryLight   = "#ef8557" // Button fill
	ColorPrimaryDark    = "#ffffff" // Button text
	ColorTextPrimary    = "#ffffff" // Crisp white headings and titles
	ColorTextCream      = "#ebd6ff" // Cream accent
	ColorTextSecondary  = "#cbd5e1" // High contrast readable body text
	ColorTextMuted      = "#94a3b8" // Slate muted metadata text
	ColorBorder         = "#1e293b" // Subtle dark border
	ColorBorderSubtle   = "#2e3856" // Subtle highlight border
	ColorSuccess        = "#10b981" // Emerald green status accent
	ColorDanger         = "#ef4444" // Red alert accent
	ColorWarning        = "#f59e0b" // Amber warning accent
)

// Brand Banner & Asset paths
const (
	BannerGeneral = "assets/banner.png"
	BannerWelcome = "assets/welcome_banner.png"
	FooterLogo    = "assets/footer_logo.png"
	NoiseTexture  = "assets/noise.png"
)

// Drixy Mascot Asset filenames for contextual email templates
const (
	DrixyAssetBaseURL           = "drixy"
	DrixyMascotWavingHello      = "drixy_waving_hello.png"
	DrixyMascotHappyCelebrating = "drixy_happy_celebrating.png"
	DrixyMascotStandingReady    = "drixy_standing_ready.png"
	DrixyMascotCheeringSuccess  = "drixy_cheering_success.png"
	DrixyMascotLyingRelaxed     = "drixy_lying_relaxed.png"
	DrixyMascotPointingGuide    = "drixy_pointing_guide.png"
	DrixyMascotThinking         = "drixy_thinking_analyzing.png"
	DrixyMascotCuriousLooking   = "drixy_curious_looking.png"
	DrixyMascotReadingBook      = "drixy_reading_book.png"
	DrixyMascotCoolThumbsUp     = "drixy_cool_thumbs_up.png"
	DrixyMascotIdeaLightbulb    = "drixy_idea_lightbulb.png"
	DrixyMascotScanningPages    = "drixy_scanning_pages.png"
	DrixyMascotShockedAlert     = "drixy_shocked_alert.png"
	DrixyMascotCoffeeBreak      = "drixy_coffee_break.png"
	DrixyMascotHuggingKnees     = "drixy_hugging_knees_waiting.png"
	DrixyMascotFloatingWaving   = "drixy_floating_waving.png"
	DrixyMascotInBoxUnboxing    = "drixy_in_box_unboxing.png"
	DrixyMascotWalkingBackpack  = "drixy_walking_backpack.png"
)

// GetAssetURL returns the absolute URL for a template asset.
func GetAssetURL(assetPath string) string {
	base := os.Getenv("EMAIL_ASSET_BASE_URL")
	if base == "" {
		base = os.Getenv("ASSET_BASE_URL")
	}
	if base == "" {
		base = "https://www.scandrix.dev"
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(assetPath, "/")
}

// GetDrixyURL returns the asset URL for a Drixy mascot image.
func GetDrixyURL(imageName string) string {
	if imageName == "" {
		return ""
	}
	base := os.Getenv("EMAIL_ASSET_BASE_URL")
	if base == "" {
		base = os.Getenv("ASSET_BASE_URL")
	}
	if base == "" {
		base = "https://www.scandrix.dev"
	}
	return strings.TrimRight(base, "/") + "/drixy/" + strings.TrimLeft(imageName, "/")
}

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

