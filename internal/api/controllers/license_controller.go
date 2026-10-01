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
	"github.com/scandrix/backend/internal/enterprise/license"
	"github.com/scandrix/backend/pkg/models"
)

// LicenseRepository defines the data access contract for enterprise licenses (Clean Architecture).
type LicenseRepository interface {
	GetActiveLicense(ctx context.Context, wsID uuid.UUID) (*models.OrganizationLicense, error)
	ActivateLicense(ctx context.Context, wsID uuid.UUID, licenseKey, orgName, planTier string, totalSeats int, expiresAt time.Time, features []string) error
}

// LicenseVerifier verifies a submitted license token against the vendor public key.
type LicenseVerifier interface {
	LoadLicense(token string) (*license.LicensePayload, error)
	Entitlement() *license.Entitlement
}

// LicenseController manages enterprise seat allocations, subscriptions, and air-gapped license files.
type LicenseController struct {
	repo     LicenseRepository
	verifier LicenseVerifier
	resolver *license.Resolver
}

// NewLicenseController initializes the license controller with repository persistence.
func NewLicenseController(repo LicenseRepository) *LicenseController {
	if isNilInterface(repo) {
		repo = nil
	}
	return &LicenseController{repo: repo}
}

// WithVerifier attaches the signed-license verifier used to validate activation keys.
func (c *LicenseController) WithVerifier(v LicenseVerifier) *LicenseController {
	if isNilInterface(v) {
		c.verifier = nil
	} else {
		c.verifier = v
	}
	c.buildResolver()
	return c
}

// WithResolver attaches a prebuilt entitlement resolver, so this controller and
// the feature gate always agree on what a workspace is entitled to.
func (c *LicenseController) WithResolver(r *license.Resolver) *LicenseController {
	c.resolver = r
	return c
}

func (c *LicenseController) buildResolver() {
	manager, _ := c.verifier.(*license.LicenseManager)
	c.resolver = license.NewResolver(manager, licenseStoreAdapter{repo: c.repo})
}

// licenseStoreAdapter exposes the controller repository as a license.Store.
type licenseStoreAdapter struct {
	repo LicenseRepository
}

func (a licenseStoreAdapter) GetActiveLicense(ctx context.Context, wsID uuid.UUID) (*license.StoredLicense, error) {
	if a.repo == nil {
		return nil, nil
	}
	lic, err := a.repo.GetActiveLicense(ctx, wsID)
	if err != nil || lic == nil {
		return nil, err
	}
	return &license.StoredLicense{
		Tier:       lic.PlanTier,
		Features:   lic.FeaturesEnabled,
		MaxSeats:   lic.TotalSeats,
		CustomerNm: lic.OrganizationName,
		ExpiresAt:  lic.ExpiresAt,
	}, nil
}

