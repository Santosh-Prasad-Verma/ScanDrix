// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package usecases

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/pkg/models"
)

// TemplateVariables carries dynamic review context for message interpolation.
type TemplateVariables struct {
	PRNumber        int
	Title           string
	Author          string
	HeadSHA         string
	BaseSHA         string
	Branch          string
	CriticalCount   int
	HighCount       int
	MediumCount     int
	LowCount        int
	TotalFindings   int
	ReviewDuration  time.Duration
	PassedReview    bool
	Platform        string // "github", "gitlab", "bitbucket"
	TraceURL        string
	ScandrixVersion string
}

// ReviewMessageTemplateEngine compiles and renders customized PR summary and status messages.
type ReviewMessageTemplateEngine struct{}

// NewReviewMessageTemplateEngine constructs the template engine.
func NewReviewMessageTemplateEngine() *ReviewMessageTemplateEngine {
	return &ReviewMessageTemplateEngine{}
}

// Render interpolates template variables into the raw template string.
func (e *ReviewMessageTemplateEngine) Render(template string, vars TemplateVariables) string {
	if template == "" {
		return ""
	}

	res := template
	res = strings.ReplaceAll(res, "{{pr_number}}", strconv.Itoa(vars.PRNumber))
	res = strings.ReplaceAll(res, "{{title}}", vars.Title)
	res = strings.ReplaceAll(res, "{{author}}", vars.Author)
	res = strings.ReplaceAll(res, "{{head_sha}}", safeShortSHA(vars.HeadSHA))
	res = strings.ReplaceAll(res, "{{base_sha}}", safeShortSHA(vars.BaseSHA))
	res = strings.ReplaceAll(res, "{{branch}}", vars.Branch)
	res = strings.ReplaceAll(res, "{{critical_count}}", strconv.Itoa(vars.CriticalCount))
	res = strings.ReplaceAll(res, "{{high_count}}", strconv.Itoa(vars.HighCount))
	res = strings.ReplaceAll(res, "{{medium_count}}", strconv.Itoa(vars.MediumCount))
	res = strings.ReplaceAll(res, "{{low_count}}", strconv.Itoa(vars.LowCount))
	res = strings.ReplaceAll(res, "{{total_findings}}", strconv.Itoa(vars.TotalFindings))
	res = strings.ReplaceAll(res, "{{review_time_ms}}", strconv.FormatInt(vars.ReviewDuration.Milliseconds(), 10))
	res = strings.ReplaceAll(res, "{{trace_url}}", vars.TraceURL)

	version := vars.ScandrixVersion
	if version == "" {
		version = "v2.5.0-enterprise"
	}
	res = strings.ReplaceAll(res, "{{scandrix_version}}", version)

	statusBadge := "PASSED"
	if !vars.PassedReview {
		statusBadge = "CHANGES_REQUESTED"
	}
	res = strings.ReplaceAll(res, "{{status_badge}}", statusBadge)

	return res
}

func safeShortSHA(sha string) string {
	if len(sha) >= 7 {
		return sha[:7]
	}
	return sha
}

// PRMessagesHierarchyCoordinator coordinates multi-level message overrides, inheritance pruning,
// directory prefix matching, and SCM markdown banner synthesis.
type PRMessagesHierarchyCoordinator struct {
	repo           domain.IPullRequestMessagesRepository
	templateEngine *ReviewMessageTemplateEngine
	mu             sync.RWMutex
}

// NewPRMessagesHierarchyCoordinator constructs a hierarchy coordinator.
func NewPRMessagesHierarchyCoordinator(repo domain.IPullRequestMessagesRepository) *PRMessagesHierarchyCoordinator {
	return &PRMessagesHierarchyCoordinator{
		repo:           repo,
		templateEngine: NewReviewMessageTemplateEngine(),
	}
}

