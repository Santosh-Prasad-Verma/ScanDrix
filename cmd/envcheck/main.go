package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type EnvVar struct {
	Name        string
	Required    bool
	Description string
	IsSecret    bool
}

var expectedVars = []EnvVar{
	// Core Server & Authentication
	{Name: "APP_ENV", Required: false, Description: "Runtime environment (development, staging, production)", IsSecret: false},
	{Name: "PORT", Required: false, Description: "HTTP listen port (default: 8080)", IsSecret: false},
	{Name: "JWT_SECRET", Required: true, Description: "Secret key used for HMAC-SHA256 JWT signing", IsSecret: true},

	// Database Connection (PostgreSQL 16 + pgvector)
	{Name: "DATABASE_URL", Required: true, Description: "PostgreSQL connection string or individual POSTGRES_* vars", IsSecret: true},

	// Message Broker (RabbitMQ)
	{Name: "RABBITMQ_URL", Required: true, Description: "AMQP connection URL for async reviews and webhooks", IsSecret: true},

	// Distributed Cache & Locks (Redis 7.2)
	{Name: "REDIS_URL", Required: true, Description: "Redis connection URL for idempotency locks and rate limits", IsSecret: true},

	// Security & Envelope Encryption
	{Name: "KMS_MASTER_KEY", Required: false, Description: "Master 256-bit hex key for AES-GCM envelope encryption", IsSecret: true},

	// Billing Webhook Verification
	{Name: "STRIPE_WEBHOOK_SECRET", Required: false, Description: "Stripe webhook signature secret (whsec_...)", IsSecret: true},
	{Name: "RAZORPAY_WEBHOOK_SECRET", Required: false, Description: "Razorpay webhook signature secret", IsSecret: true},

	// AI Engine Provider (At least one provider recommended)
	{Name: "OPENROUTER_API_KEY", Required: false, Description: "OpenRouter LLM inference API key", IsSecret: true},
	{Name: "OPENAI_API_KEY", Required: false, Description: "Direct OpenAI API key", IsSecret: true},
}

func main() {
	fmt.Println("=================================================================")
	fmt.Println("           ScanDrix Pre-flight Environment Validator             ")
	fmt.Println("=================================================================")

	// Load local .env if exists
	_ = godotenv.Load()
	_ = godotenv.Load(".env")

	var missingRequired []string
	var missingOptional []string
	validCount := 0

	for _, v := range expectedVars {
		val := strings.TrimSpace(os.Getenv(v.Name))
		if val == "" {
			// Special fallback check for DATABASE_URL vs individual POSTGRES_*
			if v.Name == "DATABASE_URL" {
				host := os.Getenv("POSTGRES_HOST")
				db := os.Getenv("POSTGRES_DB")
				if host != "" && db != "" {
					fmt.Printf("  [OK]   %-25s : Configured via POSTGRES_HOST / POSTGRES_DB\n", v.Name)
					validCount++
					continue
				}
			}

			if v.Required {
				fmt.Printf("  [FAIL] %-25s : MISSING (Required: %s)\n", v.Name, v.Description)
				missingRequired = append(missingRequired, v.Name)
			} else {
				fmt.Printf("  [WARN] %-25s : Not configured (Optional: %s)\n", v.Name, v.Description)
				missingOptional = append(missingOptional, v.Name)
			}
		} else {
			status := "configured"
			if v.IsSecret {
				status = fmt.Sprintf("set (%d chars)", len(val))
			} else {
				status = fmt.Sprintf("set [%s]", val)
			}
			fmt.Printf("  [OK]   %-25s : %s\n", v.Name, status)
			validCount++
		}
	}

	fmt.Println("-----------------------------------------------------------------")
	fmt.Printf("Summary: %d Configured | %d Missing Required | %d Missing Optional\n",
		validCount, len(missingRequired), len(missingOptional))
	fmt.Println("=================================================================")

	if len(missingRequired) > 0 {
		fmt.Fprintf(os.Stderr, "\n[FATAL] Missing %d required environment variable(s): %s\n",
			len(missingRequired), strings.Join(missingRequired, ", "))
		fmt.Fprintln(os.Stderr, "Please configure these in your .env or environment before launching ScanDrix.")
		os.Exit(1)
	}

	fmt.Println("\nAll required environment variables are set and validated.")
}