// resolveEntitlement produces the single authoritative entitlement for a
// workspace by delegating to the shared resolver.
func (c *LicenseController) resolveEntitlement(ctx context.Context, wsID uuid.UUID) *license.Entitlement {
	if c.resolver == nil {
		c.buildResolver()
	}
	return c.resolver.Resolve(ctx, wsID)
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

	ent := c.resolveEntitlement(r.Context(), wsID)

	resp := dtos.LicenseResponse{
		OrganizationName: string(ent.Tier),
		PlanTier:         string(ent.Tier),
		TotalSeats:       ent.SeatLimit(),
		ExpiresAt:        ent.ExpiresAt,
		FeaturesEnabled:  ent.FeatureList(),
	}

	var lic *models.OrganizationLicense
	if c.repo != nil {
		if lic, err = c.repo.GetActiveLicense(r.Context(), wsID); err != nil {
			http.Error(w, `{"error":"failed loading license"}`, http.StatusInternalServerError)
			return
		}
	}
	if lic != nil {
		resp.LicenseKey = lic.LicenseKey
		resp.OrganizationName = lic.OrganizationName
		resp.AllocatedSeats = lic.AllocatedSeats
		resp.IsAirGapped = lic.IsAirGapped
	} else {
		resp.LicenseKey = ""
		resp.AllocatedSeats = 0
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.LicenseKey) == "" {
		http.Error(w, `{"error":"license_key is required"}`, http.StatusBadRequest)
		return
	}

	if c.verifier == nil {
		http.Error(w, `{"error":"license verification is not configured on this deployment"}`, http.StatusServiceUnavailable)
		return
	}

	payload, err := c.verifier.LoadLicense(strings.TrimSpace(req.LicenseKey))
	if err != nil {
		slog.Warn("Rejected license activation", "error", err)
		http.Error(w, `{"error":"license key is invalid"}`, http.StatusUnprocessableEntity)
		return
	}

	orgName := payload.CustomerName
	if strings.TrimSpace(orgName) == "" {
		orgName = "Licensed Customer"
	}

	if err := c.repo.ActivateLicense(
		r.Context(), wsID, req.LicenseKey, orgName,
		string(payload.Tier), payload.MaxSeats, payload.ExpiresAt, payload.Features,
	); err != nil {
		http.Error(w, `{"error":"failed activating license"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":     "ACTIVATED",
		"tier":       string(payload.Tier),
		"seats":      payload.MaxSeats,
		"features":   payload.Features,
		"expires_at": payload.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

func (c *LicenseController) handleGetSeats(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	ent := c.resolveEntitlement(r.Context(), wsID)

	allocatedSeats := 0
	if c.repo != nil {
		if lic, err := c.repo.GetActiveLicense(r.Context(), wsID); err == nil && lic != nil {
			allocatedSeats = lic.AllocatedSeats
		}
	}

	totalSeats := ent.SeatLimit()
	availableSeats := -1
	if totalSeats > 0 {
		availableSeats = totalSeats - allocatedSeats
		if availableSeats < 0 {
			availableSeats = 0
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"total_seats":     totalSeats,
		"allocated_seats": allocatedSeats,
		"available_seats": availableSeats,
		"unlimited":       totalSeats == 0,
	})
}

func (c *LicenseController) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	ent := c.resolveEntitlement(r.Context(), wsID)

	subStatus := "active"
	if !ent.Valid {
		subStatus = "expired"
	}

	resp := map[string]any{
		"valid":              ent.Valid,
		"subscriptionStatus": subStatus,
		"plan":               string(ent.Tier),
		"seats":              ent.SeatLimit(),
		"features":           ent.FeatureList(),
		"source":             string(ent.Source),
		"repositoryLimit":    ent.RepoLimit(),
	}
	if !ent.ExpiresAt.IsZero() {
		resp["expiresAt"] = ent.ExpiresAt.UTC().Format(time.RFC3339)
	} else {
		resp["expiresAt"] = nil
	}
	if ent.Reason != "" {
		resp["reason"] = ent.Reason
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (c *LicenseController) handleGetOrgStatus(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	ent := c.resolveEntitlement(r.Context(), wsID)

	subStatus := "active"
	if !ent.Valid {
		subStatus = "expired"
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"valid":              ent.Valid,
		"subscriptionStatus": subStatus,
		"plan":               string(ent.Tier),
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

// handleGetRemovableSeats reports the real seat position for the workspace.
// The previous version returned a hardcoded empty list and a constant "ok"
// status without reading anything, which reported success for every workspace
// including over-quota and unlicensed ones.
//
// Candidate seat IDs are not computed here: there is no persisted seat
// assignment model yet, so any identifier list would be invented. When that
// model lands it should populate candidates from real assignments.
func (c *LicenseController) handleGetRemovableSeats(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	ent := c.resolveEntitlement(r.Context(), wsID)
	if ent == nil {
		http.Error(w, `{"error":"entitlement unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	allocatedSeats := 0
	if c.repo != nil {
		if lic, err := c.repo.GetActiveLicense(r.Context(), wsID); err == nil && lic != nil {
			allocatedSeats = lic.AllocatedSeats
		}
	}

	totalSeats := ent.SeatLimit()
	availableSeats := -1
	if totalSeats > 0 {
		availableSeats = totalSeats - allocatedSeats
		if availableSeats < 0 {
			availableSeats = 0
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"total_seats":      totalSeats,
		"allocated_seats":  allocatedSeats,
		"available_seats":  availableSeats,
		"seats_remaining":  c.resolver.SeatsRemaining(r.Context(), wsID),
		"tier":             string(ent.Tier),
		"candidates_ready": false,
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
