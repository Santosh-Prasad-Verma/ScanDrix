// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package controllers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/enterprise/license"
)

// EntitlementResolver resolves the entitlement for a workspace.
type EntitlementResolver interface {
	BuildCapabilities(ctx context.Context, wsID uuid.UUID) *license.Capabilities
}

// CapabilitiesController serves the single entitlement payload a client needs
// to render every gate, lock and upgrade prompt.
type CapabilitiesController struct {
	resolver EntitlementResolver
}

// NewCapabilitiesController initializes the capabilities controller.
func NewCapabilitiesController(resolver EntitlementResolver) *CapabilitiesController {
	if isNilInterface(resolver) {
		resolver = nil
	}
	return &CapabilitiesController{resolver: resolver}
}

// Routes mounts the capabilities endpoint.
func (c *CapabilitiesController) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", c.handleGetCapabilities)
	return r
}

// handleGetCapabilities returns the resolved capabilities for the caller's
// workspace. It is readable by any authenticated member: gating happens on the
// protected routes themselves, so this endpoint only reports entitlement.
//
// When entitlement cannot be resolved the response is the Community plan rather
// than an error, so a client can still render a coherent locked state. A client
// that cannot reach a feature for any other reason gets a 403 from that route.
func (c *CapabilitiesController) handleGetCapabilities(w http.ResponseWriter, r *http.Request) {
	wsID, err := auth.WorkspaceFromContext(r.Context())
	if err != nil {
		http.Error(w, `{"error":"missing workspace context"}`, http.StatusUnauthorized)
		return
	}

	if c.resolver == nil {
		writeCapabilitiesJSON(w, license.NewCommunityCapabilities())
		return
	}

	caps := c.resolver.BuildCapabilities(r.Context(), wsID)
	if caps == nil {
		writeCapabilitiesJSON(w, license.NewCommunityCapabilities())
		return
	}

	writeCapabilitiesJSON(w, caps)
}

func writeCapabilitiesJSON(w http.ResponseWriter, payload *license.Capabilities) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(payload)
}
