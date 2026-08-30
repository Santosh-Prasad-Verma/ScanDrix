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
