// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package product

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/scandrix/backend/internal/integrations/zoho"
)

// TelemetryService provides the unified entry point for all customer lifecycle and product analytics events.
// Every event is isolated via safeCall so network failures or analytics downtime never break core business logic.
type TelemetryService struct {
	posthog PostHogClient
	resend  ResendClient
	n8n     N8nClient
	crm     CRMClient
	logger  *slog.Logger
}

// NewTelemetryService constructs an initialized product TelemetryService.
func NewTelemetryService(posthog PostHogClient, resend ResendClient, n8n N8nClient, logger *slog.Logger) *TelemetryService {
	if logger == nil {
		logger = slog.Default()
	}
	if posthog == nil {
		posthog = NewPostHogClient()
	}
	if resend == nil {
		resend = NewResendClient()
	}
	if n8n == nil {
		n8n = NewN8nClient()
	}

	return &TelemetryService{
		posthog: posthog,
		resend:  resend,
		n8n:     n8n,
		crm:     zoho.NewClientFromEnv(),
		logger:  logger,
	}
}

// WithCRM allows overriding the CRM client (e.g. for testing).
func (s *TelemetryService) WithCRM(crm CRMClient) *TelemetryService {
	s.crm = crm
	return s
}

// ─── Event Parameter Structures ──────────────────────────────────────────────

type UserSignedUpParams struct {
	UserID           string `json:"user_id"`
	Email            string `json:"email"`
	Name             string `json:"name,omitempty"`
	OrganizationID   string `json:"organization_id"`
	OrganizationName string `json:"organization_name,omitempty"`
	TeamID           string `json:"team_id,omitempty"`
	TeamName         string `json:"team_name,omitempty"`
}

type UserInvitationAcceptedParams struct {
	UserID         string `json:"user_id"`
	Email          string `json:"email"`
	Name           string `json:"name,omitempty"`
	OrganizationID string `json:"organization_id,omitempty"`
	TeamID         string `json:"team_id,omitempty"`
}

type OrganizationUpdatedParams struct {
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name,omitempty"`
	TenantName     string `json:"tenant_name,omitempty"`
}

type TeamCreatedParams struct {
	TeamID           string `json:"team_id"`
	Name             string `json:"name,omitempty"`
	OrganizationID   string `json:"organization_id,omitempty"`
	OrganizationName string `json:"organization_name,omitempty"`
	ActorUserID      string `json:"actor_user_id,omitempty"`
}

type RepositoryConnectedParams struct {
	RepositoryID       string `json:"repository_id"`
	Name               string `json:"name"`
	FullName           string `json:"full_name"`
	Platform           string `json:"platform"`
	OrganizationID     string `json:"organization_id"`
	AgentReviewEnabled bool   `json:"agent_review_enabled"`
	ActorUserID        string `json:"actor_user_id,omitempty"`
}

type ByokConfiguredParams struct {
	UserID         string `json:"user_id"`
	OrganizationID string `json:"organization_id"`
	Provider       string `json:"provider,omitempty"`
	Slot           string `json:"slot,omitempty"` // "main" | "fallback"
}

type OnboardingCompletedParams struct {
	UserID           string `json:"user_id"`
	Email            string `json:"email,omitempty"`
	OrganizationID   string `json:"organization_id"`
	OrganizationName string `json:"organization_name,omitempty"`
	TeamID           string `json:"team_id"`
	TeamName         string `json:"team_name,omitempty"`
	ReviewedPR       bool   `json:"reviewed_pr"`
	OrgMemberCount   int    `json:"org_member_count,omitempty"`
}

type OnboardingReviewTriggeredParams struct {
	UserID         string `json:"user_id"`
	Email          string `json:"email,omitempty"`
	TeamID         string `json:"team_id"`
	OrganizationID string `json:"organization_id,omitempty"`
	RepositoryID   string `json:"repository_id,omitempty"`
}