// SaveWithInheritanceDetection saves or updates message configurations, automatically pruning
// child configurations that match the parent configuration to enable seamless inheritance.
func (c *PRMessagesHierarchyCoordinator) SaveWithInheritanceDetection(
	ctx context.Context,
	msg *domain.PullRequestMessages,
) (*domain.PullRequestMessages, bool, error) {
	if msg.OrganizationID == "" {
		return nil, false, fmt.Errorf("organizationId is required")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// 1. Check if non-global config matches its parent
	if msg.ConfigLevel != domain.ConfigLevelGlobal {
		parent, err := c.resolveParentConfiguration(ctx, msg)
		if err == nil && parent != nil {
			if c.isIdenticalConfiguration(msg, parent) {
				// Matches parent: Prune redundant child configuration if one exists
				_, _ = c.repo.DeleteByFilter(ctx, domain.MessagesFilter{
					OrganizationID: msg.OrganizationID,
					ConfigLevel:    msg.ConfigLevel,
					RepositoryID:   msg.RepositoryID,
					DirectoryID:    msg.DirectoryID,
				})
				return parent, true, nil // true = inherited from parent
			}
		}
	}

	// 2. Persist or update the configuration
	existing, err := c.repo.FindOne(ctx, domain.MessagesFilter{
		OrganizationID: msg.OrganizationID,
		ConfigLevel:    msg.ConfigLevel,
		RepositoryID:   msg.RepositoryID,
		DirectoryID:    msg.DirectoryID,
	})

	now := time.Now().UTC()
	if err == nil && existing != nil {
		msg.ID = existing.ID
		msg.CreatedAt = existing.CreatedAt
		msg.UpdatedAt = now
		updated, err := c.repo.Update(ctx, msg)
		return updated, false, err
	}

	msg.ID = uuid.New()
	msg.CreatedAt = now
	msg.UpdatedAt = now
	created, err := c.repo.Create(ctx, msg)
	return created, false, err
}

// ResolveEffectiveForFile determines the most specific message configuration for a file path
// using directory prefix hierarchy: File Directory -> Repository -> Global -> Builtin Default.
func (c *PRMessagesHierarchyCoordinator) ResolveEffectiveForFile(
	ctx context.Context,
	orgID string,
	repoID string,
	filePath string,
) (*domain.PullRequestMessages, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organizationId is required")
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	// 1. Check matching directory configurations in order of longest directory prefix
	if repoID != "" && filePath != "" {
		dirConfigs, err := c.repo.Find(ctx, domain.MessagesFilter{
			OrganizationID: orgID,
			ConfigLevel:    domain.ConfigLevelDirectory,
			RepositoryID:   repoID,
		})
		if err == nil && len(dirConfigs) > 0 {
			// Sort directory configs by directory path length descending
			sort.Slice(dirConfigs, func(i, j int) bool {
				return len(dirConfigs[i].DirectoryID) > len(dirConfigs[j].DirectoryID)
			})

			normFile := filepath.ToSlash(filepath.Clean(filePath))
			for _, dc := range dirConfigs {
				normDir := filepath.ToSlash(filepath.Clean(dc.DirectoryID))
				if strings.HasPrefix(normFile, normDir) {
					return &dc, nil
				}
			}
		}
	}

	// 2. Check repository-level configuration
	if repoID != "" {
		repoConfig, err := c.repo.FindOne(ctx, domain.MessagesFilter{
			OrganizationID: orgID,
			ConfigLevel:    domain.ConfigLevelRepository,
			RepositoryID:   repoID,
		})
		if err == nil && repoConfig != nil {
			return repoConfig, nil
		}
	}

	// 3. Check organization global configuration
	globalConfig, err := c.repo.FindOne(ctx, domain.MessagesFilter{
		OrganizationID: orgID,
		ConfigLevel:    domain.ConfigLevelGlobal,
	})
	if err == nil && globalConfig != nil {
		return globalConfig, nil
	}

	// 4. Built-in defaults
	return c.builtinDefaultMessages(orgID), nil
}

// RenderReviewSummaryBanner formats a complete enterprise markdown review comment.
func (c *PRMessagesHierarchyCoordinator) RenderReviewSummaryBanner(
	config *domain.PullRequestMessages,
	vars TemplateVariables,
	findings []models.CodeFinding,
) string {
	var sb strings.Builder

	// 1. Header Banner
	statusEmoji := "✅"
	statusText := "ScanDrix Automated Review: Passed"
	if !vars.PassedReview {
		statusEmoji = "❌"
		statusText = "ScanDrix Automated Review: Changes Requested"
	}

	sb.WriteString(fmt.Sprintf("## %s %s\n\n", statusEmoji, statusText))

	// 2. Custom or Default Intro Message
	introTemplate := ""
	if config != nil && config.EndReviewMessage != nil && config.EndReviewMessage.Content != "" {
		introTemplate = config.EndReviewMessage.Content
	} else {
		introTemplate = "Automated multi-agent security and code quality review completed for **{{author}}** on branch `{{branch}}`."
	}
	sb.WriteString(c.templateEngine.Render(introTemplate, vars))
	sb.WriteString("\n\n")

	// 3. Metrics Summary Table
	sb.WriteString("| Severity | Count | Review Status |\n")
	sb.WriteString("| :--- | :---: | :--- |\n")
	sb.WriteString(fmt.Sprintf("| 🔴 Critical | %d | %s |\n", vars.CriticalCount, severityBadge(vars.CriticalCount, true)))
	sb.WriteString(fmt.Sprintf("| 🟠 High | %d | %s |\n", vars.HighCount, severityBadge(vars.HighCount, true)))
	sb.WriteString(fmt.Sprintf("| 🟡 Medium | %d | %s |\n", vars.MediumCount, severityBadge(vars.MediumCount, false)))
	sb.WriteString(fmt.Sprintf("| 🔵 Low / Info | %d | %s |\n\n", vars.LowCount, severityBadge(vars.LowCount, false)))

	// 4. Grouped Findings by File
	if len(findings) > 0 {
		sb.WriteString("### 🔍 Detailed Recommendations\n\n")
		grouped := groupFindingsByFile(findings)

		for file, fileFindings := range grouped {
			sb.WriteString(fmt.Sprintf("<details>\n<summary><b>%s</b> (%d issues)</summary>\n\n", file, len(fileFindings)))

			for _, f := range fileFindings {
				sb.WriteString(fmt.Sprintf("#### [%s] %s (Line %d)\n", f.Severity, f.Title, f.StartLine))
				sb.WriteString(fmt.Sprintf("> %s\n\n", f.Description))
				if f.Remediation != "" {
					sb.WriteString(fmt.Sprintf("**Remediation:** %s\n\n", f.Remediation))
				}
				if f.SuggestedDiff != "" {
					sb.WriteString("```suggestion\n")
					sb.WriteString(f.SuggestedDiff)
					sb.WriteString("\n```\n\n")
				}
			}

			sb.WriteString("</details>\n\n")
		}
	} else {
		sb.WriteString("> 🎉 **Zero critical or high severity defects detected.** Code changes meet team standards.\n\n")
	}

	// 5. Footer Trace
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("<sub>Reviewed by [ScanDrix AI](https://scandrix.dev) | Review Time: %dms | SHA: `%s`</sub>\n",
		vars.ReviewDuration.Milliseconds(), safeShortSHA(vars.HeadSHA)))

	return sb.String()
}

