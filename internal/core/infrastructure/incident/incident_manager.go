// Package incident manages observability heartbeats, incident deduplication, and SLA protection.
package incident

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/telemetry"
)

const (
	DefaultDeduplicationTTL = 5 * time.Minute
)

// IncidentManager manages heartbeat pings and throttles duplicate incident generation.
type IncidentManager struct {
	client           *BetterStackClient
	dedupTTL         time.Duration
	dedupMu          sync.RWMutex
	recentIncidents  map[string]time.Time
	logger           *telemetry.LoggerWrapper
	environment      string
	componentName    string
}

// NewIncidentManager instantiates an IncidentManager.
func NewIncidentManager(
	client *BetterStackClient,
	env string,
	component string,
	dedupTTL time.Duration,
) *IncidentManager {
	if dedupTTL <= 0 {
		dedupTTL = DefaultDeduplicationTTL
	}
	if env == "" {
		env = "production"
	}
	if component == "" {
		component = "backend"
	}

	return &IncidentManager{
		client:          client,
		dedupTTL:        dedupTTL,
		recentIncidents: make(map[string]time.Time),
		logger:          telemetry.DefaultLogger(),
		environment:     env,
		componentName:   component,
	}
}

// PingHeartbeat sends a healthy heartbeat check-in.
func (m *IncidentManager) PingHeartbeat(ctx context.Context, heartbeatURL string) error {
	if heartbeatURL == "" {
		return nil
	}
	err := m.client.PingHeartbeat(ctx, heartbeatURL)
	if err != nil {
		m.logger.Warn(fmt.Sprintf("Failed to ping heartbeat: %v", err))
		return err
	}
	return nil
}

// FailHeartbeat notifies BetterStack of a service degradation or monitor failure.
func (m *IncidentManager) FailHeartbeat(ctx context.Context, heartbeatURL string, message string, extra map[string]any) error {
	if heartbeatURL == "" {
		return nil
	}

	contextData := m.BuildContext(extra)
	err := m.client.FailHeartbeat(ctx, heartbeatURL, message, contextData)
	if err != nil {
		m.logger.Error("Failed to dispatch heartbeat failure", err)
		return err
	}
	return nil
}

// ReportIncident creates an incident if an identical incident hasn't fired within the deduplication window.
func (m *IncidentManager) ReportIncident(ctx context.Context, key string, payload CreateIncidentPayload) (bool, error) {
	if key == "" {
		key = payload.Name
	}

	m.dedupMu.Lock()
	now := time.Now()
	if lastFired, exists := m.recentIncidents[key]; exists {
		if now.Sub(lastFired) < m.dedupTTL {
			m.dedupMu.Unlock()
			m.logger.Debug(fmt.Sprintf("Deduplicated incident [%s], suppressed duplicate alert", key))
			return false, nil
		}
	}
	m.recentIncidents[key] = now
	m.dedupMu.Unlock()

	err := m.client.CreateIncident(ctx, payload)
	if err != nil {
		m.logger.Error("Failed to dispatch incident to BetterStack", err)
		return false, err
	}

	return true, nil
}

// BuildContext constructs standardized operational metadata for observability reports.
func (m *IncidentManager) BuildContext(extra map[string]any) map[string]any {
	result := map[string]any{
		"environment": m.environment,
		"component":   m.componentName,
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
	}
	for k, v := range extra {
		result[k] = v
	}
	return result
}
