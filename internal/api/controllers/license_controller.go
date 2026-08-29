package controllers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/internal/api/dtos"
)

// LicenseController manages enterprise seat allocations, subscriptions, and air-gapped license files.
type LicenseController struct{}

// NewLicenseController initializes the license controller.
func NewLicenseController() *LicenseController {
	return &LicenseController{}
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
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtos.LicenseResponse{
		LicenseKey:       "SCANDRIX-ENT-9988-7766-ABCD",
		OrganizationName: "Enterprise Tier Customer",
		PlanTier:         "ENTERPRISE",
		TotalSeats:       500,
		AllocatedSeats:   42,
		ExpiresAt:        time.Now().AddDate(1, 0, 0),
		IsAirGapped:      false,
		FeaturesEnabled: []string{
			"saml_sso", "scim_provisioning", "custom_rules", "audit_log_cef",
			"unlimited_repos", "priority_ai_router", "on_prem_workers",
		},
	})
}

func (c *LicenseController) handleActivateLicense(w http.ResponseWriter, r *http.Request) {
	var req dtos.ActivateLicenseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.LicenseKey == "" {
		http.Error(w, `{"error":"license_key is required"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":      "ACTIVATED",
		"license_key": req.LicenseKey,
		"valid_until": time.Now().AddDate(1, 0, 0),
	})
}

func (c *LicenseController) handleGetSeats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"total_seats":     500,
		"allocated_seats": 42,
		"available_seats": 458,
		"active_users":    38,
	})
}
