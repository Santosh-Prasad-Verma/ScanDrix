// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: create_drixy_rule_dto.go
// ═══════════════════════════════════════════════════════════════

package dtos

import (
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
)

// CreateDrixyRuleDto is the input payload for creating or updating a rule.
type CreateDrixyRuleDto struct {
	UUID               string                                  `json:"uuid,omitempty"`
	Title              string                                  `json:"title"`
	Rule               string                                  `json:"rule"`
	Path               string                                  `json:"path,omitempty"`
	SourcePath         string                                  `json:"sourcePath,omitempty"`
	CentralizedConfig  *interfaces.DrixyRuleCentralizedConfig  `json:"centralizedConfig,omitempty"`
	SourceAnchor       string                                  `json:"sourceAnchor,omitempty"`
	Status             interfaces.DrixyRulesStatus             `json:"status,omitempty"`
	Severity           string                                  `json:"severity"`
	Label              string                                  `json:"label,omitempty"`
	Type               interfaces.DrixyRulesType               `json:"type,omitempty"`
	ExtendedContext    *interfaces.DrixyRulesExtendedContext   `json:"extendedContext,omitempty"`
	Examples           []interfaces.DrixyRulesExample          `json:"examples,omitempty"`
	RepositoryID       string                                  `json:"repositoryId"`
	SourceRepositoryID string                                  `json:"sourceRepositoryId,omitempty"`
	LastContentHash    string                                  `json:"lastContentHash,omitempty"`
	Origin             interfaces.DrixyRulesOrigin             `json:"origin,omitempty"`
	Reason             *string                                 `json:"reason,omitempty"`
	Scope              interfaces.DrixyRulesScope              `json:"scope,omitempty"`
	DirectoryID        string                                  `json:"directoryId,omitempty"`
	Inheritance        *interfaces.DrixyRulesInheritance       `json:"inheritance,omitempty"`
	ContextReferenceID string                                  `json:"contextReferenceId,omitempty"`
	RequestType        interfaces.DrixyRuleRequestType         `json:"requestType,omitempty"`
	TargetRuleUUID     string                                  `json:"targetRuleUuid,omitempty"`
	TeamID             string                                  `json:"teamId,omitempty"`
	PinnedSync         bool                                    `json:"pinnedSync,omitempty"`
	LockedByPlan       bool                                    `json:"lockedByPlan,omitempty"`
}

// ToRule converts the DTO to the domain interface model.
func (d *CreateDrixyRuleDto) ToRule() interfaces.DrixyRule {
	status := d.Status
	if status == "" {
		status = interfaces.DrixyRulesStatusActive
	}
	scope := d.Scope
	if scope == "" {
		scope = interfaces.DrixyRulesScopeFile
	}
	rType := d.Type
	if rType == "" {
		rType = interfaces.DrixyRulesTypeStandard
	}
	sev := d.Severity
	if sev == "" {
		sev = "HIGH"
	}
	return interfaces.DrixyRule{
		UUID:               d.UUID,
		Title:              d.Title,
		Rule:               d.Rule,
		Path:               d.Path,
		SourcePath:         d.SourcePath,
		CentralizedConfig:  d.CentralizedConfig,
		SourceAnchor:       d.SourceAnchor,
		Status:             status,
		Severity:           sev,
		Label:              d.Label,
		Type:               rType,
		ExtendedContext:    d.ExtendedContext,
		Examples:           d.Examples,
		RepositoryID:       d.RepositoryID,
		SourceRepositoryID: d.SourceRepositoryID,
		LastContentHash:    d.LastContentHash,
		Origin:             d.Origin,
		Reason:             d.Reason,
		Scope:              scope,
		DirectoryID:        d.DirectoryID,
		Inheritance:        d.Inheritance,
		ContextReferenceID: d.ContextReferenceID,
		RequestType:        d.RequestType,
		TargetRuleUUID:     d.TargetRuleUUID,
		PinnedSync:         d.PinnedSync,
		LockedByPlan:       d.LockedByPlan,
	}
}
