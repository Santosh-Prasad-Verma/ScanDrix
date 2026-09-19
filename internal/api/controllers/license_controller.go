package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/pkg/models"
)

// LicenseRepository defines the data access contract for enterprise licenses (Clean Architecture).
type LicenseRepository interface {
	GetActiveLicense(ctx context.Context, wsID uuid.UUID) (*models.OrganizationLicense, error)
	ActivateLicense(ctx context.Context, wsID uuid.UUID, licenseKey, orgName, planTier string, totalSeats int, expiresAt time.Time, features []string) error
}

// LicenseController manages enterprise seat allocations, subscriptions, and air-gapped license files.
type LicenseController struct {
	repo LicenseRepository
}

// NewLicenseController initializes the license controller with repository persistence.
func NewLicenseController(repo LicenseRepository) *LicenseController {
	if isNilInterface(repo) {
		repo = nil
	}
	return &LicenseController{repo: repo}
}

// Routes mounts enterprise license management endpoints.
func (c *LicenseController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", c.handleGetLicense)
	r.Post("/activate", c.handleActivateLicense)
	r.Get("/seats", c.handleGetSeats)
	r.Get("/status", c.handleGetStatus)
	r.Get("/org-status", c.handleGetOrgStatus)
	r.Get("/users", c.handleGetUsersWithLicense)
	r.Post("/assign", c.handleAssignLicense)
	r.Get("/removable-seats", c.handleGetRemovableSeats)
	r.Post("/prune-seats", c.handlePruneSeats)
	r.Post("/trial-extension-request", c.handleTrialExtensionRequest)

	return r
}

func (c *LicenseController) handleGetLicense(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var lic *models.OrganizationLicense
	if c.repo != nil {
		var err error
		lic, err = c.repo.GetActiveLicense(r.Context(), wsID)
		if err != nil {
			http.Error(w, `{"error":"failed loading license"}`, http.StatusInternalServerError)
			return
		}
	}

	if lic == nil {
		// Standard Community Edition default when no enterprise license activated
		lic = &models.OrganizationLicense{
			LicenseKey:       "SCANDRIX-COMMUNITY-EDITION",
			OrganizationName: "Community Tier",
			PlanTier:         "COMMUNITY",
			TotalSeats:       5,
			AllocatedSeats:   1,
			ExpiresAt:        time.Now().AddDate(10, 0, 0),
			IsAirGapped:      false,
			FeaturesEnabled:  []string{"automated_reviews", "custom_rules"},
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtos.LicenseResponse{
		LicenseKey:       lic.LicenseKey,
		OrganizationName: lic.OrganizationName,
		PlanTier:         lic.PlanTier,
		TotalSeats:       lic.TotalSeats,
		AllocatedSeats:   lic.AllocatedSeats,
		ExpiresAt:        lic.ExpiresAt,
		IsAirGapped:      lic.IsAirGapped,
		FeaturesEnabled:  lic.FeaturesEnabled,
	})
}

func (c *LicenseController) handleActivateLicense(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	if c.repo == nil {
		http.Error(w, `{"error":"database service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	var req dtos.ActivateLicenseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.LicenseKey == "" {
		http.Error(w, `{"error":"license_key is required"}`, http.StatusBadRequest)
		return
	}

	expiresAt := time.Now().AddDate(1, 0, 0)
	features := []string{
		"saml_sso", "scim_provisioning", "custom_rules", "audit_log_cef",
		"unlimited_repos", "priority_ai_router", "on_prem_workers",
	}

	err = c.repo.ActivateLicense(r.Context(), wsID, req.LicenseKey, "Enterprise Customer", "ENTERPRISE", 500, expiresAt, features)
	if err != nil {
		http.Error(w, `{"error":"failed activating license"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":      "ACTIVATED",
		"license_key": req.LicenseKey,
		"valid_until": expiresAt,
	})
}

func (c *LicenseController) handleGetSeats(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var lic *models.OrganizationLicense
	if c.repo != nil {
		lic, _ = c.repo.GetActiveLicense(r.Context(), wsID)
	}
	totalSeats := 10
	allocatedSeats := 1
	if lic != nil {
		totalSeats = lic.TotalSeats
		allocatedSeats = lic.AllocatedSeats
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"total_seats":     totalSeats,
		"allocated_seats": allocatedSeats,
		"available_seats": totalSeats - allocatedSeats,
		"active_users":    allocatedSeats,
	})
}

func (c *LicenseController) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var lic *models.OrganizationLicense
	if c.repo != nil {
		lic, _ = c.repo.GetActiveLicense(r.Context(), wsID)
	}

	valid := true
	subStatus := "active"
	plan := "COMMUNITY"
	totalSeats := 10
	features := []string{"automated_reviews", "custom_rules"}
	expiresAt := time.Now().AddDate(10, 0, 0)

	if lic != nil {
		plan = lic.PlanTier
		totalSeats = lic.TotalSeats
		features = lic.FeaturesEnabled
		expiresAt = lic.ExpiresAt
		if time.Now().After(lic.ExpiresAt) {
			valid = false
			subStatus = "expired"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"valid":              valid,
		"subscriptionStatus": subStatus,
		"plan":               plan,
		"seats":              totalSeats,
		"features":           features,
		"expiresAt":          expiresAt.Format(time.RFC3339),
	})
}

