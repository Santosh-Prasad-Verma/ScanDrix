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

	// Security & Auth Secrets
	JWTSecret    string
	KMSMasterKey string

	// Web App & OAuth Configuration
	AppBaseURL             string
	GitHubOAuthClientID     string
	GitHubOAuthClientSecret string
	GitLabOAuthClientID     string
	GitLabOAuthClientSecret string

	// Transactional Email / SMTP Configuration
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string

	// SCM API & Webhook Secrets
	GitHubToken               string
	GitHubWebhookSecret       string
	GitLabWebhookSecret       string
	BitbucketUsername         string
	BitbucketPassword         string
	BitbucketWebhookSecret     string
	AzureDevOpsPAT            string
	AzureDevOpsWebhookSecret  string
	ForgejoToken              string
	ForgejoWebhookSecret      string
	ForgejoBaseURL            string

	// AI Engine Credentials (BYOK / Enterprise Cloud)
	AnthropicAPIKey   string
	OpenAIAPIKey      string
	GeminiAPIKey      string
	DeepSeekAPIKey    string
	OpenRouterAPIKey  string
	BedrockToken      string
	BedrockRegion     string
	VertexToken       string
	VertexProject     string
	VertexLocation    string
	OllamaEndpoint    string
	VLLMEndpoint      string
	LocalLLMEndpoint  string
	MoonshotAPIKey    string
	AlibabaAPIKey     string
	MiniMaxAPIKey     string
	XAIAPIKey         string
	MistralAPIKey     string

	// Ephemeral Sandbox Execution
	E2BAPIKey                string
	E2BEndpoint              string
	ProofOfFixSandboxEnabled bool

	// OpenRouter Multi-Model Configuration
	AIModelTriage      string
	AIModelLogic       string
	AIModelSecurity    string
	AIModelThreatModel string
	AIModelArbiter     string
	AIModelSynthesizer string
	AIModelDefault     string

	// Razorpay Billing & Subscription Configuration
	RazorpayKeyID         string
	RazorpayKeySecret     string
	RazorpayWebhookSecret string
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

	portVal := os.Getenv("PORT")
	if portVal == "" {
		portVal = os.Getenv("API_PORT")
	}
	if portVal == "" {
		portVal = "8080"
	}
	apiPort, err := strconv.Atoi(portVal)
	if err != nil {
		return nil, fmt.Errorf("invalid PORT / API_PORT value: %w", err)
	}

	webhooksPort, err := strconv.Atoi(getEnvOrDefault("WEBHOOKS_PORT", "8081"))
	if err != nil {
		return nil, fmt.Errorf("invalid WEBHOOKS_PORT value: %w", err)
	}

	smtpPort, _ := strconv.Atoi(getEnvOrDefault("SMTP_PORT", "587"))

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("SUPABASE_DATABASE_URL")
	}
	if dbURL == "" {
		dbURL = os.Getenv("SUPABASE_POOLER_URL")
	}

	rmqURL := os.Getenv("RABBITMQ_URL")
	if rmqURL == "" {
		rmqURL = os.Getenv("API_RABBITMQ_URI")
	}
	if rmqURL == "" {
		rmqURL = os.Getenv("CLOUDAMQP_URL")
	}
	if rmqURL == "" {
		rmqURL = "amqp://guest:guest@localhost:5672/"
	}

	cfg := &Config{
		Environment:               appEnv,
		APIPort:                   apiPort,
		WebhooksPort:              webhooksPort,
		DatabaseURL:               dbURL,
		RabbitMQURL:               rmqURL,
		RedisURL:                  getEnvOrDefault("REDIS_URL", "redis://localhost:6379"),
		AppwriteEndpoint:          getEnvOrDefault("APPWRITE_ENDPOINT", "https://sgp.cloud.appwrite.io/v1"),
		AppwriteProjectID:         os.Getenv("APPWRITE_PROJECT_ID"),
		AppwriteAPIKey:            os.Getenv("APPWRITE_API_KEY"),
		GitHubToken:               os.Getenv("GITHUB_TOKEN"),
		GitHubWebhookSecret:       os.Getenv("GITHUB_WEBHOOK_SECRET"),
		GitLabWebhookSecret:       os.Getenv("GITLAB_WEBHOOK_SECRET"),
		BitbucketUsername:         os.Getenv("BITBUCKET_USERNAME"),
		BitbucketPassword:         os.Getenv("BITBUCKET_PASSWORD"),
		BitbucketWebhookSecret:    os.Getenv("BITBUCKET_WEBHOOK_SECRET"),
		AzureDevOpsPAT:            os.Getenv("AZURE_DEVOPS_PAT"),
		AzureDevOpsWebhookSecret:  os.Getenv("AZURE_DEVOPS_WEBHOOK_SECRET"),
		ForgejoToken:              os.Getenv("FORGEJO_TOKEN"),
		ForgejoWebhookSecret:      os.Getenv("FORGEJO_WEBHOOK_SECRET"),
		ForgejoBaseURL:            getEnvOrDefault("FORGEJO_BASE_URL", "https://codeberg.org"),
		JWTSecret:                 os.Getenv("JWT_SECRET"),
		KMSMasterKey:              os.Getenv("KMS_MASTER_KEY"),
		AppBaseURL:                getEnvOrDefault("APP_BASE_URL", "http://localhost:3000"),
		GitHubOAuthClientID:       os.Getenv("GITHUB_OAUTH_CLIENT_ID"),
		GitHubOAuthClientSecret:   os.Getenv("GITHUB_OAUTH_CLIENT_SECRET"),
		GitLabOAuthClientID:       os.Getenv("GITLAB_OAUTH_CLIENT_ID"),
		GitLabOAuthClientSecret:   os.Getenv("GITLAB_OAUTH_CLIENT_SECRET"),
		SMTPHost:                  os.Getenv("SMTP_HOST"),
		SMTPPort:                  smtpPort,
		SMTPUsername:              os.Getenv("SMTP_USERNAME"),
		SMTPPassword:              os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:                  getEnvOrDefault("SMTP_FROM", "no-reply@scandrix.dev"),
		AnthropicAPIKey:           os.Getenv("ANTHROPIC_API_KEY"),
		OpenAIAPIKey:              os.Getenv("OPENAI_API_KEY"),
		GeminiAPIKey:              os.Getenv("GEMINI_API_KEY"),
		DeepSeekAPIKey:            os.Getenv("DEEPSEEK_API_KEY"),
		OpenRouterAPIKey:          os.Getenv("OPENROUTER_API_KEY"),
		BedrockToken:              os.Getenv("BEDROCK_BEARER_TOKEN"),
		BedrockRegion:             getEnvOrDefault("BEDROCK_REGION", "us-east-1"),
		VertexToken:               os.Getenv("VERTEX_ACCESS_TOKEN"),
		VertexProject:             os.Getenv("VERTEX_PROJECT_ID"),
		VertexLocation:            getEnvOrDefault("VERTEX_LOCATION", "us-central1"),
		OllamaEndpoint:            getEnvOrDefault("OLLAMA_ENDPOINT", "http://localhost:11434"),
		VLLMEndpoint:              getEnvOrDefault("VLLM_ENDPOINT", "http://localhost:8000"),
		LocalLLMEndpoint:          os.Getenv("VLLM_ENDPOINT"),
		MoonshotAPIKey:            os.Getenv("MOONSHOT_API_KEY"),
		AlibabaAPIKey:             os.Getenv("ALIBABA_API_KEY"),
		MiniMaxAPIKey:             os.Getenv("MINIMAX_API_KEY"),
		XAIAPIKey:                 os.Getenv("XAI_API_KEY"),
		MistralAPIKey:             os.Getenv("MISTRAL_API_KEY"),
		E2BAPIKey:                 os.Getenv("E2B_API_KEY"),
		E2BEndpoint:               os.Getenv("E2B_ENDPOINT"),
		ProofOfFixSandboxEnabled:  os.Getenv("PROOFOFFIX_SANDBOX_ENABLED") == "true",
		AIModelTriage:             getEnvOrDefault("AI_MODEL_TRIAGE", "minimax/minimax-m3:free"),
		AIModelLogic:              getEnvOrDefault("AI_MODEL_LOGIC", "nvidia/nemotron-3-ultra-550b-a55b:free"),
		AIModelSecurity:           getEnvOrDefault("AI_MODEL_SECURITY", "stealth/ox-alpha"),
		AIModelThreatModel:        getEnvOrDefault("AI_MODEL_THREAT_MODEL", "thinkingmachines/inkling:free"),
		AIModelArbiter:            getEnvOrDefault("AI_MODEL_ARBITER", "stealth/ox-alpha"),
		AIModelSynthesizer:        getEnvOrDefault("AI_MODEL_SYNTHESIZER", "thinkingmachines/inkling:free"),
		AIModelDefault:            getEnvOrDefault("AI_MODEL_DEFAULT", "stealth/ox-alpha"),
		RazorpayKeyID:             os.Getenv("RAZORPAY_KEY_ID"),
		RazorpayKeySecret:         os.Getenv("RAZORPAY_KEY_SECRET"),
		RazorpayWebhookSecret:     os.Getenv("RAZORPAY_WEBHOOK_SECRET"),
	}

	// Validate required variables
	var validationErrors []string

	if cfg.DatabaseURL == "" && appEnv == EnvProduction {
		validationErrors = append(validationErrors, "DATABASE_URL is required (Master Rule 1.6)")
	}

	if appEnv == EnvProduction {
		if cfg.JWTSecret == "" {
			validationErrors = append(validationErrors, "JWT_SECRET is required in production (Master Rule 1.1 & 1.6)")
		}
		if cfg.AppwriteProjectID == "" {
			validationErrors = append(validationErrors, "APPWRITE_PROJECT_ID is required in production")
		}
		if cfg.AppwriteAPIKey == "" {
			validationErrors = append(validationErrors, "APPWRITE_API_KEY is required in production")
		}
		if cfg.AnthropicAPIKey == "" && cfg.OpenAIAPIKey == "" && cfg.GeminiAPIKey == "" &&
			cfg.DeepSeekAPIKey == "" && cfg.OpenRouterAPIKey == "" && cfg.BedrockToken == "" && cfg.VertexToken == "" &&
			cfg.OllamaEndpoint == "" && cfg.VLLMEndpoint == "" {
			validationErrors = append(validationErrors, "At least one AI provider key or local LLM endpoint must be configured in environment variables")
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
