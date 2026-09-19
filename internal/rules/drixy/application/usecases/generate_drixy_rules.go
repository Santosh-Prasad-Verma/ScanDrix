// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: generate_drixy_rules.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
	"fmt"

	appServices "github.com/scandrix/backend/internal/rules/drixy/application/services"
	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
	"github.com/scandrix/backend/internal/rules/drixy/dtos"
)

// GenerateDrixyRulesDTO specifies time range and repository filters for past PR learning.
type GenerateDrixyRulesDTO = dtos.GenerateDrixyRulesDTO

// ICommentAnalysisService analyzes PR review comments and extracts reusable coding rules.
type ICommentAnalysisService interface {
	GenerateRulesFromComments(ctx context.Context, comments []string) ([]dtos.CreateDrixyRuleDto, error)
}

// GenerateDrixyRulesUseCase synthesizes organization coding rules from historical PR comments.
type GenerateDrixyRulesUseCase struct {
	rulesService          contracts.IDrixyRulesService
	createOrUpdateUseCase *CreateOrUpdateDrixyRuleUseCase
	commentAnalysis       ICommentAnalysisService
	notificationUseCase   *SendRulesNotificationUseCase
}

// NewGenerateDrixyRulesUseCase constructs the rules generator use case.
func NewGenerateDrixyRulesUseCase(
	rulesService contracts.IDrixyRulesService,
	createOrUpdateUseCase *CreateOrUpdateDrixyRuleUseCase,
	commentAnalysis ICommentAnalysisService,
	notificationUseCase *SendRulesNotificationUseCase,
) *GenerateDrixyRulesUseCase {
	return &GenerateDrixyRulesUseCase{
		rulesService:          rulesService,
		createOrUpdateUseCase: createOrUpdateUseCase,
		commentAnalysis:       commentAnalysis,
		notificationUseCase:   notificationUseCase,
	}
}

// Execute triggers the AI rule synthesis workflow across historical PR activity.
func (uc *GenerateDrixyRulesUseCase) Execute(
	ctx context.Context,
	dto GenerateDrixyRulesDTO,
	organizationID string,
) ([]*interfaces.DrixyRule, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	model := appServices.ResolveDrixyRulesModelPolicy("pro", "")
	_ = model
	var userInfo *contracts.UserAuditInfo

	var candidates []dtos.CreateDrixyRuleDto
	if uc.commentAnalysis != nil && len(dto.Comments) > 0 {
		cands, err := uc.commentAnalysis.GenerateRulesFromComments(ctx, dto.Comments)
		if err == nil {
			candidates = cands
		}
	}

	createdRules := make([]*interfaces.DrixyRule, 0, len(candidates))
	ruleTitles := make([]string, 0, len(candidates))

	primaryRepoID := ""
	if len(dto.RepositoriesIDs) > 0 {
		primaryRepoID = dto.RepositoriesIDs[0]
	}

	for _, c := range candidates {
		c.Origin = interfaces.DrixyRulesOriginPastReviews
		c.Status = interfaces.DrixyRulesStatusActive
		c.Type = interfaces.DrixyRulesTypeStandard
		if c.RepositoryID == "" && primaryRepoID != "" {
			c.RepositoryID = primaryRepoID
		}

		created, err := uc.createOrUpdateUseCase.Execute(ctx, c, organizationID, userInfo)
		if err != nil {
			continue
		}
		createdRules = append(createdRules, created)
		ruleTitles = append(ruleTitles, created.Title)
	}

	if uc.notificationUseCase != nil && len(ruleTitles) > 0 {
		_ = uc.notificationUseCase.Execute(ctx, organizationID, ruleTitles)
	}

	return createdRules, nil
}
