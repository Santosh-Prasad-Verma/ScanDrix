package devservercli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/auth/cliauth"
	"github.com/scandrix/backend/internal/auth/mailer"
	"github.com/scandrix/backend/internal/auth/oauth"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/review"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/internal/storage"
	"github.com/scandrix/backend/pkg/models"
)

// StartDevServer boots an instant in-memory ScanDrix API server for local testing.
func StartDevServer(port int) error {
	if port <= 0 {
		port = 8080
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = os.Getenv("API_JWT_SECRET")
	}

	authService := auth.NewAuthenticator(jwtSecret)
	streamHub := review.NewStreamHub()
	evaluator, _ := rules.NewEvaluator(rules.DefaultCatalog())
	inMemStore := cliauth.NewInMemorySessionStore()
	deviceFlow := cliauth.NewDeviceFlowManager(inMemStore, fmt.Sprintf("http://127.0.0.1:%d", port))
	artifactClient := storage.NewArtifactClient("", "", "")
	aiGateway := llm.NewGateway("", "", "", "")
	orchestrator := review.NewOrchestrator(nil, aiGateway, artifactClient, evaluator)

	oauthService := oauth.NewOAuthService(
		oauth.ProviderConfig{ClientID: os.Getenv("GITHUB_OAUTH_CLIENT_ID"), ClientSecret: os.Getenv("GITHUB_OAUTH_CLIENT_SECRET")},
		oauth.ProviderConfig{ClientID: os.Getenv("GITLAB_OAUTH_CLIENT_ID"), ClientSecret: os.Getenv("GITLAB_OAUTH_CLIENT_SECRET")},
	)
	mockMailer := mailer.NewNoopSender()

	router := api.BuildRouter(api.RouterConfig{
		Repo:         nil,
		AuthService:  authService,
		Orchestrator: orchestrator,
		StreamHub:    streamHub,
		Evaluator:    evaluator,
		SCIMService:  nil,
		DeviceFlow:   deviceFlow,
		OAuthService: oauthService,
		Mailer:       mockMailer,
		AppBaseURL:   fmt.Sprintf("http://127.0.0.1:%d", port),
		JWTSecret:    jwtSecret,
	})

	// Pre-create an active session for instant verification
	demoUserID := uuid.New()
	demoWsID := uuid.New()
	token, demoRefresh, _ := authService.GenerateTokenPairWithEmail(demoUserID, demoWsID, models.RoleOwner, "developer@scandrix.dev")

	demoSession := &cliauth.CLIDeviceSession{
		UUID:         uuid.New(),
		DeviceCode:   "DEMO-DEVICE-" + uuid.New().String()[:8],
		UserCode:     "DEMO-" + uuid.New().String()[:4],
		Status:       cliauth.StatusCompleted,
		AccessToken:  token,
		RefreshToken: demoRefresh,
		UserEmail:    "developer@scandrix.dev",
		ExpiresAt:    time.Now().Add(60 * time.Minute),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	_ = inMemStore.CreateSession(context.Background(), demoSession)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	fmt.Printf("🌐 ScanDrix Local API Server running on http://localhost:%d\n", port)
	fmt.Printf("   Device Authorization URL : http://localhost:%d/cli/authorize\n", port)
	fmt.Printf("   Health Check Endpoint    : http://localhost:%d/health\n", port)
	fmt.Println("Press Ctrl+C to exit.")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	serverErr := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	case <-stop:
		fmt.Println("\nShutting down ScanDrix Local API Server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}
