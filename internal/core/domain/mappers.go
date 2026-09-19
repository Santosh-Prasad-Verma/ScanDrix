package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Common domain validation errors.
var (
	ErrNilEntity           = errors.New("entity cannot be nil")
	ErrInvalidWorkspaceID  = errors.New("workspace ID must be a valid non-nil UUID")
	ErrTenantMismatch      = errors.New("tenant workspace ID does not match target entity workspace ID")
	ErrEmptyRequiredField  = errors.New("required field is missing or empty")
)

// ToWorkspace converts WorkspaceCreateDTO to a persistent Workspace entity.
func ToWorkspace(dto *WorkspaceCreateDTO) (*Workspace, error) {
	if dto == nil {
		return nil, ErrNilEntity
	}
	slug := strings.ToLower(strings.TrimSpace(dto.Slug))
	name := strings.TrimSpace(dto.Name)
	if slug == "" || name == "" {
		return nil, ErrEmptyRequiredField
	}
	now := time.Now().UTC()
	tier := dto.Tier
	if tier == "" {
		tier = "COMMUNITY"
	}
	meta := dto.Metadata
	if meta == nil {
		meta = make(JSONBMap)
	}

	return &Workspace{
		BaseEntity: BaseEntity{
			ID:        uuid.New(),
			CreatedAt: now,
			UpdatedAt: now,
		},
		Slug:                slug,
		Name:                name,
		Status:              "ACTIVE",
		Tier:                tier,
		SpendLimitUSD:       dto.SpendLimitUSD,
		EnforceSpendLimit:   dto.EnforceSpendLimit,
		AllowedEmailDomains: dto.AllowedEmailDomains,
		Metadata:            meta,
	}, nil
}

// ApplyWorkspaceUpdates applies non-nil DTO fields to an existing Workspace entity.
func ApplyWorkspaceUpdates(ws *Workspace, dto *WorkspaceUpdateDTO) error {
	if ws == nil || dto == nil {
		return ErrNilEntity
	}
	if dto.Name != nil && strings.TrimSpace(*dto.Name) != "" {
		ws.Name = strings.TrimSpace(*dto.Name)
	}
	if dto.Status != nil && strings.TrimSpace(*dto.Status) != "" {
		ws.Status = strings.ToUpper(strings.TrimSpace(*dto.Status))
	}
	if dto.Tier != nil && strings.TrimSpace(*dto.Tier) != "" {
		ws.Tier = strings.ToUpper(strings.TrimSpace(*dto.Tier))
	}
	if dto.SpendLimitUSD != nil {
		ws.SpendLimitUSD = *dto.SpendLimitUSD
	}
	if dto.EnforceSpendLimit != nil {
		ws.EnforceSpendLimit = *dto.EnforceSpendLimit
	}
	if dto.CustomDomain != nil {
		ws.CustomDomain = dto.CustomDomain
	}
	if dto.SAMLRequired != nil {
		ws.SAMLRequired = *dto.SAMLRequired
	}
	if dto.AllowedEmailDomains != nil {
		ws.AllowedEmailDomains = *dto.AllowedEmailDomains
	}
	if dto.Metadata != nil {
		for k, v := range dto.Metadata {
			ws.Metadata[k] = v
		}
	}
	ws.UpdatedAt = time.Now().UTC()
	return nil
}

// ToOrganization converts OrganizationCreateDTO to a persistent Organization entity.
func ToOrganization(dto *OrganizationCreateDTO) (*Organization, error) {
	if dto == nil {
		return nil, ErrNilEntity
	}
	if dto.WorkspaceID == uuid.Nil {
		return nil, ErrInvalidWorkspaceID
	}
	name := strings.TrimSpace(dto.Name)
	extID := strings.TrimSpace(dto.ExternalID)
	if name == "" || extID == "" {
		return nil, ErrEmptyRequiredField
	}
	now := time.Now().UTC()
	settings := dto.Settings
	if settings == nil {
		settings = make(JSONBMap)
	}

	return &Organization{
		BaseEntity: BaseEntity{
			ID:        uuid.New(),
			CreatedAt: now,
			UpdatedAt: now,
		},
		WorkspaceID:    dto.WorkspaceID,
		ExternalID:     extID,
		Name:           name,
		AvatarURL:      dto.AvatarURL,
		BillingEmail:   strings.ToLower(strings.TrimSpace(dto.BillingEmail)),
		SCMProvider:    strings.ToUpper(strings.TrimSpace(dto.SCMProvider)),
		InstallationID: dto.InstallationID,
		IsActive:       true,
		Settings:       settings,
	}, nil
}

