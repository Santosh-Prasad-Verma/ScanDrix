package http

import (
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/automation/domain"
)

// AutomationDTO models individual automation configurations in requests.
type AutomationDTO struct {
	AutomationUUID string                `json:"automationUuid"`
	AutomationType domain.AutomationType `json:"automationType"`
	Status         bool                  `json:"status"`
}

// Validate ensures required fields and format compliance.
func (d AutomationDTO) Validate() error {
	if strings.TrimSpace(d.AutomationUUID) == "" {
		return errors.New("automationUuid is required")
	}
	if _, err := uuid.Parse(d.AutomationUUID); err != nil {
		return errors.New("automationUuid must be a valid UUID")
	}
	if strings.TrimSpace(string(d.AutomationType)) == "" {
		return errors.New("automationType is required")
	}
	return nil
}

// TeamAutomationsDTO encapsulates team automation batch updates.
type TeamAutomationsDTO struct {
	TeamID      string          `json:"teamId"`
	Automations []AutomationDTO `json:"automations"`
}

// Validate checks team identifier and nested automations.
func (d TeamAutomationsDTO) Validate() error {
	if strings.TrimSpace(d.TeamID) == "" {
		return errors.New("teamId is required")
	}
	if len(d.Automations) == 0 {
		return errors.New("automations list cannot be empty")
	}
	for i, a := range d.Automations {
		if err := a.Validate(); err != nil {
			return err
		}
		_ = i
	}
	return nil
}

// OrganizationAutomationsDTO encapsulates organization-level automation batch updates.
type OrganizationAutomationsDTO struct {
	OrganizationID string          `json:"organizationId"`
	Automations    []AutomationDTO `json:"automations"`
}

// Validate checks organization identifier and nested automations.
func (d OrganizationAutomationsDTO) Validate() error {
	if strings.TrimSpace(d.OrganizationID) == "" {
		return errors.New("organizationId is required")
	}
	if len(d.Automations) == 0 {
		return errors.New("automations list cannot be empty")
	}
	for _, a := range d.Automations {
		if err := a.Validate(); err != nil {
			return err
		}
	}
	return nil
}
