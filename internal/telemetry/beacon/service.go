// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Telemetry Subsystem
// Package: beacon
// File: service.go
// ═══════════════════════════════════════════════════════════════

package beacon

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// ISelfHostedBeaconService coordinates the daily anonymous usage heartbeat lifecycle.
type ISelfHostedBeaconService interface {
	IsDisabled() bool
	Run(ctx context.Context) error
	Preview(ctx context.Context) (map[string]interface{}, error)
}

// SelfHostedBeaconService orchestrates heartbeat metrics collection, deduplication, and transmission.
type SelfHostedBeaconService struct {
	store     TelemetryStateStore
	collector IHeartbeatCollectorService
	transport IBeaconHTTPProvider
	logger    *slog.Logger
}

// NewSelfHostedBeaconService constructs an operational beacon orchestrator.
func NewSelfHostedBeaconService(
	store TelemetryStateStore,
	collector IHeartbeatCollectorService,
	transport IBeaconHTTPProvider,
	logger *slog.Logger,
) *SelfHostedBeaconService {
	if logger == nil {
		logger = slog.Default()
	}
	return &SelfHostedBeaconService{
		store:     store,
		collector: collector,
		transport: transport,
		logger:    logger.With("component", "self_hosted_beacon_service"),
	}
}

// IsDisabled delegates to the transport provider to check current opt-out status.
func (s *SelfHostedBeaconService) IsDisabled() bool {
	if s.transport == nil {
		return true
	}
	return s.transport.IsDisabled()
}

// Run executes the daily heartbeat workflow with multi-worker claim protection and daily deduplication.
// In accordance with Master Rules, failures are logged and swallowed so host operations are never disrupted.
func (s *SelfHostedBeaconService) Run(ctx context.Context) error {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Warn("Self-hosted beacon run recovered from panic (swallowed)",
				"panic", fmt.Sprintf("%v", r),
			)
		}
	}()

	if s.IsDisabled() {
		return nil
	}

	now := time.Now().UTC()
	today := now.Format("2006-01-02")

	state, err := s.loadOrInitState(ctx)
	if err != nil {
		s.logger.Warn("Failed loading or initializing telemetry state (swallowed)", "error", err)
		return nil
	}

	// 1. Daily deduplication check
	if state.LastSentDay != nil && *state.LastSentDay == today {
		s.logger.Debug("Heartbeat already transmitted today; skipping", "day", today)
		return nil
	}

	// 2. In-flight multi-worker claim check
	if hasFreshInFlightClaim(state, today, now) {
		s.logger.Debug("Heartbeat transmission already in-flight by another worker; skipping", "day", today)
		return nil
	}

	// 3. Claim lock
	claimedState := *state
	claimedState.InFlightDay = &today
	startedAtStr := now.Format(time.RFC3339Nano)
	claimedState.InFlightStartedAt = &startedAtStr

	if err := s.store.SetState(ctx, &claimedState); err != nil {
		s.logger.Warn("Failed persisting in-flight telemetry claim (swallowed)", "error", err)
		return nil
	}

	// 4. Collect metrics
	metrics, err := s.collector.Collect(ctx, CollectInput{
		FirstSeenAt: state.FirstSeenAt,
	})
	if err != nil {
		s.logger.Warn("Failed collecting telemetry metrics (swallowed)", "error", err)
		_ = s.clearInFlightClaim(ctx, &claimedState)
		return nil
	}

	// 5. Construct wire payload
	payload := s.buildPayload(state.InstanceID, now, metrics)

	// 6. Transmit payload
	ok, err := s.transport.Send(ctx, payload, metrics.ScanDrix.Version)
	if err != nil || !ok {
		s.logger.Warn("Heartbeat transmission unsuccessful (releasing claim)", "error", err)
		_ = s.clearInFlightClaim(ctx, &claimedState)
		return nil
	}

	// 7. Success: update last_sent_day and clear in-flight state
	claimedState.LastSentDay = &today
	claimedState.InFlightDay = nil
	claimedState.InFlightStartedAt = nil
	if err := s.store.SetState(ctx, &claimedState); err != nil {
		s.logger.Warn("Failed persisting successful telemetry sent state (swallowed)", "error", err)
	}

	s.logger.Info("Self-hosted anonymous heartbeat transmitted successfully",
		"instance_id", state.InstanceID,
		"day", today,
	)
	return nil
}

// Preview compiles and returns the exact JSON payload without transmitting or advancing last_sent_day.
func (s *SelfHostedBeaconService) Preview(ctx context.Context) (map[string]interface{}, error) {
	now := time.Now().UTC()
	state, err := s.loadOrInitState(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed resolving telemetry state: %w", err)
	}

	metrics, err := s.collector.Collect(ctx, CollectInput{
		FirstSeenAt: state.FirstSeenAt,
	})
	if err != nil {
		return nil, fmt.Errorf("failed compiling telemetry metrics: %w", err)
	}

	return s.buildPayload(state.InstanceID, now, metrics), nil
}