// ToTrackedRepository converts TrackedRepositoryCreateDTO to a persistent TrackedRepository entity.
func ToTrackedRepository(dto *TrackedRepositoryCreateDTO) (*TrackedRepository, error) {
	if dto == nil {
		return nil, ErrNilEntity
	}
	if dto.WorkspaceID == uuid.Nil {
		return nil, ErrInvalidWorkspaceID
	}
	fullName := strings.TrimSpace(dto.FullName)
	if fullName == "" {
		return nil, ErrEmptyRequiredField
	}
	now := time.Now().UTC()
	configPath := dto.ConfigFilePath
	if configPath == "" {
		configPath = ".scandrix/config.json"
	}
	settings := dto.Settings
	if settings == nil {
		settings = make(JSONBMap)
	}

	return &TrackedRepository{
		TenantScopedEntity: TenantScopedEntity{
			BaseEntity: BaseEntity{
				ID:        uuid.New(),
				CreatedAt: now,
				UpdatedAt: now,
			},
			WorkspaceID: dto.WorkspaceID,
		},
		OrganizationID:  dto.OrganizationID,
		SCMProvider:     strings.ToUpper(strings.TrimSpace(dto.SCMProvider)),
		ExternalRepoID:  strings.TrimSpace(dto.ExternalRepoID),
		FullName:        fullName,
		DefaultBranch:   strings.TrimSpace(dto.DefaultBranch),
		IsPrivate:       dto.IsPrivate,
		IsActive:        true,
		WebhooksEnabled: true,
		ConfigFilePath:  configPath,
		CustomPrompt:    dto.CustomPrompt,
		Settings:        settings,
	}, nil
}

// ToDrixyRule converts DrixyRuleCreateDTO to a persistent DrixyRules entity.
func ToDrixyRule(dto *DrixyRuleCreateDTO) (*DrixyRules, error) {
	if dto == nil {
		return nil, ErrNilEntity
	}
	if dto.WorkspaceID == uuid.Nil {
		return nil, ErrInvalidWorkspaceID
	}
	key := strings.ToLower(strings.TrimSpace(dto.RuleKey))
	name := strings.TrimSpace(dto.Name)
	if key == "" || name == "" {
		return nil, ErrEmptyRequiredField
	}
	now := time.Now().UTC()
	weight := dto.WeightMultiplier
	if weight <= 0 {
		weight = 1.0
	}

	return &DrixyRules{
		TenantScopedEntity: TenantScopedEntity{
			BaseEntity: BaseEntity{
				ID:        uuid.New(),
				CreatedAt: now,
				UpdatedAt: now,
			},
			WorkspaceID: dto.WorkspaceID,
		},
		RuleKey:             key,
		Name:                name,
		Category:            strings.ToUpper(strings.TrimSpace(dto.Category)),
		Severity:            strings.ToUpper(strings.TrimSpace(dto.Severity)),
		Description:         strings.TrimSpace(dto.Description),
		Pattern:             dto.Pattern,
		PromptInstructions:  strings.TrimSpace(dto.PromptInstructions),
		BadExample:          dto.BadExample,
		GoodExample:         dto.GoodExample,
		ApplicableLanguages: dto.ApplicableLanguages,
		IsActive:            dto.IsActive,
		IsSystemDefault:     false,
		WeightMultiplier:    weight,
		LikesCount:          0,
	}, nil
}

// VerifyTenantBoundary asserts that an entity's WorkspaceID matches the request's tenant ID.
func VerifyTenantBoundary(entityWorkspaceID, requestWorkspaceID uuid.UUID) error {
	if requestWorkspaceID == uuid.Nil {
		return ErrInvalidWorkspaceID
	}
	if entityWorkspaceID != requestWorkspaceID {
		return ErrTenantMismatch
	}
	return nil
}
