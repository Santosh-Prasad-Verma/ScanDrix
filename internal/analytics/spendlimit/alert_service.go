package spendlimit

import (
	"context"
	"time"
)

// SpendLimitAlertService evaluates organization month-to-date spend and emits threshold alerts and final notices.
type SpendLimitAlertService struct {
	configService        *SpendLimitConfigService
	notificationsService INotificationService
}

// NewSpendLimitAlertService creates a new alert service.
func NewSpendLimitAlertService(
	configService *SpendLimitConfigService,
	notificationsService INotificationService,
) *SpendLimitAlertService {
	return &SpendLimitAlertService{
		configService:        configService,
		notificationsService: notificationsService,
	}
}

// RunForOrganization evaluates spend for a single organization and emits any pending threshold or final alerts.
func (s *SpendLimitAlertService) RunForOrganization(
	ctx context.Context,
	orgID, teamID string,
	now time.Time,
) error {
	cfg, eval, err := s.configService.LoadAndEvaluate(ctx, orgID, teamID, now)
	if err != nil || cfg == nil || eval == nil {
		return err
	}

	periodKey := eval.PeriodKey

	var sentThresholds []int
	if cfg.ThresholdsSent != nil {
		sentThresholds = cfg.ThresholdsSent[periodKey]
	}

	var finalNoticeSent bool
	if cfg.FinalNoticeSent != nil {
		finalNoticeSent = cfg.FinalNoticeSent[periodKey]
	}

	decision := DecideSpendAlerts(
		struct {
			CrossedThresholds []int
			IsOverLimit       bool
		}{
			CrossedThresholds: eval.CrossedThresholds,
			IsOverLimit:       eval.IsOverLimit,
		},
		SpendAlertState{
			ThresholdsSent:  sentThresholds,
			FinalNoticeSent: finalNoticeSent,
		},
	)

	// Emit threshold alerts
	if s.notificationsService != nil {
		for _, pct := range decision.ThresholdsToAlert {
			_ = s.notificationsService.Emit(ctx, EventSpendLimitThresholdReached, orgID, NotificationPayload{
				Percentage:      pct,
				MonthlyLimitUSD: cfg.MonthlyLimitUSD,
				SpentUSD:        eval.SpentUSD,
				PeriodKey:       periodKey,
			})
		}

		if decision.SendFinalNotice {
			_ = s.notificationsService.Emit(ctx, EventSpendLimitExceededFinal, orgID, NotificationPayload{
				MonthlyLimitUSD: cfg.MonthlyLimitUSD,
				SpentUSD:        eval.SpentUSD,
				PeriodKey:       periodKey,
			})
		}
	}

	if decision.Changed {
		if cfg.ThresholdsSent == nil {
			cfg.ThresholdsSent = make(map[string][]int)
		}
		cfg.ThresholdsSent[periodKey] = decision.NextThresholdsSent

		if cfg.FinalNoticeSent == nil {
			cfg.FinalNoticeSent = make(map[string]bool)
		}
		cfg.FinalNoticeSent[periodKey] = decision.NextFinalNoticeSent

		return s.configService.SaveConfig(ctx, orgID, teamID, cfg)
	}

	return nil
}
