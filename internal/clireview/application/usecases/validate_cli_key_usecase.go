package usecases

import (
	"context"
	"errors"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/scandrix/backend/internal/clireview/domain"
)

// ITeamCliKeyService defines contract for validating team API keys.
type ITeamCliKeyService interface {
	ValidateKey(ctx context.Context, key string) (*TeamCliKeyData, error)
}

// TeamCliKeyData holds team and organization entities linked to an API key.
type TeamCliKeyData struct {
	Team         *TeamEntityRef         `json:"team,omitempty"`
	Organization *OrganizationEntityRef `json:"organization,omitempty"`
}

// TeamEntityRef represents an organizational team.
type TeamEntityRef struct {
	UUID         string                 `json:"uuid"`
	Name         string                 `json:"name"`
	Organization *OrganizationEntityRef `json:"organization,omitempty"`
}

// OrganizationEntityRef represents an enterprise organization.
type OrganizationEntityRef struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

// ITeamService provides team lookup capabilities.
type ITeamService interface {
	FindByID(ctx context.Context, teamID string) (*TeamEntityRef, error)
	FindFirstCreatedTeam(ctx context.Context, organizationID string) (*TeamEntityRef, error)
}

// IAuthService validates active user accounts.
type IAuthService interface {
	ValidateUser(ctx context.Context, email string) (*AuthUserRecord, error)
}

