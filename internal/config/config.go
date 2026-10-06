package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// ═══════════════════════════════════════════════════════════════
// 1. RUNTIME TARGETS & ENVIRONMENT CONSTANTS (Deployment environment targets)
// ═══════════════════════════════════════════════════════════════

// AppEnvironment represents the runtime target (development, staging, production).
type AppEnvironment string

const (
	EnvDevelopment AppEnvironment = "development"
	EnvStaging     AppEnvironment = "staging"
	EnvProduction  AppEnvironment = "production"
)

// ═══════════════════════════════════════════════════════════════
// 2. CONFIG SCHEMA & SETTINGS DEFINITION (Enterprise configuration model)
// ═══════════════════════════════════════════════════════════════

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
	JWTSecret                string
	KMSMasterKey             string
	SCIMBearerToken          string
	HelpdeskJWTPrivateKeyPEM string
	AirGapped                bool
	AirGapAllowedHosts       []string

	// Web App & OAuth Configuration
	AppBaseURL                 string
	GitHubOAuthClientID        string
	GitHubOAuthClientSecret    string
	GitLabOAuthClientID        string
	GitLabOAuthClientSecret    string
	BitbucketOAuthClientID     string
	BitbucketOAuthClientSecret string
	GitHubOAuthRedirectURI     string
	GitLabOAuthRedirectURI     string
	BitbucketOAuthRedirectURI  string

	// Transactional Email / SMTP Configuration
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string

	// Registration Anti-Abuse & Email Security
	RequireEmailVerification bool
	BlockedEmailDomains      []string
	TurnstileSecretKey       string

	// SCM API & Webhook Secrets
	GitHubToken              string
	GitLabToken              string
	GitHubWebhookSecret      string
	GitLabWebhookSecret      string
	BitbucketUsername        string
	BitbucketPassword        string
	BitbucketWebhookSecret   string
	AzureDevOpsPAT           string
	AzureDevOpsWebhookSecret string
	ForgejoToken             string
	ForgejoWebhookSecret     string
	ForgejoBaseURL           string

	// AI Engine Credentials (BYOK / Enterprise Cloud)
	AnthropicAPIKey  string
	OpenAIAPIKey     string
	OpenAIBaseURL    string
	GeminiAPIKey     string
	DeepSeekAPIKey   string
	OpenRouterAPIKey string
	BedrockToken     string
	BedrockRegion    string
	VertexToken      string
	VertexProject    string
	VertexLocation   string
	VLLMEndpoint     string
	LocalLLMEndpoint string
	MoonshotAPIKey   string
	AlibabaAPIKey    string
	MiniMaxAPIKey    string
	XAIAPIKey        string
	MistralAPIKey    string
	NovitaAPIKey     string
	GroqAPIKey       string
	CohereAPIKey     string

	// Ephemeral Sandbox Execution
	SandboxProvider          string
	E2BAPIKey                string
	E2BDomain                string
	E2BEndpoint              string
	E2BTemplateID            string
	E2BTemplateGraphID       string
	E2BProxyHost             string
	E2BProxyPort             string
	E2BProxyPassword         string
	E2BProxyMethod           string
	ProofOfFixSandboxEnabled bool

	// Multi-Model Configuration
	AIModelTriage      string
	AIModelLogic       string
	AIModelSecurity    string
	AIModelThreatModel string
	AIModelArbiter     string
	AIModelSynthesizer string
	AIModelDefault     string
	AIModelFallback    string

	// Razorpay Billing & Subscription Configuration
	RazorpayKeyID         string
	RazorpayKeySecret     string
	RazorpayWebhookSecret string
	BillingWebhookSecret  string

	// Azure Code Management Token Crypto
	CodeManagementSecret       string
	CodeManagementWebhookToken string

	// Asynchronous Worker Tuning
	WorkerRole           string
	WorkerHealthPort     int
	WorkerDrainTimeoutMs int
	WorkerConcurrency    int
}

// ═══════════════════════════════════════════════════════════════
// 3. ENVIRONMENT VARIABLE LOADER (.env discovery & runtime mapping)
// ═══════════════════════════════════════════════════════════════

