package config

import (
	"errors"
	"os"
	"testing"
)

func TestLoadEnvironmentConfig(t *testing.T) {
	os.Setenv("PORT", "9090")
	os.Setenv("COMPONENT_TYPE", "scandrix-api")
	defer func() {
		os.Unsetenv("PORT")
		os.Unsetenv("COMPONENT_TYPE")
	}()

	cfg, err := LoadEnvironmentConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != 9090 {
		t.Fatalf("expected port 9090, got %d", cfg.Port)
	}
	if cfg.ComponentType != "scandrix-api" {
		t.Fatalf("expected component scandrix-api, got %s", cfg.ComponentType)
	}
}

func TestLoadJWTConfig(t *testing.T) {
	os.Setenv("JWT_SECRET", "super-secret-key-12345")
	os.Setenv("JWT_ISSUER", "scandrix-test")
	defer func() {
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("JWT_ISSUER")
	}()

	cfg, err := LoadJWTConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Secret != "super-secret-key-12345" {
		t.Fatalf("expected secret, got %s", cfg.Secret)
	}
	if cfg.Issuer != "scandrix-test" {
		t.Fatalf("expected issuer, got %s", cfg.Issuer)
	}
}

func TestLoadPostgresConfig(t *testing.T) {
	os.Setenv("DB_HOST", "postgres.example.com")
	os.Setenv("DB_PORT", "5433")
	os.Setenv("DB_USER", "scandrix_user")
	os.Setenv("DB_NAME", "scandrix_db")
	defer func() {
		os.Unsetenv("DB_HOST")
		os.Unsetenv("DB_PORT")
		os.Unsetenv("DB_USER")
		os.Unsetenv("DB_NAME")
	}()

	cfg, err := LoadPostgresConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Host != "postgres.example.com" {
		t.Fatalf("expected host, got %s", cfg.Host)
	}
	if cfg.Port != 5433 {
		t.Fatalf("expected port 5433, got %d", cfg.Port)
	}
	dsn := cfg.DSN()
	if dsn == "" {
		t.Fatalf("expected valid DSN")
	}
}

func TestLoadRabbitMQAndQueueConfig(t *testing.T) {
	rmq, err := LoadRabbitMQConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rmq.ExchangeName == "" {
		t.Fatalf("expected exchange name")
	}

	wq, err := LoadWorkflowQueueConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wq.QueueName == "" {
		t.Fatalf("expected workflow queue name")
	}
}

func TestLoadServerConfig(t *testing.T) {
	srv, err := LoadServerConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if srv.Port <= 0 {
		t.Fatalf("expected valid server port")
	}
}

func TestSentryAndPyroscope(t *testing.T) {
	_ = SetupSentry("test-comp")
	ReportExceptionToSentry(errors.New("test sentry error"), "TestContext", map[string]string{"env": "test"}, map[string]interface{}{"code": 1})

	_ = InitPyroscope(PyroscopeConfig{
		AppName: "test-app",
	})
	StopPyroscope()
}
