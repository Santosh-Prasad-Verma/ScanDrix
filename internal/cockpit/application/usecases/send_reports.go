package usecases

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/scandrix/backend/internal/cockpit/application"
	"github.com/scandrix/backend/internal/cockpit/domain"
)

const (
	reportChunkSize = 50
)

// EmailSender abstracts email transmission for executive reports.
type EmailSender interface {
	SendEmail(ctx context.Context, toEmail, subject, htmlBody string) error
}

// OrgDirectoryService retrieves organization details.
type OrgDirectoryService interface {
	GetOrganizationName(ctx context.Context, orgID string) (string, error)
}

// SendOrgReportInput specifies arguments for the executive monthly report.
type SendOrgReportInput struct {
	OrganizationID string `json:"organization_id"`
	StartDate      string `json:"start_date"` // YYYY-MM-DD
	EndDate        string `json:"end_date"`   // YYYY-MM-DD
	BaseURL        string `json:"base_url,omitempty"`
}

// SendOrgReportUseCase delivers executive review reports to organization owners.
type SendOrgReportUseCase struct {
	logger       *slog.Logger
	orgDirectory OrgDirectoryService
	recipients   domain.ReportRecipientsService
	reports      domain.CockpitReportsService
	emailSender  EmailSender
}

// NewSendOrgReportUseCase creates an initialized executive report use case.
func NewSendOrgReportUseCase(
	logger *slog.Logger,
	orgDirectory OrgDirectoryService,
	recipients domain.ReportRecipientsService,
	reports domain.CockpitReportsService,
	emailSender EmailSender,
) *SendOrgReportUseCase {
	if logger == nil {
		logger = slog.Default()
	}
	return &SendOrgReportUseCase{
		logger:       logger,
		orgDirectory: orgDirectory,
		recipients:   recipients,
		reports:      reports,
		emailSender:  emailSender,
	}
}

// Execute orchestrates data gathering, validation, and email dispatch for org reports.
func (uc *SendOrgReportUseCase) Execute(ctx context.Context, input SendOrgReportInput) (*application.SendReportResult, error) {
	orgName, err := uc.orgDirectory.GetOrganizationName(ctx, input.OrganizationID)
	if err != nil || orgName == "" {
		return &application.SendReportResult{
			OrganizationID: input.OrganizationID,
			Skipped:        "org-not-found",
		}, nil
	}

	owners, err := uc.recipients.GetOwners(ctx, input.OrganizationID)
	if err != nil || len(owners) == 0 {
		return &application.SendReportResult{
			OrganizationID: input.OrganizationID,
			Skipped:        "no-recipients",
		}, nil
	}

	data, err := uc.reports.BuildOrgReport(ctx, input.OrganizationID, orgName, input.StartDate, input.EndDate)
	if err != nil {
		return nil, fmt.Errorf("failed to build org report: %w", err)
	}

	if data.Reviews <= 0 {
		return &application.SendReportResult{
			OrganizationID: input.OrganizationID,
			Skipped:        "no-activity",
		}, nil
	}

	cockpitLink := application.BuildCockpitLink(input.BaseURL, application.CockpitLinkOptions{
		Tab:   "scandrix-review",
		Start: input.StartDate,
		End:   input.EndDate,
	})
	data.CockpitLink = cockpitLink

	result := &application.SendReportResult{
		OrganizationID: input.OrganizationID,
	}

	for _, owner := range owners {
		if uc.emailSender != nil {
			err := uc.emailSender.SendEmail(
				ctx,
				owner.Email,
				fmt.Sprintf("ScanDrix Executive Report - %s (%s to %s)", orgName, input.StartDate, input.EndDate),
				fmt.Sprintf("Reviews: %d, Implementation Rate: %.1f%%. View cockpit: %s",
					data.Reviews, data.ImplementationRate*100.0, data.CockpitLink),
			)
			if err != nil {
				result.Failed++
				result.Failures = append(result.Failures, application.SendFailure{
					Email:  owner.Email,
					Reason: err.Error(),
				})
			} else {
				result.Sent++
			}
		} else {
			result.Sent++
		}
	}

	return result, nil
}