type OnboardingReviewSkippedParams struct {
	UserID         string `json:"user_id"`
	Email          string `json:"email,omitempty"`
	TeamID         string `json:"team_id"`
	OrganizationID string `json:"organization_id,omitempty"`
}

type FirstReviewCompletedParams struct {
	OrganizationID    string `json:"organization_id"`
	OrganizationName  string `json:"organization_name,omitempty"`
	TeamID            string `json:"team_id,omitempty"`
	RepositoryID      string `json:"repository_id,omitempty"`
	RepositoryName    string `json:"repository_name,omitempty"`
	PullRequestNumber int    `json:"pull_request_number,omitempty"`
	Platform          string `json:"platform,omitempty"`
	OwnerID           string `json:"owner_id,omitempty"`
	OwnerEmail        string `json:"owner_email,omitempty"`
	OrgMemberCount    int    `json:"org_member_count,omitempty"`
}

// ─── Lifecycle Event Handlers ────────────────────────────────────────────────

func (s *TelemetryService) UserSignedUp(ctx context.Context, p UserSignedUpParams) {
	s.safeCall("UserSignedUp", func() error {
		_ = s.posthog.Identify(ctx, p.UserID, map[string]any{
			"email":            p.Email,
			"name":             p.Name,
			"organizationId":   p.OrganizationID,
			"organizationName": p.OrganizationName,
		})

		_ = s.posthog.GroupIdentify(ctx, "organization", p.OrganizationID, map[string]any{
			"id":   p.OrganizationID,
			"name": p.OrganizationName,
		})

		if p.TeamID != "" {
			_ = s.posthog.GroupIdentify(ctx, "team", p.TeamID, map[string]any{
				"id":               p.TeamID,
				"name":             p.TeamName,
				"organizationId":   p.OrganizationID,
				"organizationName": p.OrganizationName,
			})
		}

		_ = s.posthog.Capture(ctx, p.UserID, "user_signed_up", map[string]any{
			"email":            p.Email,
			"name":             p.Name,
			"organizationId":   p.OrganizationID,
			"organizationName": p.OrganizationName,
			"teamId":           p.TeamID,
		}, map[string]string{
			"organization": p.OrganizationID,
			"team":         p.TeamID,
		})

		if s.crm != nil && s.crm.IsEnabled() {
			_, _ = s.crm.UpsertLead(ctx, zoho.Lead{
				Email:       p.Email,
				LastName:    p.Name,
				Company:     p.OrganizationName,
				LeadStatus:  "New",
				Description: fmt.Sprintf("User signed up. Org: %s, Team: %s", p.OrganizationName, p.TeamName),
			})
		}

		_ = s.resend.Send(ctx, "user.signed_up", p.Email, map[string]any{
			"userId":           p.UserID,
			"name":             p.Name,
			"organizationName": p.OrganizationName,
		})

		return s.n8n.Notify(ctx, "user.signed_up", map[string]any{
			"userId":           p.UserID,
			"email":            p.Email,
			"name":             p.Name,
			"organizationId":   p.OrganizationID,
			"organizationName": p.OrganizationName,
			"teamId":           p.TeamID,
			"teamName":         p.TeamName,
		})
	})
}

func (s *TelemetryService) UserInvitationAccepted(ctx context.Context, p UserInvitationAcceptedParams) {
	s.safeCall("UserInvitationAccepted", func() error {
		_ = s.posthog.Identify(ctx, p.UserID, map[string]any{
			"email":          p.Email,
			"name":           p.Name,
			"organizationId": p.OrganizationID,
		})

		_ = s.posthog.Capture(ctx, p.UserID, "user_invitation_accepted", map[string]any{
			"email":          p.Email,
			"name":           p.Name,
			"organizationId": p.OrganizationID,
			"teamId":         p.TeamID,
		}, map[string]string{
			"organization": p.OrganizationID,
			"team":         p.TeamID,
		})

		return s.resend.Send(ctx, "user.invitation_accepted", p.Email, map[string]any{
			"userId": p.UserID,
			"name":   p.Name,
		})
	})
}

