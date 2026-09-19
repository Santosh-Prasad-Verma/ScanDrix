package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/notifications/domain/catalog"
	"github.com/scandrix/backend/internal/notifications/domain/contracts"
	"github.com/scandrix/backend/internal/notifications/domain/entities"
	"github.com/scandrix/backend/internal/notifications/domain/enums"
)

// NotificationConfigEvent describes catalog event configuration for frontend consumption.
type NotificationConfigEvent struct {
	Event           string            `json:"event"`
	Label           string            `json:"label"`
	Category        string            `json:"category"`
	Criticality     enums.Criticality `json:"criticality"`
	DefaultChannels map[string]bool   `json:"defaultChannels"`
	Icon            string            `json:"icon,omitempty"`
	PageSeverity    bool              `json:"pageSeverity,omitempty"`
	ActionLabel     string            `json:"actionLabel,omitempty"`
	DefaultRoles    []string          `json:"defaultRoles,omitempty"`
}

// OptionItem represents a generic dropdown/selector option for frontend forms.
type OptionItem struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// NotificationConfig contains complete metadata needed by the admin settings UI.
type NotificationConfig struct {
	Events        []NotificationConfigEvent `json:"events"`
	Channels      []OptionItem              `json:"channels"`
	Criticalities []OptionItem              `json:"criticalities"`
	Categories    []OptionItem              `json:"categories"`
	Roles         []OptionItem              `json:"roles"`
}

// UpsertRuleDto defines request payload for updating or removing a routing rule.
type UpsertRuleDto struct {
	Event    string          `json:"event"`
	Role     string          `json:"role"`
	Channels map[string]bool `json:"channels"`
	Delete   bool            `json:"delete,omitempty"`
}

// RoutingRuleService handles owner-configured routing rules and catalog introspection.
type RoutingRuleService struct {
	repo contracts.RoutingRuleRepository
}

// NewRoutingRuleService creates a new routing rule management service.
func NewRoutingRuleService(repo contracts.RoutingRuleRepository) *RoutingRuleService {
	return &RoutingRuleService{repo: repo}
}

// FindByOrganization returns all routing rules customized for an organization.
func (s *RoutingRuleService) FindByOrganization(ctx context.Context, orgID uuid.UUID) ([]*entities.RoutingRule, error) {
	if s.repo == nil {
		return nil, nil
	}
	return s.repo.FindByOrganization(ctx, orgID)
}

// GetConfig returns complete static and dynamic schema for frontend routing settings.
func (s *RoutingRuleService) GetConfig() NotificationConfig {
	var events []NotificationConfigEvent
	for event, def := range catalog.EventDefaultsMap {
		defaultChannels := make(map[string]bool)
		for ch := range enums.ActiveChannels {
			enabled := false
			for _, dch := range def.DefaultChannels {
				if dch == ch {
					enabled = true
					break
				}
			}
			defaultChannels[string(ch)] = enabled
		}

		events = append(events, NotificationConfigEvent{
			Event:           string(event),
			Label:           def.Label,
			Category:        def.Category,
			Criticality:     def.Criticality,
			DefaultChannels: defaultChannels,
			Icon:            string(def.Icon),
			PageSeverity:    def.PageSeverity,
			ActionLabel:     def.ActionLabel,
			DefaultRoles:    def.DefaultRoles,
		})
	}

	var channels []OptionItem
	for ch := range enums.ActiveChannels {
		label := catalog.ChannelLabels[ch]
		if label == "" {
			label = string(ch)
		}
		channels = append(channels, OptionItem{Value: string(ch), Label: label})
	}

	var criticalities []OptionItem
	for _, crit := range []enums.Criticality{enums.CriticalitySystem, enums.CriticalityCritical, enums.CriticalityTransactional, enums.CriticalityInformational} {
		criticalities = append(criticalities, OptionItem{Value: string(crit), Label: catalog.CriticalityLabels[crit]})
	}

	var categories []OptionItem
	for _, cat := range catalog.EventCategories {
		label := catalog.CategoryLabels[cat]
		if label == "" {
			label = cat
		}
		categories = append(categories, OptionItem{Value: cat, Label: label})
	}

	roleOrder := []string{
		catalog.RoleWildcard,
		catalog.RoleOwner,
		catalog.RoleBillingManager,
		catalog.RoleRepoAdmin,
		catalog.RoleContributor,
		catalog.RoleAdmin,
		catalog.RoleMaintainer,
		catalog.RoleReviewer,
		catalog.RoleViewer,
	}
	var roles []OptionItem
	for _, r := range roleOrder {
		roles = append(roles, OptionItem{Value: r, Label: catalog.RoleLabels[r]})
	}

	return NotificationConfig{
		Events:        events,
		Channels:      channels,
		Criticalities: criticalities,
		Categories:    categories,
		Roles:         roles,
	}
}

// UpsertRule creates, updates, or deletes a specific routing rule for an organization.
func (s *RoutingRuleService) UpsertRule(ctx context.Context, orgID uuid.UUID, dto UpsertRuleDto) error {
	if s.repo == nil {
		return fmt.Errorf("routing rule repository is not initialized")
	}

	if dto.Delete {
		_, err := s.repo.DeleteByOrgEventRole(ctx, orgID, dto.Event, dto.Role)
		return err
	}

	defaults, exists := catalog.EventDefaultsMap[catalog.Event(dto.Event)]
	var category *string
	if exists {
		cat := defaults.Category
		category = &cat
	}

	rule := entities.NewRoutingRule(
		orgID,
		dto.Event,
		category,
		dto.Role,
		dto.Channels,
	)
	return s.repo.Upsert(ctx, rule)
}

// SeedDefaults creates default wildcard routing rules for a newly created organization.
func (s *RoutingRuleService) SeedDefaults(ctx context.Context, orgID uuid.UUID) error {
	if s.repo == nil {
		return nil
	}

	var rules []*entities.RoutingRule
	for event, def := range catalog.EventDefaultsMap {
		channels := make(map[string]bool)
		for _, ch := range def.DefaultChannels {
			channels[string(ch)] = true
		}
		cat := def.Category
		rules = append(rules, entities.NewRoutingRule(
			orgID,
			string(event),
			&cat,
			catalog.RoleWildcard,
			channels,
		))
	}

	return s.repo.UpsertBatch(ctx, rules)
}