// Load reads and validates configuration from environment variables and local .env files.
// In accordance with Master Rule 1.6, it fails fast if critical variables are missing.
func Load() (*Config, error) {
	// Cascade: .env.local (per-dev overrides) wins, .env provides team baseline defaults.
	_ = godotenv.Load(".env.local")
	_ = godotenv.Load(".env")
	_ = godotenv.Load("../.env.local")
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

	webhooksPort, err := strconv.Atoi(getEnvOrDefault("WEBHOOKS_PORT", getEnvOrDefault("API_WEBHOOKS_PORT", "8081")))
	if err != nil {
		return nil, fmt.Errorf("invalid WEBHOOKS_PORT value: %w", err)
	}

	smtpPort, _ := strconv.Atoi(getEnvOrDefault("SMTP_PORT", "587"))
	smtpHost := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	smtpUser := os.Getenv("SMTP_USERNAME")
	smtpPass := os.Getenv("SMTP_PASSWORD")
	resendKey := strings.TrimSpace(os.Getenv("RESEND_API_KEY"))
	if smtpHost == "" && resendKey != "" {
		smtpHost = "smtp.resend.com"
		if smtpUser == "" {
			smtpUser = "resend"
		}
		if smtpPass == "" {
			smtpPass = resendKey
		}
	}

	workerRole := strings.ToLower(getEnvOrDefault("WORKER_ROLE", "all"))
	workerHealthPort, _ := strconv.Atoi(getEnvOrDefault("WORKER_HEALTH_PORT", getEnvOrDefault("API_WORKER_PORT", "8082")))
	if workerHealthPort <= 0 {
		workerHealthPort = 8082
	}
	workerDrainTimeoutMs, _ := strconv.Atoi(getEnvOrDefault("API_WORKER_DRAIN_TIMEOUT_MS", getEnvOrDefault("WORKER_DRAIN_TIMEOUT_MS", "25000")))
	if workerDrainTimeoutMs <= 0 {
		workerDrainTimeoutMs = 25000
	}
	workerConcurrency, _ := strconv.Atoi(getEnvOrDefault("WORKER_CONCURRENCY", "8"))
	if workerConcurrency <= 0 {
		workerConcurrency = 8
	}

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
		Environment:                appEnv,
		APIPort:                    apiPort,
		WebhooksPort:               webhooksPort,
		DatabaseURL:                dbURL,
		RabbitMQURL:                rmqURL,
		RedisURL:                   getEnvOrDefault("REDIS_URL", "redis://localhost:6379"),
		AppwriteEndpoint:           getEnvOrDefault("APPWRITE_ENDPOINT", "https://sgp.cloud.appwrite.io/v1"),
		AppwriteProjectID:          os.Getenv("APPWRITE_PROJECT_ID"),
		AppwriteAPIKey:             os.Getenv("APPWRITE_API_KEY"),
		GitHubToken:                os.Getenv("GITHUB_TOKEN"),
		GitLabToken:                os.Getenv("GITLAB_TOKEN"),
		GitHubWebhookSecret:        os.Getenv("GITHUB_WEBHOOK_SECRET"),
		GitLabWebhookSecret:        os.Getenv("GITLAB_WEBHOOK_SECRET"),
		BitbucketUsername:          os.Getenv("BITBUCKET_USERNAME"),
		BitbucketPassword:          os.Getenv("BITBUCKET_PASSWORD"),
		BitbucketWebhookSecret:     os.Getenv("BITBUCKET_WEBHOOK_SECRET"),
		AzureDevOpsPAT:             os.Getenv("AZURE_DEVOPS_PAT"),
		AzureDevOpsWebhookSecret:   os.Getenv("AZURE_DEVOPS_WEBHOOK_SECRET"),
		ForgejoToken:               os.Getenv("FORGEJO_TOKEN"),
		ForgejoWebhookSecret:       os.Getenv("FORGEJO_WEBHOOK_SECRET"),
		ForgejoBaseURL:             getEnvOrDefault("FORGEJO_BASE_URL", "https://codeberg.org"),
		JWTSecret:                  os.Getenv("JWT_SECRET"),
		KMSMasterKey:               os.Getenv("KMS_MASTER_KEY"),
		SCIMBearerToken:            os.Getenv("SCIM_BEARER_TOKEN"),
		HelpdeskJWTPrivateKeyPEM:   os.Getenv("HELPDESK_JWT_PRIVATE_KEY_PEM"),
		AirGapped:                  os.Getenv("AIR_GAPPED") == "true",
		AirGapAllowedHosts:         parseCommaSeparated(getEnvOrDefault("AIRGAP_ALLOWED_HOSTS", "localhost,127.0.0.1,::1")),
		AppBaseURL:                 getEnvOrDefault("APP_BASE_URL", "http://localhost:3000"),
		GitHubOAuthClientID:        os.Getenv("GITHUB_OAUTH_CLIENT_ID"),
		GitHubOAuthClientSecret:    os.Getenv("GITHUB_OAUTH_CLIENT_SECRET"),
		GitLabOAuthClientID:        os.Getenv("GITLAB_OAUTH_CLIENT_ID"),
		GitLabOAuthClientSecret:    os.Getenv("GITLAB_OAUTH_CLIENT_SECRET"),
		BitbucketOAuthClientID:     os.Getenv("BITBUCKET_OAUTH_CLIENT_ID"),
		BitbucketOAuthClientSecret: os.Getenv("BITBUCKET_OAUTH_CLIENT_SECRET"),
		GitHubOAuthRedirectURI:     os.Getenv("GITHUB_OAUTH_REDIRECT_URI"),
		GitLabOAuthRedirectURI:     getEnvOrDefault("GITLAB_OAUTH_REDIRECT_URI", os.Getenv("GLOBAL_GITLAB_REDIRECT_URL")),
		BitbucketOAuthRedirectURI:  os.Getenv("BITBUCKET_OAUTH_REDIRECT_URI"),
		SMTPHost:                   smtpHost,
		SMTPPort:                   smtpPort,
		SMTPUsername:               smtpUser,
		SMTPPassword:               smtpPass,
		SMTPFrom:                   getEnvOrDefault("SMTP_FROM", "no-reply@scandrix.dev"),
		RequireEmailVerification:   os.Getenv("REQUIRE_EMAIL_VERIFICATION") == "true",
		BlockedEmailDomains:        parseCommaSeparated(os.Getenv("BLOCKED_EMAIL_DOMAINS")),
		TurnstileSecretKey:         os.Getenv("TURNSTILE_SECRET_KEY"),
		AnthropicAPIKey:            getEnvOrDefault("ANTHROPIC_API_KEY", os.Getenv("API_ANTHROPIC_API_KEY")),
		OpenAIAPIKey:               getEnvOrDefault("OPENAI_API_KEY", os.Getenv("API_OPEN_AI_API_KEY")),
		OpenAIBaseURL:              getEnvOrDefault("OPENAI_BASE_URL", getEnvOrDefault("API_OPENAI_FORCE_BASE_URL", "https://api.openai.com/v1")),
		GeminiAPIKey:               getEnvOrDefault("GEMINI_API_KEY", os.Getenv("API_GOOGLE_AI_API_KEY")),
		DeepSeekAPIKey:             getEnvOrDefault("DEEPSEEK_API_KEY", os.Getenv("API_DEEPSEEK_API_KEY")),
		OpenRouterAPIKey:           getEnvOrDefault("OPENROUTER_API_KEY", os.Getenv("API_OPEN_ROUTER_API_KEY")),
		BedrockToken:               os.Getenv("BEDROCK_BEARER_TOKEN"),
		BedrockRegion:              getEnvOrDefault("BEDROCK_REGION", "us-east-1"),
		VertexToken:                os.Getenv("VERTEX_ACCESS_TOKEN"),
		VertexProject:              os.Getenv("VERTEX_PROJECT_ID"),
		VertexLocation:             getEnvOrDefault("VERTEX_LOCATION", "us-central1"),
		VLLMEndpoint:               getEnvOrDefault("VLLM_ENDPOINT", "http://localhost:8000"),
		LocalLLMEndpoint:           os.Getenv("VLLM_ENDPOINT"),
		MoonshotAPIKey:             getEnvOrDefault("MOONSHOT_API_KEY", os.Getenv("API_MOONSHOT_API_KEY")),
		AlibabaAPIKey:              os.Getenv("ALIBABA_API_KEY"),
		MiniMaxAPIKey:              os.Getenv("MINIMAX_API_KEY"),
		XAIAPIKey:                  os.Getenv("XAI_API_KEY"),
		MistralAPIKey:              getEnvOrDefault("MISTRAL_API_KEY", os.Getenv("API_MISTRAL_API_KEY")),
		NovitaAPIKey:               getEnvOrDefault("NOVITA_API_KEY", os.Getenv("API_NOVITA_AI_API_KEY")),
		GroqAPIKey:                 getEnvOrDefault("GROQ_API_KEY", os.Getenv("API_GROQ_API_KEY")),
		CohereAPIKey:               getEnvOrDefault("COHERE_API_KEY", os.Getenv("API_COHERE_API_KEY")),
		SandboxProvider:            getEnvOrDefault("SANDBOX_PROVIDER", "auto"),
		E2BAPIKey:                  getEnvOrDefault("E2B_API_KEY", os.Getenv("API_E2B_KEY")),
		E2BDomain:                  getEnvOrDefault("E2B_DOMAIN", "e2b.dev"),
		E2BEndpoint:                getEnvOrDefault("E2B_ENDPOINT", os.Getenv("E2B_API_URL")),
		E2BTemplateID:              getEnvOrDefault("E2B_TEMPLATE_ID", os.Getenv("API_E2B_TEMPLATE_ID")),
		E2BTemplateGraphID:         getEnvOrDefault("E2B_TEMPLATE_GRAPH_ID", os.Getenv("API_E2B_TEMPLATE_GRAPH_ID")),
		E2BProxyHost:               os.Getenv("E2B_PROXY_HOST"),
		E2BProxyPort:               getEnvOrDefault("E2B_PROXY_PORT", "8388"),
		E2BProxyPassword:           os.Getenv("E2B_PROXY_PASSWORD"),
		E2BProxyMethod:             getEnvOrDefault("E2B_PROXY_METHOD", "aes-256-gcm"),
		AIModelTriage:              os.Getenv("AI_MODEL_TRIAGE"),
		AIModelLogic:               os.Getenv("AI_MODEL_LOGIC"),
		AIModelSecurity:            os.Getenv("AI_MODEL_SECURITY"),
		AIModelThreatModel:         os.Getenv("AI_MODEL_THREAT_MODEL"),
		AIModelArbiter:             os.Getenv("AI_MODEL_ARBITER"),
		AIModelSynthesizer:         os.Getenv("AI_MODEL_SYNTHESIZER"),
		AIModelDefault:             getEnvOrDefault("AI_MODEL_DEFAULT", os.Getenv("API_LLM_PROVIDER_MODEL")),
		AIModelFallback:            os.Getenv("AI_MODEL_FALLBACK"),
		RazorpayKeyID:              os.Getenv("RAZORPAY_KEY_ID"),
		RazorpayKeySecret:          os.Getenv("RAZORPAY_KEY_SECRET"),
		RazorpayWebhookSecret:      os.Getenv("RAZORPAY_WEBHOOK_SECRET"),
		BillingWebhookSecret:       getBillingWebhookSecret(),
		CodeManagementSecret:       os.Getenv("CODE_MANAGEMENT_SECRET"),
		CodeManagementWebhookToken: os.Getenv("CODE_MANAGEMENT_WEBHOOK_TOKEN"),
		WorkerRole:                 workerRole,
		WorkerHealthPort:           workerHealthPort,
		WorkerDrainTimeoutMs:       workerDrainTimeoutMs,
		WorkerConcurrency:          workerConcurrency,
	}

	// ═══════════════════════════════════════════════════════════════
	// 4. ENTERPRISE INTEGRITY & SECRET VALIDATION (Fail-fast security rules)
	// ═══════════════════════════════════════════════════════════════

	var validationErrors []string

	if cfg.DatabaseURL == "" && appEnv == EnvProduction {
		validationErrors = append(validationErrors, "DATABASE_URL is required (Master Rule 1.6)")
	}

	if appEnv == EnvProduction || appEnv == EnvStaging {
		if strings.TrimSpace(cfg.JWTSecret) == "" {
			validationErrors = append(validationErrors, "JWT_SECRET is required in production and staging (Master Rule 1.1 & 1.6)")
		}
		// Appwrite is deliberately NOT a startup requirement (F-63).
		//
		// It is only used to archive review diffs, and every call site already
		// degrades: NewArtifactClient accepts empty credentials, the orchestrator
		// guards on a nil client, and an upload failure is a warning, not an
		// error. Requiring it here meant an optional artifact-archive feature
		// would crash-loop the whole API in production -- taking auth, findings
		// and dashboards down with it -- and it defeated the :? compose guard
		// differently for Terraform, which passed neither variable at all.
		//
		// This is deliberately different from SMTP_HOST just below. A missing
		// mailer silently discards password-reset and confirmation email and
		// locks users out of their own accounts, which is a security-relevant
		// failure. A missing artifact store loses review history and nothing
		// else. The two are not the same class of dependency.
		if appEnv == EnvProduction && (cfg.AppwriteProjectID == "" || cfg.AppwriteAPIKey == "") {
			slog.Warn("Appwrite storage is not configured: review diff and audit-report " +
				"archival will be skipped. Set APPWRITE_PROJECT_ID and APPWRITE_API_KEY to enable it. " +
				"No other feature depends on this.")
		}
		// Without SMTP_HOST the mailer returns ErrSMTPNotConfigured for every
		// send, so password reset and email confirmation deliver nothing while
		// appearing to succeed. That locks users out of their accounts, so it
		// is a hard startup failure rather than a warning
		// (AUDIT_REMEDIATION.md F-04).
		if strings.TrimSpace(cfg.SMTPHost) == "" {
			validationErrors = append(validationErrors,
				"SMTP_HOST is required in production and staging; without it all transactional email (password reset, email confirmation, billing) is silently discarded")
		}
		if cfg.AnthropicAPIKey == "" && cfg.OpenAIAPIKey == "" && cfg.GeminiAPIKey == "" &&
			cfg.DeepSeekAPIKey == "" && cfg.OpenRouterAPIKey == "" && cfg.BedrockToken == "" && cfg.VertexToken == "" &&
			cfg.VLLMEndpoint == "" && cfg.MistralAPIKey == "" && cfg.GroqAPIKey == "" && cfg.CohereAPIKey == "" {
			validationErrors = append(validationErrors, "At least one AI provider key or local LLM endpoint must be configured in environment variables")
		}
	}

	if len(validationErrors) > 0 {
		return nil, errors.New("configuration validation failed:\n - " + strings.Join(validationErrors, "\n - "))
	}

	return cfg, nil
}

// ═══════════════════════════════════════════════════════════════
// 5. TYPE CONVERSION & SANITIZATION HELPERS (Fallback strings & slices)
// ═══════════════════════════════════════════════════════════════

func getEnvOrDefault(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getBillingWebhookSecret() string {
	if val := os.Getenv("API_BILLING_WEBHOOK_SECRET"); val != "" {
		return val
	}
	if val := os.Getenv("BILLING_WEBHOOK_SECRET"); val != "" {
		return val
	}
	return os.Getenv("RAZORPAY_WEBHOOK_SECRET")
}

func parseCommaSeparated(val string) []string {
	trimmed := strings.TrimSpace(val)
	if trimmed == "" {
		return nil
	}
	var res []string
	for _, part := range strings.Split(trimmed, ",") {
		if p := strings.TrimSpace(part); p != "" {
			res = append(res, p)
		}
	}
	return res
}