func (s *TelemetryService) OrganizationUpdated(ctx context.Context, p OrganizationUpdatedParams) {
	s.safeCall("OrganizationUpdated", func() error {
		return s.posthog.GroupIdentify(ctx, "organization", p.OrganizationID, map[string]any{
			"id":         p.OrganizationID,
			"name":       p.Name,
			"tenantName": p.TenantName,
		})
	})
}

func (s *TelemetryService) TeamCreated(ctx context.Context, p TeamCreatedParams) {
	s.safeCall("TeamCreated", func() error {
		_ = s.posthog.GroupIdentify(ctx, "team", p.TeamID, map[string]any{
			"id":               p.TeamID,
			"name":             p.Name,
			"organizationId":   p.OrganizationID,
			"organizationName": p.OrganizationName,
		})

		if p.ActorUserID != "" {
			return s.posthog.Capture(ctx, p.ActorUserID, "team_created", map[string]any{
				"teamId":         p.TeamID,
				"name":           p.Name,
				"organizationId": p.OrganizationID,
			}, map[string]string{
				"organization": p.OrganizationID,
				"team":         p.TeamID,
			})
		}
		return nil
	})
}

func (s *TelemetryService) RepositoryConnected(ctx context.Context, p RepositoryConnectedParams) {
	s.safeCall("RepositoryConnected", func() error {
		_ = s.posthog.GroupIdentify(ctx, "repository", p.RepositoryID, map[string]any{
			"repositoryId":       p.RepositoryID,
			"name":               p.Name,
			"fullName":           p.FullName,
			"platform":           p.Platform,
			"organizationId":     p.OrganizationID,
			"agentReviewEnabled": p.AgentReviewEnabled,
		})

		if p.ActorUserID != "" {
			return s.posthog.Capture(ctx, p.ActorUserID, "repository_connected", map[string]any{
				"repositoryId":   p.RepositoryID,
				"fullName":       p.FullName,
				"platform":       p.Platform,
				"organizationId": p.OrganizationID,
			}, map[string]string{
				"organization": p.OrganizationID,
				"repository":   p.RepositoryID,
			})
		}
		return nil
	})
}

func (s *TelemetryService) ByokConfigured(ctx context.Context, p ByokConfiguredParams) {
	s.safeCall("ByokConfigured", func() error {
		return s.posthog.Capture(ctx, p.UserID, "byok_configured", map[string]any{
			"organizationId": p.OrganizationID,
			"provider":       p.Provider,
			"slot":           p.Slot,
		}, map[string]string{
			"organization": p.OrganizationID,
		})
	})
}

func (s *TelemetryService) OnboardingCompleted(ctx context.Context, p OnboardingCompletedParams) {
	s.safeCall("OnboardingCompleted", func() error {
		_ = s.posthog.Capture(ctx, p.UserID, "onboarding_completed", map[string]any{
			"organizationId":   p.OrganizationID,
			"organizationName": p.OrganizationName,
			"teamId":           p.TeamID,
			"teamName":         p.TeamName,
			"reviewedPR":       p.ReviewedPR,
			"orgMemberCount":   p.OrgMemberCount,
		}, map[string]string{
			"organization": p.OrganizationID,
			"team":         p.TeamID,
		})

		if s.crm != nil && s.crm.IsEnabled() && p.Email != "" {
			_, _ = s.crm.UpsertLead(ctx, zoho.Lead{
				Email:       p.Email,
				Company:     p.OrganizationName,
				LeadStatus:  "Trial Active",
				Description: fmt.Sprintf("Onboarding completed. Org: %s, Reviewed PR: %t", p.OrganizationName, p.ReviewedPR),
			})
		}

		if p.Email != "" {
			_ = s.resend.Send(ctx, "onboarding.completed", p.Email, map[string]any{
				"userId":           p.UserID,
				"organizationName": p.OrganizationName,
				"reviewedPR":       p.ReviewedPR,
			})
		}

		return s.n8n.Notify(ctx, "onboarding.completed", map[string]any{
			"userId":           p.UserID,
			"email":            p.Email,
			"organizationId":   p.OrganizationID,
			"organizationName": p.OrganizationName,
			"teamId":           p.TeamID,
			"teamName":         p.TeamName,
			"reviewedPR":       p.ReviewedPR,
			"orgMemberCount":   p.OrgMemberCount,
		})
	})
}