// SendRepoReportInput specifies arguments for the per-repo digest.
type SendRepoReportInput struct {
	OrganizationID string `json:"organization_id"`
	StartDate      string `json:"start_date"` // YYYY-MM-DD
	EndDate        string `json:"end_date"`   // YYYY-MM-DD
	BaseURL        string `json:"base_url,omitempty"`
}

// SendRepoReportUseCase delivers repository-specific digests to authorized administrators.
type SendRepoReportUseCase struct {
	logger       *slog.Logger
	orgDirectory OrgDirectoryService
	recipients   domain.ReportRecipientsService
	reports      domain.CockpitReportsService
	emailSender  EmailSender
}

// NewSendRepoReportUseCase creates an initialized repo digest use case.
func NewSendRepoReportUseCase(
	logger *slog.Logger,
	orgDirectory OrgDirectoryService,
	recipients domain.ReportRecipientsService,
	reports domain.CockpitReportsService,
	emailSender EmailSender,
) *SendRepoReportUseCase {
	if logger == nil {
		logger = slog.Default()
	}
	return &SendRepoReportUseCase{
		logger:       logger,
		orgDirectory: orgDirectory,
		recipients:   recipients,
		reports:      reports,
		emailSender:  emailSender,
	}
}

// Execute aggregates repository metrics and delivers customized digests.
func (uc *SendRepoReportUseCase) Execute(ctx context.Context, input SendRepoReportInput) (*application.SendReportResult, error) {
	orgName, err := uc.orgDirectory.GetOrganizationName(ctx, input.OrganizationID)
	if err != nil || orgName == "" {
		return &application.SendReportResult{
			OrganizationID: input.OrganizationID,
			Skipped:        "org-not-found",
		}, nil
	}

	admins, err := uc.recipients.GetRepoAdmins(ctx, input.OrganizationID)
	if err != nil || len(admins) == 0 {
		return &application.SendReportResult{
			OrganizationID: input.OrganizationID,
			Skipped:        "no-recipients",
		}, nil
	}

	uniqueRepoSet := make(map[string]bool)
	for _, a := range admins {
		for _, r := range a.Repositories {
			uniqueRepoSet[r] = true
		}
	}
	var uniqueRepos []string
	for r := range uniqueRepoSet {
		uniqueRepos = append(uniqueRepos, r)
	}

	sections, err := uc.reports.BuildRepoSections(ctx, input.OrganizationID, uniqueRepos, input.StartDate, input.EndDate)
	if err != nil {
		return nil, fmt.Errorf("failed to build repo sections: %w", err)
	}

	sectionMap := make(map[string]domain.RepoReportSection, len(sections))
	for _, s := range sections {
		sectionMap[s.Repository] = s
	}

	result := &application.SendReportResult{
		OrganizationID: input.OrganizationID,
	}

	hasAnyActive := false
	for _, admin := range admins {
		var adminSections []domain.RepoReportSection
		for _, repo := range admin.Repositories {
			if s, found := sectionMap[repo]; found {
				s.CockpitLink = application.BuildCockpitLink(input.BaseURL, application.CockpitLinkOptions{
					Tab:        "scandrix-review",
					Start:      input.StartDate,
					End:        input.EndDate,
					Repository: repo,
				})
				adminSections = append(adminSections, s)
			}
		}

		if len(adminSections) == 0 {
			continue
		}
		hasAnyActive = true

		if uc.emailSender != nil {
			err := uc.emailSender.SendEmail(
				ctx,
				admin.Email,
				fmt.Sprintf("ScanDrix Repository Digest - %s (%s to %s)", orgName, input.StartDate, input.EndDate),
				fmt.Sprintf("You have %d active repositories reviewed in this period.", len(adminSections)),
			)
			if err != nil {
				result.Failed++
				result.Failures = append(result.Failures, application.SendFailure{
					Email:  admin.Email,
					Reason: err.Error(),
				})
			} else {
				result.Sent++
			}
		} else {
			result.Sent++
		}
	}

	if !hasAnyActive {
		return &application.SendReportResult{
			OrganizationID: input.OrganizationID,
			Skipped:        "no-activity",
		}, nil
	}

	return result, nil
}