// AuthUserRecord represents an authenticated system user.
type AuthUserRecord struct {
	UUID   string `json:"uuid"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	Status string `json:"status"` // "ACTIVE", "INACTIVE", "REMOVED"
}

// ICliDeviceService manages terminal device validation and token registration.
type ICliDeviceService interface {
	ValidateOrRegisterDevice(ctx context.Context, input DeviceRegistrationInput) (*DeviceRegistrationResult, error)
}

// DeviceRegistrationInput parameterizes device checks.
type DeviceRegistrationInput struct {
	DeviceID       string `json:"deviceId"`
	DeviceToken    string `json:"deviceToken,omitempty"`
	OrganizationID string `json:"organizationId"`
	UserAgent      string `json:"userAgent,omitempty"`
}

// DeviceRegistrationResult contains minted or validated device token.
type DeviceRegistrationResult struct {
	DeviceToken string `json:"deviceToken"`
	Valid       bool   `json:"valid"`
	ErrorCode   string `json:"errorCode,omitempty"`
	ErrorMsg    string `json:"errorMsg,omitempty"`
}

// ValidateCliKeyUseCase encapsulates team API key and JWT authentication with device tracking.
type ValidateCliKeyUseCase struct {
	teamCliKeyService ITeamCliKeyService
	teamService       ITeamService
	authService       IAuthService
	cliDeviceService  ICliDeviceService
	jwtSecret         string
}

// NewValidateCliKeyUseCase creates an initialized ValidateCliKeyUseCase.
func NewValidateCliKeyUseCase(
	teamCliKeyService ITeamCliKeyService,
	teamService ITeamService,
	authService IAuthService,
	cliDeviceService ICliDeviceService,
	jwtSecret string,
) *ValidateCliKeyUseCase {
	return &ValidateCliKeyUseCase{
		teamCliKeyService: teamCliKeyService,
		teamService:       teamService,
		authService:       authService,
		cliDeviceService:  cliDeviceService,
		jwtSecret:         jwtSecret,
	}
}

// Execute validates credentials and attaches device tracking telemetry.
func (uc *ValidateCliKeyUseCase) Execute(ctx context.Context, input domain.ValidateCliKeyInput) (*domain.ValidateCliKeyResult, error) {
	payload := uc.validateAuth(ctx, input)

	shouldTrackDevice := input.DeviceID != "" && payload.Valid && payload.OrganizationID != nil && *payload.OrganizationID != ""
	if !shouldTrackDevice || uc.cliDeviceService == nil {
		return payload, nil
	}

	deviceResult, err := uc.cliDeviceService.ValidateOrRegisterDevice(ctx, DeviceRegistrationInput{
		DeviceID:       input.DeviceID,
		DeviceToken:    input.DeviceToken,
		OrganizationID: *payload.OrganizationID,
		UserAgent:      input.UserAgent,
	})

	if err != nil || (deviceResult != nil && !deviceResult.Valid) {
		errCode := ""
		errMsg := "Device rejected"
		if err != nil {
			errMsg = err.Error()
		} else if deviceResult != nil && deviceResult.ErrorMsg != "" {
			errMsg = deviceResult.ErrorMsg
			errCode = deviceResult.ErrorCode
		}

		payload.Valid = false
		payload.Error = errMsg
		payload.Code = errCode
		return payload, nil
	}

	if deviceResult != nil && deviceResult.DeviceToken != "" {
		payload.DeviceToken = deviceResult.DeviceToken
		if payload.Data != nil {
			payload.Data["deviceToken"] = deviceResult.DeviceToken
		}
	}

	return payload, nil
}

func (uc *ValidateCliKeyUseCase) validateAuth(ctx context.Context, input domain.ValidateCliKeyInput) *domain.ValidateCliKeyResult {
	teamKey := strings.TrimSpace(input.TeamKey)
	authHeader := strings.TrimSpace(input.AuthHeader)

	var bearerToken string
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		bearerToken = strings.TrimSpace(authHeader[7:])
	}

	buildInvalid := func(msg string) *domain.ValidateCliKeyResult {
		res := &domain.ValidateCliKeyResult{
			Valid: false,
			Error: msg,
			Team: &domain.ValidateCliKeyEntity{
				ID:   nil,
				Name: "",
			},
			Organization: &domain.ValidateCliKeyEntity{
				ID:   nil,
				Name: "",
			},
			User: &domain.ValidateCliKeyUser{
				Email: "",
				Name:  "",
			},
			Data: make(map[string]any),
		}
		res.Data["valid"] = false
		res.Data["error"] = msg
		return res
	}

	// Route 1: Team API key (via X-Team-Key or Bearer with scandrix_ prefix)
	if teamKey != "" || strings.HasPrefix(bearerToken, "scandrix_") {
		key := teamKey
		if key == "" {
			key = bearerToken
		}

		if key == "" {
			return buildInvalid("Team API key required. Provide via X-Team-Key or Authorization: Bearer header.")
		}

		if uc.teamCliKeyService == nil {
			return buildInvalid("Team CLI key service unavailable")
		}

		teamData, err := uc.teamCliKeyService.ValidateKey(ctx, key)
		if err != nil || teamData == nil {
			return buildInvalid("Invalid or revoked team API key")
		}

		team := teamData.Team
		org := teamData.Organization
		var teamID, teamName, orgID, orgName string

		if team != nil {
			teamID = team.UUID
			teamName = team.Name
		}
		if org != nil {
			orgID = org.UUID
			orgName = org.Name
		}

		isValid := teamID != "" && orgID != ""
		res := &domain.ValidateCliKeyResult{
			Valid:            isValid,
			TeamID:           toStrPtr(teamID),
			OrganizationID:   toStrPtr(orgID),
			TeamName:         teamName,
			OrganizationName: orgName,
			Team: &domain.ValidateCliKeyEntity{
				ID:   toStrPtr(teamID),
				Name: teamName,
			},
			Organization: &domain.ValidateCliKeyEntity{
				ID:   toStrPtr(orgID),
				Name: orgName,
			},
			User: &domain.ValidateCliKeyUser{
				Email: "",
				Name:  "",
			},
			Data: make(map[string]any),
		}

		if !isValid {
			res.Error = "Invalid or incomplete team API key"
		}

		res.Data["valid"] = res.Valid
		res.Data["teamId"] = teamID
		res.Data["organizationId"] = orgID
		res.Data["teamName"] = res.TeamName
		res.Data["organizationName"] = res.OrganizationName
		return res
	}

	// Route 2: JWT Bearer token
	if bearerToken != "" {
		if uc.jwtSecret == "" {
			return buildInvalid("JWT authentication unconfigured")
		}

		parsedToken, err := jwt.Parse(bearerToken, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(uc.jwtSecret), nil
		})

		if err != nil || !parsedToken.Valid {
			return buildInvalid("Invalid or expired JWT token")
		}

		claims, ok := parsedToken.Claims.(jwt.MapClaims)
		if !ok {
			return buildInvalid("Invalid JWT token claims")
		}

		email, _ := claims["email"].(string)
		role, _ := claims["role"].(string)
		status, _ := claims["status"].(string)
		tokenOrgID, _ := claims["organizationId"].(string)

		if uc.authService == nil {
			return buildInvalid("Auth service unavailable")
		}

		userRecord, err := uc.authService.ValidateUser(ctx, email)
		if err != nil || userRecord == nil {
			return buildInvalid("User account is inactive or removed")
		}

		if (role != "" && userRecord.Role != role) ||
			(status != "" && userRecord.Status != status) ||
			userRecord.Status == "REMOVED" {
			return buildInvalid("User account is inactive or removed")
		}

		if uc.teamService == nil {
			return buildInvalid("Team service unavailable")
		}

		var team *TeamEntityRef
		queryTeamID := input.QueryTeamID
		if queryTeamID != "" {
			t, err := uc.teamService.FindByID(ctx, queryTeamID)
			if err == nil {
				team = t
			}
		}

		if team == nil && queryTeamID != "" && queryTeamID != tokenOrgID {
			return buildInvalid("Team not found for the provided teamId: " + queryTeamID)
		}

		if team == nil {
			t, err := uc.teamService.FindFirstCreatedTeam(ctx, tokenOrgID)
			if err == nil {
				team = t
			}
		}

		if team == nil {
			return buildInvalid("No active team found for the authenticated user")
		}

		if team.Organization != nil && team.Organization.UUID != "" && team.Organization.UUID != tokenOrgID {
			return buildInvalid("Team does not belong to the authenticated organization")
		}

		teamName := team.Name
		orgName := ""
		if team.Organization != nil {
			orgName = team.Organization.Name
		}

		res := &domain.ValidateCliKeyResult{
			Valid:            true,
			TeamID:           toStrPtr(team.UUID),
			OrganizationID:   toStrPtr(tokenOrgID),
			TeamName:         teamName,
			OrganizationName: orgName,
			Team: &domain.ValidateCliKeyEntity{
				ID:   toStrPtr(team.UUID),
				Name: teamName,
			},
			Organization: &domain.ValidateCliKeyEntity{
				ID:   toStrPtr(tokenOrgID),
				Name: orgName,
			},
			User: &domain.ValidateCliKeyUser{
				Email: email,
				Name:  userRecord.Name,
			},
			UserEmail: email,
			Data:      make(map[string]any),
		}

		res.Data["valid"] = true
		res.Data["teamId"] = team.UUID
		res.Data["organizationId"] = tokenOrgID
		res.Data["teamName"] = res.TeamName
		res.Data["organizationName"] = res.OrganizationName
		res.Data["userEmail"] = email
		return res
	}

	return buildInvalid("Authentication required. Provide a team API key via X-Team-Key header, or a JWT via Authorization: Bearer header.")
}

func toStrPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
