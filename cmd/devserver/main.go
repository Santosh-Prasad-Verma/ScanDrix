package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
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

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	port := 8080
	slog.Info("Starting ScanDrix Local Verification API Server", "port", port)

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
	token, _, _ := authService.GenerateTokenPairWithEmail(demoUserID, demoWsID, models.RoleOwner, "developer@scandrix.dev")

	demoSession := &cliauth.CLIDeviceSession{
		UUID:         uuid.New(),
		DeviceCode:   "DEMO-DEVICE-CODE-12345",
		UserCode:     "DEMO-CODE",
		Status:       cliauth.StatusCompleted,
		AccessToken:  token,
		RefreshToken: "demo-refresh-token",
		UserEmail:    "developer@scandrix.dev",
		ExpiresAt:    time.Now().Add(10 * time.Minute),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	_ = inMemStore.CreateSession(context.Background(), demoSession)

	slog.Info("Demo credentials initialized", "demo_device_code", "DEMO-DEVICE-CODE-12345", "user_code", "DEMO-CODE")

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("Server error", "error", err)
	}
}
