// Package infrastructure implements event consumption and background synchronization for centralized config updates.
package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/scandrix/backend/internal/centralizedconfig/domain"
)

// SyncEvent represents a message emitted when repository configuration changes.
type SyncEvent struct {
	OrganizationID string `json:"organizationId"`
	TeamID         string `json:"teamId"`
	RepositoryID   string `json:"repositoryId,omitempty"`
	TriggerSource  string `json:"triggerSource"`
}

// SyncListener consumes sync messages and triggers repository reconciliation.
type SyncListener struct {
	configService domain.CentralizedConfigService
}

// NewSyncListener creates a new SyncListener instance.
func NewSyncListener(svc domain.CentralizedConfigService) *SyncListener {
	return &SyncListener{configService: svc}
}

// HandleSyncMessage processes an incoming sync event payload.
func (l *SyncListener) HandleSyncMessage(ctx context.Context, payloadBytes []byte) error {
	var event SyncEvent
	if err := json.Unmarshal(payloadBytes, &event); err != nil {
		return fmt.Errorf("failed to decode sync event: %w", err)
	}

	if event.OrganizationID == "" {
		return fmt.Errorf("missing organizationId in sync event")
	}

	return l.configService.SyncRepositoryConfig(ctx, event.OrganizationID, event.TeamID)
}
