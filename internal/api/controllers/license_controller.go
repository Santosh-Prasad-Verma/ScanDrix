package controllers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/internal/api/dtos"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/pkg/models"
)

// LicenseController manages enterprise seat allocations, subscriptions, and air-gapped license files.
type LicenseController struct {
	repo *database.Repository
}

// NewLicenseController initializes the license controller with database persistence.
func NewLicenseController(repo *database.Repository) *LicenseController {
	return &LicenseController{repo: repo}
}

// Routes mounts license endpoints.
func (c *LicenseController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", c.handleGetLicense)
	r.Post("/activate", c.handleActivateLicense)
	r.Get("/seats", c.handleGetSeats)

	return r
}

func (c *LicenseController) handleGetLicense(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	lic, err := c.repo.GetActiveLicense(r.Context(), wsID)
	if err != nil {
		http.Error(w, `{"error":"failed loading license"}`, http.StatusInternalServerError)
		return
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

	lic, err := c.repo.GetActiveLicense(r.Context(), wsID)
	totalSeats := 10
	allocatedSeats := 1
	if err == nil && lic != nil {
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
