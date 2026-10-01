// Command envcheck is ScanDrix's pre-flight environment validator.
//
// AUDIT_REMEDIATION.md F-76: this previously checked a hand-written list of 13
// names while the code read several hundred, so a renamed or missing variable
// surfaced as a runtime failure rather than a startup failure. The set of
// variables is now generated from the source into env_contract_gen.go and
// drift-checked by a test, so it cannot silently fall behind again.
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/joho/godotenv"
)

// bootCriticalEnv is the set of variables the process genuinely cannot start
// without: identity/signing material, the datastore, cache, and broker. Every
// other variable the code reads is reported but never blocks startup, because
// the rest are optional provider integrations and feature flags.
var bootCriticalEnv = map[string]bool{
	"DATABASE_URL":   true,
	"JWT_SECRET":     true,
	"KMS_MASTER_KEY": true,
	"REDIS_URL":      true,
	"RABBITMQ_URL":   true,
}

func main() {
	fmt.Println("=================================================================")
	fmt.Println("           ScanDrix Pre-flight Environment Validator             ")
	fmt.Println("=================================================================")

	// Local .env is a developer convenience; real deployments inject the
	// environment directly.
	_ = godotenv.Load(".env")

	var missingRequired []string
	configured := 0
	missingOptional := 0

	for _, c := range envContractNames {
		if strings.TrimSpace(os.Getenv(c.Name)) != "" {
			configured++
			continue
		}

		// DATABASE_URL may be assembled from the discrete POSTGRES_* values.
		if c.Name == "DATABASE_URL" && os.Getenv("POSTGRES_HOST") != "" && os.Getenv("POSTGRES_DB") != "" {
			configured++
			continue
		}

		if c.Required {
			missingRequired = append(missingRequired, c.Name)
		} else {
			missingOptional++
		}
	}

	// Values are never printed. A variable is reported as set or unset, never
	// echoed, so running envcheck can never leak a credential into a log or a
	// terminal scrollback.
	total := len(envContractNames)
	fmt.Printf("Checked %d environment variables read by the application code.\n", total)
	fmt.Println("-----------------------------------------------------------------")
	fmt.Printf("Configured: %d | Missing required: %d | Not configured (optional): %d\n",
		configured, len(missingRequired), missingOptional)
	fmt.Println("=================================================================")

	if len(missingRequired) > 0 {
		sort.Strings(missingRequired)
		fmt.Fprintf(os.Stderr, "\n[FATAL] Missing %d required environment variable(s):\n  %s\n",
			len(missingRequired), strings.Join(missingRequired, "\n  "))
		fmt.Fprintln(os.Stderr, "Configure these in your environment before launching ScanDrix.")
		os.Exit(1)
	}

	fmt.Println("\nAll required environment variables are set.")
}