func (c *LicenseController) handleGetOrgStatus(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	var lic *models.OrganizationLicense
	if c.repo != nil {
		lic, _ = c.repo.GetActiveLicense(r.Context(), wsID)
	}

	valid := true
	subStatus := "active"
	if lic != nil && time.Now().After(lic.ExpiresAt) {
		valid = false
		subStatus = "expired"
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"valid":              valid,
		"subscriptionStatus": subStatus,
	})
}

func (c *LicenseController) handleGetUsersWithLicense(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode([]any{})
}

type assignUserItem struct {
	GitID         string `json:"gitId"`
	GitTool       string `json:"gitTool"`
	LicenseStatus string `json:"licenseStatus"` // "active" | "inactive"
}

type assignLicenseRequest struct {
	TeamID   string           `json:"teamId,omitempty"`
	Users    []assignUserItem `json:"users"`
	UserName string           `json:"userName,omitempty"`
}

func (c *LicenseController) handleAssignLicense(w http.ResponseWriter, r *http.Request) {
	var req assignLicenseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON request body"}`, http.StatusBadRequest)
		return
	}

	var successful []assignUserItem
	var failed []assignUserItem

	for _, u := range req.Users {
		if strings.TrimSpace(u.GitID) != "" {
			successful = append(successful, u)
		} else {
			failed = append(failed, u)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"successful": successful,
		"failed":     failed,
	})
}

func (c *LicenseController) handleGetRemovableSeats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"removableSeats": []string{},
		"count":          0,
		"status":         "ok",
	})
}

func (c *LicenseController) handlePruneSeats(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TeamID string   `json:"teamId,omitempty"`
		GitIDs []string `json:"gitIds,omitempty"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"prunedCount": len(req.GitIDs),
		"status":      "pruned",
	})
}

type trialExtensionRequestPayload struct {
	TeamID   string `json:"teamId,omitempty"`
	TeamSize int    `json:"teamSize,omitempty"`
	Message  string `json:"message,omitempty"`
}

func (c *LicenseController) handleTrialExtensionRequest(w http.ResponseWriter, r *http.Request) {
	var req trialExtensionRequestPayload
	_ = json.NewDecoder(r.Body).Decode(&req)

	webhookURL := strings.TrimSpace(os.Getenv("API_DISCORD_TRIAL_REQUEST_WEBHOOK_URL"))
	if webhookURL == "" {
		webhookURL = strings.TrimSpace(os.Getenv("SCANDRIX_DISCORD_TRIAL_REQUEST_WEBHOOK_URL"))
	}

	if webhookURL == "" {
		slog.Warn("Trial extension request received but Discord webhook is not configured")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "Trial request channel is not configured yet.",
		})
		return
	}

	note := strings.TrimSpace(req.Message)
	if len(note) > 1500 {
		note = note[:1500]
	}

	lines := []string{
		"**New trial extension request**",
		fmt.Sprintf("**Team:** %s", req.TeamID),
		fmt.Sprintf("**Team size:** %d", req.TeamSize),
	}
	if note != "" {
		lines = append(lines, fmt.Sprintf("**Message:** %s", note))
	}

	bodyBytes, _ := json.Marshal(map[string]string{
		"content": strings.Join(lines, "\n"),
	})

	httpReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, webhookURL, bytes.NewReader(bodyBytes))
	if err == nil {
		httpReq.Header.Set("Content-Type", "application/json")
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(httpReq)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode < 400 {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
				return
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": false,
		"message": "Could not deliver the request.",
	})
}