func (c *PRMessagesHierarchyCoordinator) resolveParentConfiguration(
	ctx context.Context,
	msg *domain.PullRequestMessages,
) (*domain.PullRequestMessages, error) {
	switch msg.ConfigLevel {
	case domain.ConfigLevelDirectory:
		// Parent is repository config, or global if repository has none
		repoConfig, err := c.repo.FindOne(ctx, domain.MessagesFilter{
			OrganizationID: msg.OrganizationID,
			ConfigLevel:    domain.ConfigLevelRepository,
			RepositoryID:   msg.RepositoryID,
		})
		if err == nil && repoConfig != nil {
			return repoConfig, nil
		}
		fallthrough

	case domain.ConfigLevelRepository:
		// Parent is global config
		return c.repo.FindOne(ctx, domain.MessagesFilter{
			OrganizationID: msg.OrganizationID,
			ConfigLevel:    domain.ConfigLevelGlobal,
		})

	default:
		return nil, nil
	}
}

func (c *PRMessagesHierarchyCoordinator) isIdenticalConfiguration(a, b *domain.PullRequestMessages) bool {
	if a == nil || b == nil {
		return false
	}
	return contentEqual(a.StartReviewMessage, b.StartReviewMessage) &&
		contentEqual(a.EndReviewMessage, b.EndReviewMessage) &&
		contentEqual(a.ErrorReviewMessage, b.ErrorReviewMessage)
}

func contentEqual(a, b *domain.PullRequestMessageContent) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return strings.TrimSpace(a.Content) == strings.TrimSpace(b.Content) && a.Status == b.Status
}

func (c *PRMessagesHierarchyCoordinator) builtinDefaultMessages(orgID string) *domain.PullRequestMessages {
	start := domain.DefaultStartReviewTemplate()
	end := domain.DefaultEndReviewTemplate()
	err := domain.DefaultErrorReviewTemplate()

	return &domain.PullRequestMessages{
		ID:                 uuid.New(),
		OrganizationID:     orgID,
		ConfigLevel:        domain.ConfigLevelGlobal,
		StartReviewMessage: &start,
		EndReviewMessage:   &end,
		ErrorReviewMessage: &err,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
}

func groupFindingsByFile(findings []models.CodeFinding) map[string][]models.CodeFinding {
	m := make(map[string][]models.CodeFinding)
	for _, f := range findings {
		m[f.FilePath] = append(m[f.FilePath], f)
	}
	return m
}

func severityBadge(count int, isHighRisk bool) string {
	if count == 0 {
		return "Clean"
	}
	if isHighRisk {
		return "Action Required"
	}
	return "Advisory"
}