func (s *TelemetryService) OnboardingReviewTriggered(ctx context.Context, p OnboardingReviewTriggeredParams) {
	s.safeCall("OnboardingReviewTriggered", func() error {
		_ = s.posthog.Capture(ctx, p.UserID, "onboarding_review_triggered", map[string]any{
			"teamId":         p.TeamID,
			"organizationId": p.OrganizationID,
			"repositoryId":   p.RepositoryID,
		}, map[string]string{
			"organization": p.OrganizationID,
			"team":         p.TeamID,
			"repository":   p.RepositoryID,
		})

		if p.Email != "" {
			return s.resend.Send(ctx, "onboarding.review_triggered", p.Email, map[string]any{
				"userId":       p.UserID,
				"repositoryId": p.RepositoryID,
			})
		}
		return nil
	})
}

func (s *TelemetryService) OnboardingReviewSkipped(ctx context.Context, p OnboardingReviewSkippedParams) {
	s.safeCall("OnboardingReviewSkipped", func() error {
		_ = s.posthog.Capture(ctx, p.UserID, "onboarding_review_skipped", map[string]any{
			"teamId":         p.TeamID,
			"organizationId": p.OrganizationID,
		}, map[string]string{
			"organization": p.OrganizationID,
			"team":         p.TeamID,
		})

		if p.Email != "" {
			return s.resend.Send(ctx, "onboarding.review_skipped", p.Email, map[string]any{
				"userId": p.UserID,
			})
		}
		return nil
	})
}

func (s *TelemetryService) FirstReviewCompleted(ctx context.Context, p FirstReviewCompletedParams) {
	s.safeCall("FirstReviewCompleted", func() error {
		actorID := p.OwnerID
		if actorID == "" {
			actorID = p.OrganizationID
		}

		_ = s.posthog.Capture(ctx, actorID, "first_review_completed", map[string]any{
			"organizationId":    p.OrganizationID,
			"organizationName":  p.OrganizationName,
			"teamId":            p.TeamID,
			"repositoryId":      p.RepositoryID,
			"repositoryName":    p.RepositoryName,
			"pullRequestNumber": p.PullRequestNumber,
			"platform":          p.Platform,
			"ownerEmail":        p.OwnerEmail,
			"orgMemberCount":    p.OrgMemberCount,
		}, map[string]string{
			"organization": p.OrganizationID,
			"team":         p.TeamID,
			"repository":   p.RepositoryID,
		})

		return s.n8n.Notify(ctx, "first_review.completed", map[string]any{
			"organizationId":    p.OrganizationID,
			"organizationName":  p.OrganizationName,
			"teamId":            p.TeamID,
			"repositoryId":      p.RepositoryID,
			"repositoryName":    p.RepositoryName,
			"pullRequestNumber": p.PullRequestNumber,
			"platform":          p.Platform,
			"ownerEmail":        p.OwnerEmail,
			"ownerId":           p.OwnerID,
			"orgMemberCount":    p.OrgMemberCount,
		})
	})
}

// safeCall provides a fail-safe barrier: catches and logs provider errors without breaking the host flow.
func (s *TelemetryService) safeCall(label string, fn func() error) {
	if fn == nil {
		return
	}
	if err := fn(); err != nil {
		s.logger.Warn("Product telemetry call failed (swallowed)",
			"label", label,
			"error", err.Error(),
		)
	}
}
