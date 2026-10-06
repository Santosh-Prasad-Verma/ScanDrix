package config_test

import (
	"os"
	"testing"

	"github.com/scandrix/backend/internal/config"
)

func TestConfigLoadDevelopment(t *testing.T) {
	os.Setenv("APP_ENV", "development")
	os.Setenv("PORT", "9090")
	os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/testdb")
	os.Setenv("DEEPSEEK_API_KEY", "test_ds_key")
	defer func() {
		os.Unsetenv("APP_ENV")
		os.Unsetenv("PORT")
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("DEEPSEEK_API_KEY")
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error loading dev config: %v", err)
	}

	if cfg.Environment != config.EnvDevelopment || cfg.APIPort != 9090 {
		t.Fatalf("unexpected config values: %+v", cfg)
	}

	if cfg.DeepSeekAPIKey != "test_ds_key" {
		t.Fatalf("expected DeepSeekAPIKey to be loaded from environment")
	}
}

func TestConfigLoadProductionValidation(t *testing.T) {
	os.Setenv("APP_ENV", "production")
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("JWT_SECRET")
	os.Unsetenv("APPWRITE_PROJECT_ID")
	os.Unsetenv("APPWRITE_API_KEY")
	defer os.Unsetenv("APP_ENV")

	_, err := config.Load()
	if err == nil {
		t.Fatalf("expected validation error in production when secrets are missing")
	}
}

// F-63: Appwrite backs review-artifact archival only, and every call site
// already degrades (nil-safe client, warning on upload failure). It must not be
// a startup requirement, because an optional archive feature crash-looping the
// whole production API is an availability bug.
//
// The other requirements in this block (JWT_SECRET, DATABASE_URL, SMTP_HOST) are
// genuinely load-bearing and are asserted separately.
func TestProductionBootsWithoutAppwrite(t *testing.T) {
	for k, v := range map[string]string{
		"APP_ENV":             "production",
		"DATABASE_URL":        "postgres://localhost:5432/db",
		"JWT_SECRET":          "a-sufficiently-long-test-secret-value",
		"SMTP_HOST":           "smtp.example.test",
		"ANTHROPIC_API_KEY":   "sk-test",
		"APPWRITE_PROJECT_ID": "",
		"APPWRITE_API_KEY":    "",
	} {
		t.Setenv(k, v)
	}
	t.Setenv("APPWRITE_ENDPOINT", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("production must boot without Appwrite credentials, got: %v", err)
	}
	if cfg.AppwriteProjectID != "" || cfg.AppwriteAPIKey != "" {
		t.Fatalf("expected empty Appwrite credentials, got project=%q key set=%v",
			cfg.AppwriteProjectID, cfg.AppwriteAPIKey != "")
	}
}

// Guard the other half: dropping a genuinely required secret must still fail,
// so demoting Appwrite did not weaken the block it lives in.
func TestProductionStillRequiresJWTAndDatabaseAndSMTP(t *testing.T) {
	base := map[string]string{
		"APP_ENV":           "production",
		"DATABASE_URL":      "postgres://localhost:5432/db",
		"JWT_SECRET":        "a-sufficiently-long-test-secret-value",
		"SMTP_HOST":         "smtp.example.test",
		"ANTHROPIC_API_KEY": "sk-test",
	}
	for _, missing := range []string{"DATABASE_URL", "JWT_SECRET", "SMTP_HOST"} {
		t.Run("missing "+missing, func(t *testing.T) {
			for k, v := range base {
				t.Setenv(k, v)
			}
			t.Setenv(missing, "")

			if _, err := config.Load(); err == nil {
				t.Fatalf("production must still fail without %s", missing)
			}
		})
	}
}

func TestResendAPIKeyFallbackForSMTP(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/db")
	t.Setenv("JWT_SECRET", "a-sufficiently-long-test-secret-value")
	t.Setenv("SMTP_HOST", "")
	t.Setenv("RESEND_API_KEY", "re_test_key_12345")
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected config to load with RESEND_API_KEY fallback, got error: %v", err)
	}
	if cfg.SMTPHost != "smtp.resend.com" {
		t.Fatalf("expected SMTPHost to be smtp.resend.com, got %s", cfg.SMTPHost)
	}
	if cfg.SMTPUsername != "resend" {
		t.Fatalf("expected SMTPUsername to be resend, got %s", cfg.SMTPUsername)
	}
	if cfg.SMTPPassword != "re_test_key_12345" {
		t.Fatalf("expected SMTPPassword to be re_test_key_12345, got %s", cfg.SMTPPassword)
	}
}
