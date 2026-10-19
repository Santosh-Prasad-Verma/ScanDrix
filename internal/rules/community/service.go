// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package community

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// RuleWriter abstracts the persistence of rules into a tenant's workspace.
type RuleWriter interface {
	CreateReviewRule(ctx context.Context, rule *models.ReviewRule) error
}

// Service provides high-level operations for discovering, importing, and installing community rule packs.
type Service struct {
	importer *Importer
	packs    []RulePack
}

// NewService initializes the community rules service with default curated packs.
func NewService(importer ...*Importer) *Service {
	imp := NewImporter()
	if len(importer) > 0 && importer[0] != nil {
		imp = importer[0]
	}
	return &Service{
		importer: imp,
		packs:    CuratedPacks(),
	}
}

// ListPacks returns community rule packs filtered by search query, category, or tag.
func (s *Service) ListPacks(search, category, tag string) []RulePack {
	search = strings.ToLower(strings.TrimSpace(search))
	category = strings.ToUpper(strings.TrimSpace(category))
	tag = strings.ToLower(strings.TrimSpace(tag))

	results := make([]RulePack, 0, len(s.packs))
	for _, p := range s.packs {
		if category != "" && category != "ALL" && strings.ToUpper(p.Category) != category {
			continue
		}
		if search != "" {
			matches := strings.Contains(strings.ToLower(p.Title), search) ||
				strings.Contains(strings.ToLower(p.Description), search) ||
				strings.Contains(strings.ToLower(p.Author), search)
			if !matches {
				continue
			}
		}
		if tag != "" {
			tagMatch := false
			for _, t := range p.Tags {
				if strings.ToLower(t) == tag {
					tagMatch = true
					break
				}
			}
			if !tagMatch {
				continue
			}
		}
		results = append(results, p)
	}
	return results
}

// GetPack finds a pack by its unique ID or slug.
func (s *Service) GetPack(packID string) (*RulePack, error) {
	for _, p := range s.packs {
		if p.ID == packID || p.Slug == packID {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("rule pack '%s' not found", packID)
}

// InstallPack installs all rules contained within a pack directly into a workspace.
func (s *Service) InstallPack(ctx context.Context, wsID uuid.UUID, packID string, writer RuleWriter) (int, error) {
	if writer == nil {
		return 0, fmt.Errorf("rule writer is required to install rules")
	}

	pack, err := s.GetPack(packID)
	if err != nil {
		return 0, err
	}

	installed := 0
	for _, r := range pack.Rules {
		sev := models.FindingSeverity(strings.ToUpper(r.Severity))
		if sev == "" {
			sev = models.SeverityMedium
		}

		ruleRecord := &models.ReviewRule{
			ID:          uuid.New(),
			WorkspaceID: wsID,
			Name:        fmt.Sprintf("[%s] %s", pack.Title, r.Name),
			Description: r.Description,
			PathPattern: r.PathPattern,
			RuleType:    "REGEX",
			RuleContent: r.RegexRule,
			Severity:    sev,
			Remediation: r.Remediation,
			IsEnabled:   r.Enabled,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		}

		if err := writer.CreateReviewRule(ctx, ruleRecord); err != nil {
			return installed, fmt.Errorf("failed creating rule '%s': %w", r.Name, err)
		}
		installed++
	}

	return installed, nil
}

// ImportAndInstall fetches a remote rule pack, optionally installing it immediately.
func (s *Service) ImportAndInstall(ctx context.Context, wsID uuid.UUID, targetURL string, autoInstall bool, writer RuleWriter) (*RulePack, int, error) {
	pack, err := s.importer.FetchAndValidate(ctx, targetURL)
	if err != nil {
		return nil, 0, err
	}

	installed := 0
	if autoInstall && writer != nil {
		for _, r := range pack.Rules {
			sev := models.FindingSeverity(strings.ToUpper(r.Severity))
			if sev == "" {
				sev = models.SeverityMedium
			}

			ruleRecord := &models.ReviewRule{
				ID:          uuid.New(),
				WorkspaceID: wsID,
				Name:        fmt.Sprintf("[%s] %s", pack.Title, r.Name),
				Description: r.Description,
				PathPattern: r.PathPattern,
				RuleType:    "REGEX",
				RuleContent: r.RegexRule,
				Severity:    sev,
				Remediation: r.Remediation,
				IsEnabled:   r.Enabled,
				CreatedAt:   time.Now().UTC(),
				UpdatedAt:   time.Now().UTC(),
			}

			if err := writer.CreateReviewRule(ctx, ruleRecord); err != nil {
				return pack, installed, fmt.Errorf("failed creating imported rule '%s': %w", r.Name, err)
			}
			installed++
		}
	}

	return pack, installed, nil
}
