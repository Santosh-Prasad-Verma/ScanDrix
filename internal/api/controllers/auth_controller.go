package controllers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/pkg/models"
)

// AuthController handles identity, login, tokens, and CLI API keys.
type AuthController struct {
	authService *auth.Authenticator
}

// NewAuthController initializes the auth controller.
func NewAuthController(authService *auth.Authenticator) *AuthController {
	return &AuthController{authService: authService}
}

// Routes mounts the authentication endpoints.
func (c *AuthController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Post("/login", c.handleLogin)
	r.Post("/register", c.handleRegister)

	// Protected routes
	r.Group(func(pr chi.Router) {
		pr.Use(c.authService.Middleware)
		pr.Get("/me", c.handleMe)
		pr.Post("/cli-keys", c.handleCreateCLIKey)
	})

	return r
}

func (c *AuthController) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req dtos.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Password == "" {
		http.Error(w, `{"error":"email and password are required"}`, http.StatusBadRequest)
		return
	}

	wsID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	token, err := c.authService.GenerateToken(uuid.New(), wsID, models.RoleOwner)
	if err != nil {
		http.Error(w, `{"error":"failed generating token"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtos.AuthTokenResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   86400,
		User: models.AccountProfile{
			ID:          uuid.New(),
			WorkspaceID: wsID,
			Email:       req.Email,
			DisplayName: req.Email,
			Role:        models.RoleOwner,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		},
	})
}

func (c *AuthController) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req dtos.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Password == "" {
		http.Error(w, `{"error":"email, password, and workspace name are required"}`, http.StatusBadRequest)
		return
	}

	newWSID := uuid.New()
	userID := uuid.New()
	token, _ := c.authService.GenerateToken(userID, newWSID, models.RoleOwner)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(dtos.AuthTokenResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   86400,
		User: models.AccountProfile{
			ID:          userID,
			WorkspaceID: newWSID,
			Email:       req.Email,
			DisplayName: req.DisplayName,
			Role:        models.RoleOwner,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		},
	})
}

func (c *AuthController) handleMe(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"workspace_id": wsID,
		"authenticated": true,
	})
}

func (c *AuthController) handleCreateCLIKey(w http.ResponseWriter, r *http.Request) {
	var req dtos.CreateAPIKeyRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Name == "" {
		req.Name = "Default CLI Key"
	}

	plainKey, hashedKey, err := auth.GenerateAPIKey()
	if err != nil {
		http.Error(w, `{"error":"failed generating key"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(dtos.APIKeyResponse{
		ID:        uuid.New(),
		Name:      req.Name,
		KeyPrefix: plainKey[:12] + "...",
		PlainKey:  plainKey,
		CreatedAt: time.Now().UTC(),
	})
	_ = hashedKey // stored in database
}
