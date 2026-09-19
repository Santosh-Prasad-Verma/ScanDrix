package usecases

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/scandrix/backend/internal/platform/application/services"
	"github.com/scandrix/backend/internal/platform/domain/contracts"
	"github.com/scandrix/backend/internal/platform/domain/types"
)

// NormalizedUser provides a uniform user model across GitHub, GitLab, Bitbucket, Azure, and Forgejo.
type NormalizedUser struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatarUrl"`
	Source    string `json:"source,omitempty"` // "id", "username", "emailOrName", "member"
}

// ═══════════════════════════════════════════════════════════════
// 1. GetCodeManagementMemberListUseCase
// ═══════════════════════════════════════════════════════════════

type GetCodeManagementMemberListUseCase struct {
	memberListService *services.OrganizationMemberListService
}

func NewGetCodeManagementMemberListUseCase(
	memberListService *services.OrganizationMemberListService,
) *GetCodeManagementMemberListUseCase {
	return &GetCodeManagementMemberListUseCase{
		memberListService: memberListService,
	}
}

func (uc *GetCodeManagementMemberListUseCase) Execute(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	skipCache bool,
) (*services.OrganizationMemberListResult, error) {
	return uc.memberListService.Fetch(ctx, orgData, skipCache)
}

func (uc *GetCodeManagementMemberListUseCase) Refresh(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
) (*services.OrganizationMemberListResult, error) {
	return uc.memberListService.RefreshMembers(ctx, orgData)
}

// ═══════════════════════════════════════════════════════════════
// 2. GetCurrentCodeManagementUserUseCase
// ═══════════════════════════════════════════════════════════════

type GetCurrentCodeManagementUserUseCase struct {
	codeManagement contracts.ICodeManagementService
}

func NewGetCurrentCodeManagementUserUseCase(
	codeManagement contracts.ICodeManagementService,
) *GetCurrentCodeManagementUserUseCase {
	return &GetCurrentCodeManagementUserUseCase{
		codeManagement: codeManagement,
	}
}

func (uc *GetCurrentCodeManagementUserUseCase) Execute(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
) (*NormalizedUser, error) {
	if strings.TrimSpace(orgData.OrganizationID) == "" {
		return nil, fmt.Errorf("organizationId is required")
	}

	user, err := uc.codeManagement.GetCurrentUser(ctx, orgData)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to retrieve current code management user",
			"organizationId", orgData.OrganizationID,
			"teamId", orgData.TeamID,
			"error", err,
		)
		return nil, err
	}

	if user == nil {
		return nil, nil
	}

	return &NormalizedUser{
		ID:        user.ID,
		Name:      user.Name,
		Username:  user.Username,
		Email:     user.Email,
		AvatarURL: user.AvatarURL,
		Source:    "current",
	}, nil
}

// ═══════════════════════════════════════════════════════════════
// 3. SearchCodeManagementUsersUseCase
// ═══════════════════════════════════════════════════════════════

type SearchUsersParams struct {
	OrganizationID string
	TeamID         string
	Query          string
	UserID         string
	Limit          int
}

type SearchCodeManagementUsersUseCase struct {
	codeManagement    contracts.ICodeManagementService
	memberListService *services.OrganizationMemberListService
}

func NewSearchCodeManagementUsersUseCase(
	codeManagement contracts.ICodeManagementService,
	memberListService *services.OrganizationMemberListService,
) *SearchCodeManagementUsersUseCase {
	return &SearchCodeManagementUsersUseCase{
		codeManagement:    codeManagement,
		memberListService: memberListService,
	}
}

func (uc *SearchCodeManagementUsersUseCase) Execute(
	ctx context.Context,
	params SearchUsersParams,
) ([]NormalizedUser, error) {
	if strings.TrimSpace(params.OrganizationID) == "" {
		return nil, fmt.Errorf("organizationId is required")
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}

	orgData := types.OrganizationAndTeamData{
		OrganizationID: params.OrganizationID,
		TeamID:         params.TeamID,
	}

	var results []NormalizedUser
	seen := make(map[string]bool)

	addUser := func(user *types.PullRequestUser, source string) {
		if user == nil {
			return
		}
		id := strings.TrimSpace(user.ID)
		if id == "" {
			id = strings.TrimSpace(user.Username)
		}
		if id == "" || seen[id] {
			return
		}

		seen[id] = true
		name := user.Name
		if name == "" {
			name = user.Username
		}

		results = append(results, NormalizedUser{
			ID:        id,
			Name:      name,
			Username:  user.Username,
			Email:     user.Email,
			AvatarURL: user.AvatarURL,
			Source:    source,
		})
	}

	// 1. Search by User ID
	if params.UserID != "" {
		if u, err := uc.codeManagement.GetUserByID(ctx, orgData, params.UserID); err == nil && u != nil {
			addUser(u, "id")
		}
	}

	// 2. Search by Query (Username & Email/Name)
	if params.Query != "" && len(results) < limit {
		if u, err := uc.codeManagement.GetUserByUsername(ctx, orgData, params.Query); err == nil && u != nil {
			addUser(u, "username")
		}

		if len(results) < limit {
			email := ""
			if strings.Contains(params.Query, "@") {
				email = params.Query
			}
			if u, err := uc.codeManagement.GetUserByEmailOrName(ctx, orgData, email, params.Query); err == nil && u != nil {
				addUser(u, "emailOrName")
			}
		}
	}

	// 3. Fallback to Member list filtering if fewer than limit results
	if len(results) < limit && uc.memberListService != nil && params.Query != "" {
		memberResult, err := uc.memberListService.Fetch(ctx, orgData, false)
		if err == nil && memberResult != nil && memberResult.Status == "ok" {
			qLower := strings.ToLower(params.Query)
			for _, m := range memberResult.Members {
				if len(results) >= limit {
					break
				}
				if seen[m.ID] {
					continue
				}

				if strings.Contains(strings.ToLower(m.Name), qLower) || strings.Contains(strings.ToLower(m.ID), qLower) {
					seen[m.ID] = true
					results = append(results, NormalizedUser{
						ID:     m.ID,
						Name:   m.Name,
						Source: "member",
					})
				}
			}
		}
	}

	return results, nil
}

// ═══════════════════════════════════════════════════════════════
// 4. GetWebhookStatusUseCase (translates get-webhook-status.use-case.ts)
// ═══════════════════════════════════════════════════════════════

type GetWebhookStatusUseCase struct {
	codeManagement contracts.ICodeManagementService
}

func NewGetWebhookStatusUseCase(
	codeManagement contracts.ICodeManagementService,
) *GetWebhookStatusUseCase {
	return &GetWebhookStatusUseCase{
		codeManagement: codeManagement,
	}
}

func (uc *GetWebhookStatusUseCase) Execute(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repositoryID string,
) (bool, error) {
	if strings.TrimSpace(repositoryID) == "" {
		return false, fmt.Errorf("repositoryId is required")
	}
	if strings.TrimSpace(orgData.OrganizationID) == "" || strings.TrimSpace(orgData.TeamID) == "" {
		return false, fmt.Errorf("organizationId and teamId are required")
	}

	active, err := uc.codeManagement.IsWebhookActive(ctx, orgData, repositoryID)
	if err != nil {
		slog.ErrorContext(ctx, "Error while checking webhook status",
			"organizationId", orgData.OrganizationID,
			"teamId", orgData.TeamID,
			"repositoryId", repositoryID,
			"error", err,
		)
		return false, nil
	}

	return active, nil
}
