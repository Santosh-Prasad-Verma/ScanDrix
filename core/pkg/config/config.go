package config

import (
	"os"

	"github.com/joho/godotenv"
)

// AppConfig represents loaded runtime configuration.
type AppConfig struct {
	Environment        string
	Port               string
	DatabaseURL        string
	RedisURL           string
	AWSRegion          string
	AWSEndpointURL     string
	AWSAccessKeyID     string
	AWSSecretAccessKey string
	ArtifactsBucket    string
	TemporalHostPort   string
	NATSURL            string

	// Doppler Secrets Configuration
	DopplerToken   string
	DopplerProject string
	DopplerConfig  string

	// Supabase Configuration
	SupabaseURL           string
	SupabasePublishableKey string
	SupabaseSecretKey     string
	SupabaseJWKSURL       string

	// OpenRouter AI Multi-Model Gateway
	OpenRouterAPIKey string
}

// Load reads from environment variables with fallback defaults.
func Load() *AppConfig {
	_ = godotenv.Load(".env")
	_ = godotenv.Load("../.env")

	return &AppConfig{
		Environment:        getEnv("ENVIRONMENT", "development"),
		Port:               getEnv("PORT", "8081"),
		DatabaseURL:        getEnv("DATABASE_URL", "postgresql://codehound:devpassword@localhost:5433/codehound_dev?sslmode=disable"),
		RedisURL:           getEnv("REDIS_URL", "redis://localhost:6379/0"),
		AWSRegion:          getEnv("AWS_DEFAULT_REGION", "us-east-1"),
		AWSEndpointURL:     getEnv("AWS_ENDPOINT_URL", "http://localhost:4566"),
		AWSAccessKeyID:     getEnv("AWS_ACCESS_KEY_ID", "test"),
		AWSSecretAccessKey: getEnv("AWS_SECRET_ACCESS_KEY", "test"),
		ArtifactsBucket:    getEnv("ARTIFACTS_BUCKET", "codehound-artifacts-local"),
		TemporalHostPort:   getEnv("TEMPORAL_HOST_PORT", "localhost:7233"),
		NATSURL:            getEnv("NATS_URL", "nats://localhost:4222"),
		DopplerToken:          getEnv("DOPPLER_TOKEN", ""),
		DopplerProject:        getEnv("DOPPLER_PROJECT", "codehound"),
		DopplerConfig:         getEnv("DOPPLER_CONFIG", "dev"),
		SupabaseURL:           getEnv("SUPABASE_URL", ""),
		SupabasePublishableKey: getEnv("SUPABASE_PUBLISHABLE_KEY", ""),
		SupabaseSecretKey:     getEnv("SUPABASE_SECRET_KEY", ""),
		SupabaseJWKSURL:       getEnv("SUPABASE_JWKS_URL", ""),
		OpenRouterAPIKey:      getEnv("OPENROUTER_API_KEY", ""),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