func (s *SelfHostedBeaconService) loadOrInitState(ctx context.Context) (*TelemetryStateValue, error) {
	if s.store == nil {
		return &TelemetryStateValue{
			InstanceID:  uuid.NewString(),
			FirstSeenAt: time.Now().UTC().Format(time.RFC3339Nano),
		}, nil
	}

	existing, err := s.store.GetState(ctx)
	if err == nil && existing != nil && existing.InstanceID != "" && existing.FirstSeenAt != "" {
		return existing, nil
	}

	fresh := &TelemetryStateValue{
		InstanceID:  uuid.NewString(),
		FirstSeenAt: time.Now().UTC().Format(time.RFC3339Nano),
	}

	if err := s.store.SetState(ctx, fresh); err != nil {
		s.logger.Warn("Failed saving initial telemetry state to persistent store", "error", err)
	}
	return fresh, nil
}

func (s *SelfHostedBeaconService) clearInFlightClaim(ctx context.Context, state *TelemetryStateValue) error {
	state.InFlightDay = nil
	state.InFlightStartedAt = nil
	return s.store.SetState(ctx, state)
}

func (s *SelfHostedBeaconService) buildPayload(instanceID string, sentAt time.Time, metrics *HeartbeatMetrics) map[string]interface{} {
	payload := map[string]interface{}{
		"schema_version": 1,
		"instance_id":    instanceID,
		"sent_at":        sentAt.Format(time.RFC3339Nano),
		"scandrix":       metrics.ScanDrix,
		"runtime":        metrics.Runtime,
		"usage_7d":       metrics.Usage7d,
		"config":         metrics.Config,
	}
	return payload
}

func hasFreshInFlightClaim(state *TelemetryStateValue, today string, now time.Time) bool {
	if state.InFlightDay == nil || *state.InFlightDay != today {
		return false
	}
	if state.InFlightStartedAt == nil || *state.InFlightStartedAt == "" {
		return false
	}
	startedAt, err := time.Parse(time.RFC3339Nano, *state.InFlightStartedAt)
	if err != nil {
		startedAt, err = time.Parse(time.RFC3339, *state.InFlightStartedAt)
	}
	if err != nil {
		return false
	}
	// Expire after 30 minutes
	return now.Sub(startedAt) < 30*time.Minute
}

// ═══════════════════════════════════════════════════════════════
// Background Cron Job Adapter
// ═══════════════════════════════════════════════════════════════

// BeaconCronJob implements the cron.CronJob interface to trigger daily heartbeats.
type BeaconCronJob struct {
	beacon ISelfHostedBeaconService
	logger *slog.Logger
}

// NewBeaconCronJob creates a cron task and outputs the boot transparency log.
func NewBeaconCronJob(beacon ISelfHostedBeaconService, logger *slog.Logger) *BeaconCronJob {
	if logger == nil {
		logger = slog.Default()
	}
	job := &BeaconCronJob{
		beacon: beacon,
		logger: logger.With("component", "beacon_cron_job"),
	}
	job.logBootTransparency()
	return job
}

func (b *BeaconCronJob) logBootTransparency() {
	defer func() { _ = recover() }()

	if b.beacon.IsDisabled() {
		b.logger.Info("Anonymous usage telemetry is DISABLED (SCANDRIX_TELEMETRY_DISABLED is set). No heartbeat will be sent.")
		return
	}

	b.logger.Info("Anonymous usage telemetry is enabled. One heartbeat per UTC day to telemetry.scandrix.dev with aggregated counters only — no code, names, or identifiers. Inspect with `scandrix telemetry preview`. Disable with SCANDRIX_TELEMETRY_DISABLED=true.")
}

// Name identifies the task in the background scheduler.
func (b *BeaconCronJob) Name() string {
	return "self-hosted-beacon"
}

// Interval specifies the recurrence period (24 hours).
func (b *BeaconCronJob) Interval() time.Duration {
	return 24 * time.Hour
}

// Run executes the heartbeat transmission.
func (b *BeaconCronJob) Run(ctx context.Context) error {
	start := time.Now()
	err := b.beacon.Run(ctx)
	b.logger.Info("Self-hosted beacon execution completed", "duration", time.Since(start))
	return err
}

// MarshalPayloadToJSON is a convenience helper for formatting preview output.
func MarshalPayloadToJSON(payload map[string]interface{}) ([]byte, error) {
	return json.MarshalIndent(payload, "", "  ")
}
