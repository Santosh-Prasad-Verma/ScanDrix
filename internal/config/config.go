package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// AppEnvironment represents the runtime target (development, staging, production).
type AppEnvironment string

const (
	EnvDevelopment AppEnvironment = "development"
	EnvStaging     AppEnvironment = "staging"
	EnvProduction  AppEnvironment = "production"
)

// Config encapsulates all validated runtime configuration settings.
type Config struct {
	Environment AppEnvironment

	// Server Ports
	APIPort      int
	WebhooksPort int

	// Database
	DatabaseURL string

	// Message Queue
	RabbitMQURL string

	// Cache & Redis
	RedisURL string

	// Appwrite Platform & Storage
	AppwriteEndpoint  string
	AppwriteProjectID string
	AppwriteAPIKey    string

	// SCM Webhook Secrets
	GitHubWebhookSecret string
	GitLabWebhookSecret string

	// AI Engine Credentials
	AnthropicAPIKey string
	OpenAIAPIKey    string
	GeminiAPIKey    string
	LocalLLMEndpoint string
}

// Load reads and validates configuration from environment variables and local .env files.
// In accordance with Master Rule 1.6, it fails fast if critical variables are missing.
func Load() (*Config, error) {
	// Attempt to load .env if present (non-fatal if missing in production)
	_ = godotenv.Load(".env")
	_ = godotenv.Load("../.env")

	envStr := strings.ToLower(getEnvOrDefault("APP_ENV", "development"))
	var appEnv AppEnvironment
	switch envStr {
	case "production", "prod":
		appEnv = EnvProduction
	case "staging", "stage":
		appEnv = EnvStaging
	default:
		appEnv = EnvDevelopment
	}

	apiPort, err := strconv.Atoi(getEnvOrDefault("PORT", "8080"))
	if err != nil {
		return nil, fmt.Errorf("invalid PORT value: %w", err)
	}

	webhooksPort, err := strconv.Atoi(getEnvOrDefault("WEBHOOKS_PORT", "8081"))
	if err != nil {
		return nil, fmt.Errorf("invalid WEBHOOKS_PORT value: %w", err)
	}

	cfg := &Config{
		Environment:         appEnv,
		APIPort:             apiPort,
		WebhooksPort:        webhooksPort,
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		RabbitMQURL:         getEnvOrDefault("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		RedisURL:            getEnvOrDefault("REDIS_URL", "redis://localhost:6379"),
		AppwriteEndpoint:    getEnvOrDefault("APPWRITE_ENDPOINT", "https://sgp.cloud.appwrite.io/v1"),
		AppwriteProjectID:   os.Getenv("APPWRITE_PROJECT_ID"),
		AppwriteAPIKey:      os.Getenv("APPWRITE_API_KEY"),
		GitHubWebhookSecret: os.Getenv("GITHUB_WEBHOOK_SECRET"),
		GitLabWebhookSecret: os.Getenv("GITLAB_WEBHOOK_SECRET"),
		AnthropicAPIKey:     os.Getenv("ANTHROPIC_API_KEY"),
		OpenAIAPIKey:        os.Getenv("OPENAI_API_KEY"),
		GeminiAPIKey:        os.Getenv("GEMINI_API_KEY"),
		LocalLLMEndpoint:    os.Getenv("VLLM_ENDPOINT"),
	}

	// Validate required variables
	var validationErrors []string

	if cfg.DatabaseURL == "" {
		validationErrors = append(validationErrors, "DATABASE_URL is required (Master Rule 1.6)")
	}

	if appEnv == EnvProduction {
		if cfg.AppwriteProjectID == "" {
			validationErrors = append(validationErrors, "APPWRITE_PROJECT_ID is required in production")
		}
		if cfg.AppwriteAPIKey == "" {
			validationErrors = append(validationErrors, "APPWRITE_API_KEY is required in production")
		}
		if cfg.AnthropicAPIKey == "" && cfg.OpenAIAPIKey == "" && cfg.GeminiAPIKey == "" && cfg.LocalLLMEndpoint == "" {
			validationErrors = append(validationErrors, "At least one AI provider key or local LLM endpoint must be specified")
		}
	}

	if len(validationErrors) > 0 {
		return nil, errors.New("configuration validation failed:\n - " + strings.Join(validationErrors, "\n - "))
	}

	return cfg, nil
}

func getEnvOrDefault(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
