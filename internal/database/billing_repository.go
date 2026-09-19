// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Database Access Layer
// File: billing_repository.go
// ═══════════════════════════════════════════════════════════════

package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/scandrix/backend/internal/enterprise/license"
	"github.com/scandrix/backend/pkg/models"
)

// RecordTokenUsage logs token consumption for billing and usage caps.
func (r *Repository) RecordTokenUsage(ctx context.Context, wsID uuid.UUID, reviewID *uuid.UUID, promptTokens, completionTokens int64, costUSD float64) error {
	if r == nil || r.client == nil {
		return nil
	}

	query := `
		INSERT INTO token_usage_records (id, workspace_id, review_id, prompt_tokens, completion_tokens, cost_usd, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, now());
	`
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, uuid.New(), wsID, reviewID, promptTokens, completionTokens, costUSD)
		return err
	})
}


// GetWorkspaceUsage calculates aggregated token usage and cost for a given time window.
func (r *Repository) GetWorkspaceUsage(ctx context.Context, wsID uuid.UUID, since time.Time) (promptTokens, completionTokens int64, costUSD float64, err error) {
	if r == nil || r.client == nil {
		return 0, 0, 0, nil
	}

	query := `
		SELECT COALESCE(SUM(prompt_tokens), 0), COALESCE(SUM(completion_tokens), 0), COALESCE(SUM(cost_usd), 0)
		FROM token_usage_records
		WHERE workspace_id = $1 AND created_at >= $2;
	`
	err = r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, wsID, since).Scan(&promptTokens, &completionTokens, &costUSD)
	})
	return promptTokens, completionTokens, costUSD, err
}


// UpdateSpendLimit updates the monthly spend limit for a workspace.
func (r *Repository) UpdateSpendLimit(ctx context.Context, wsID uuid.UUID, limitUSD float64) error {
	if r == nil || r.client == nil {
		return nil
	}

	query := `
		INSERT INTO workspace_spend_limits (workspace_id, monthly_spend_limit_usd, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (workspace_id) DO UPDATE
		SET monthly_spend_limit_usd = EXCLUDED.monthly_spend_limit_usd, updated_at = now();
	`
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, wsID, limitUSD)
		return err
	})
}


// GetSpendLimit retrieves the configured monthly spend limit.
func (r *Repository) GetSpendLimit(ctx context.Context, wsID uuid.UUID) (float64, error) {
	if r == nil || r.client == nil {
		return 50.00, nil
	}

	query := `SELECT monthly_spend_limit_usd FROM workspace_spend_limits WHERE workspace_id = $1;`
	var limit float64
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, wsID).Scan(&limit)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 50.00, nil
		}
		return 0, err
	}
	return limit, nil
}


// GetActiveLicense retrieves the active enterprise license for a workspace.
func (r *Repository) GetActiveLicense(ctx context.Context, wsID uuid.UUID) (*models.OrganizationLicense, error) {
	if r == nil || r.client == nil {
		return nil, nil
	}

	query := `
		SELECT id, workspace_id, license_key, organization_name, plan_tier, total_seats, allocated_seats, expires_at, is_air_gapped, features_enabled, activated_at, updated_at
		FROM organization_licenses
		WHERE workspace_id = $1
		ORDER BY activated_at DESC
		LIMIT 1;
	`
	var lic models.OrganizationLicense
	var featuresJSON []byte
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, wsID).Scan(
			&lic.ID, &lic.WorkspaceID, &lic.LicenseKey, &lic.OrganizationName,
			&lic.PlanTier, &lic.TotalSeats, &lic.AllocatedSeats, &lic.ExpiresAt,
			&lic.IsAirGapped, &featuresJSON, &lic.ActivatedAt, &lic.UpdatedAt,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	_ = json.Unmarshal(featuresJSON, &lic.FeaturesEnabled)
	return &lic, nil
}


// ActivateLicense stores or updates an enterprise license in PostgreSQL.
func (r *Repository) ActivateLicense(ctx context.Context, wsID uuid.UUID, licenseKey, orgName, planTier string, totalSeats int, expiresAt time.Time, features []string) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("database unavailable")
	}

	featuresJSON, _ := json.Marshal(features)
	query := `
		INSERT INTO organization_licenses (id, workspace_id, license_key, organization_name, plan_tier, total_seats, allocated_seats, expires_at, is_air_gapped, features_enabled, activated_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 1, $7, false, $8, now(), now())
		ON CONFLICT (license_key) DO UPDATE
		SET total_seats = EXCLUDED.total_seats, expires_at = EXCLUDED.expires_at, features_enabled = EXCLUDED.features_enabled, updated_at = now();
	`
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, uuid.New(), wsID, licenseKey, orgName, planTier, totalSeats, expiresAt, string(featuresJSON))
		return err
	})

}


// RecordBillingTransaction stores a checkout or webhook transaction in PostgreSQL.
func (r *Repository) RecordBillingTransaction(ctx context.Context, tx *models.BillingTransaction) error {
	if r == nil || r.client == nil {
		return nil
	}
	if tx.ID == uuid.Nil {
		tx.ID = uuid.New()
	}

	query := `
		INSERT INTO billing_transactions (
			id, workspace_id, provider, order_id, payment_id, signature,
			amount, currency, plan_tier, status, receipt, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now(), now())
		ON CONFLICT (id) DO UPDATE SET
			payment_id = EXCLUDED.payment_id,
			signature = EXCLUDED.signature,
			status = EXCLUDED.status,
			updated_at = now();
	`
	return r.client.ExecWithTenant(ctx, tx.WorkspaceID, func(pgTx pgx.Tx) error {
		_, err := pgTx.Exec(ctx, query,
			tx.ID, tx.WorkspaceID, tx.Provider, tx.OrderID, tx.PaymentID, tx.Signature,
			tx.Amount, tx.Currency, tx.PlanTier, tx.Status, tx.Receipt,
		)
		return err
	})
}


// UpdateBillingTransactionStatus updates the status and payment ID of an order.
func (r *Repository) UpdateBillingTransactionStatus(ctx context.Context, wsID uuid.UUID, orderID, paymentID, signature, status string) error {
	if r == nil || r.client == nil {
		return nil
	}

	query := `
		UPDATE billing_transactions
		SET payment_id = COALESCE(NULLIF($1, ''), payment_id),
		    signature = COALESCE(NULLIF($2, ''), signature),
		    status = $3,
		    updated_at = now()
		WHERE workspace_id = $4 AND order_id = $5;
	`
	return r.client.ExecWithTenant(ctx, wsID, func(pgTx pgx.Tx) error {
		_, err := pgTx.Exec(ctx, query, paymentID, signature, status, wsID, orderID)
		return err
	})
}


// UpgradeWorkspacePlan updates the active organization license and seat table to reflect a purchased plan.
func (r *Repository) UpgradeWorkspacePlan(ctx context.Context, wsID uuid.UUID, planTier string, maxSeats int, expiresAt time.Time, features []string) error {
	if r == nil || r.client == nil {
		return nil
	}

	featuresJSON, _ := json.Marshal(features)
	licenseKey := fmt.Sprintf("SUB-%s-%s", strings.ToUpper(planTier), wsID.String()[:8])

	queryLicense := `
		INSERT INTO organization_licenses (
			id, workspace_id, license_key, organization_name, plan_tier,
			total_seats, allocated_seats, expires_at, is_air_gapped, features_enabled,
			activated_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, 1, $7, false, $8, now(), now())
		ON CONFLICT (license_key) DO UPDATE SET
			plan_tier = EXCLUDED.plan_tier,
			total_seats = EXCLUDED.total_seats,
			expires_at = EXCLUDED.expires_at,
			features_enabled = EXCLUDED.features_enabled,
			updated_at = now();
	`

	querySeats := `
		INSERT INTO organization_billing_seats (
			id, workspace_id, tier, max_seats, allocated_seats, byok_enabled, dora_enabled, active_until, created_at, updated_at
		) VALUES ($1, $2, $3, $4, 1, true, true, $5, now(), now())
		ON CONFLICT (id) DO NOTHING;
	`

	return r.client.ExecWithTenant(ctx, wsID, func(pgTx pgx.Tx) error {
		if _, err := pgTx.Exec(ctx, queryLicense, uuid.New(), wsID, licenseKey, planTier+" Plan", planTier, maxSeats, expiresAt, string(featuresJSON)); err != nil {
			return err
		}
		_, err := pgTx.Exec(ctx, querySeats, uuid.New(), wsID, planTier, maxSeats, expiresAt)
		return err
	})
}


// UpgradeWorkspacePlanAtomic atomically marks a billing transaction captured and elevates the organization license and seat entitlements within a single tenant transaction.
func (r *Repository) UpgradeWorkspacePlanAtomic(ctx context.Context, wsID uuid.UUID, orderID, paymentID, signature, planTier string, maxSeats int, expiresAt time.Time, features []string) error {
	if r == nil || r.client == nil {
		return nil
	}

	featuresJSON, _ := json.Marshal(features)
	licenseKey := fmt.Sprintf("SUB-%s-%s", strings.ToUpper(planTier), wsID.String()[:8])

	queryTx := `
		UPDATE billing_transactions
		SET payment_id = COALESCE(NULLIF($1, ''), payment_id),
		    signature = COALESCE(NULLIF($2, ''), signature),
		    status = 'captured',
		    updated_at = now()
		WHERE workspace_id = $3 AND order_id = $4;
	`

	queryLicense := `
		INSERT INTO organization_licenses (
			id, workspace_id, license_key, organization_name, plan_tier,
			total_seats, allocated_seats, expires_at, is_air_gapped, features_enabled,
			activated_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, 1, $7, false, $8, now(), now())
		ON CONFLICT (license_key) DO UPDATE SET
			plan_tier = EXCLUDED.plan_tier,
			total_seats = EXCLUDED.total_seats,
			expires_at = EXCLUDED.expires_at,
			features_enabled = EXCLUDED.features_enabled,
			updated_at = now();
	`

	querySeats := `
		INSERT INTO organization_billing_seats (
			id, workspace_id, tier, max_seats, allocated_seats, byok_enabled, dora_enabled, active_until, created_at, updated_at
		) VALUES ($1, $2, $3, $4, 1, true, true, $5, now(), now())
		ON CONFLICT (id) DO NOTHING;
	`

	return r.client.ExecWithTenant(ctx, wsID, func(pgTx pgx.Tx) error {
		if orderID != "" {
			tag, err := pgTx.Exec(ctx, queryTx, paymentID, signature, wsID, orderID)
			if err != nil {
				return fmt.Errorf("failed updating billing_transactions status to captured: %w", err)
			}
			if tag.RowsAffected() == 0 {
				slog.Warn("Atomic plan upgrade could not find existing billing transaction for order", "workspace_id", wsID, "order_id", orderID)
			}
		}

		if _, err := pgTx.Exec(ctx, queryLicense, uuid.New(), wsID, licenseKey, planTier+" Plan", planTier, maxSeats, expiresAt, string(featuresJSON)); err != nil {
			return fmt.Errorf("failed upserting organization_licenses: %w", err)
		}

		if _, err := pgTx.Exec(ctx, querySeats, uuid.New(), wsID, planTier, maxSeats, expiresAt); err != nil {
			return fmt.Errorf("failed upserting organization_billing_seats: %w", err)
		}

		return nil
	})
}


// GetPlanConfiguration fetches dynamic plan pricing and quotas from PostgreSQL.
func (r *Repository) GetPlanConfiguration(ctx context.Context, tier string) (*models.PlanConfiguration, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, nil
	}

	normTier := string(license.NormalizeTier(license.LicenseTier(tier)))

	query := `
		SELECT tier, display_name, amount_inr, amount_usd, monthly_tokens,
		       burst_limit_per_min, max_seats, max_repositories, max_concurrent_reviews,
		       byok_allowed, allocated_models, features_enabled, created_at, updated_at
		FROM plan_configurations
		WHERE tier = $1;
	`
	var plan models.PlanConfiguration
	var modelsJSON, featuresJSON []byte

	err := r.client.Pool.QueryRow(ctx, query, normTier).Scan(
		&plan.Tier, &plan.DisplayName, &plan.AmountINR, &plan.AmountUSD,
		&plan.MonthlyTokens, &plan.BurstLimitPerMin, &plan.MaxSeats,
		&plan.MaxRepositories, &plan.MaxConcurrentReviews, &plan.BYOKAllowed,
		&modelsJSON, &featuresJSON, &plan.CreatedAt, &plan.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	_ = json.Unmarshal(modelsJSON, &plan.AllocatedModels)
	_ = json.Unmarshal(featuresJSON, &plan.FeaturesEnabled)
	return &plan, nil
}


// ListPlanConfigurations returns all active subscription plans from PostgreSQL.
func (r *Repository) ListPlanConfigurations(ctx context.Context) ([]models.PlanConfiguration, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, nil
	}

	query := `
		SELECT tier, display_name, amount_inr, amount_usd, monthly_tokens,
		       burst_limit_per_min, max_seats, max_repositories, max_concurrent_reviews,
		       byok_allowed, allocated_models, features_enabled, created_at, updated_at
		FROM plan_configurations
		ORDER BY amount_inr ASC;
	`
	rows, err := r.client.Pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plans []models.PlanConfiguration
	for rows.Next() {
		var plan models.PlanConfiguration
		var modelsJSON, featuresJSON []byte
		if err := rows.Scan(
			&plan.Tier, &plan.DisplayName, &plan.AmountINR, &plan.AmountUSD,
			&plan.MonthlyTokens, &plan.BurstLimitPerMin, &plan.MaxSeats,
			&plan.MaxRepositories, &plan.MaxConcurrentReviews, &plan.BYOKAllowed,
			&modelsJSON, &featuresJSON, &plan.CreatedAt, &plan.UpdatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(modelsJSON, &plan.AllocatedModels)
		_ = json.Unmarshal(featuresJSON, &plan.FeaturesEnabled)
		plans = append(plans, plan)
	}
	return plans, nil
}


// GetWorkspacePlanDetails queries the real license, database-configured quotas, and aggregated token consumption.
func (r *Repository) GetWorkspacePlanDetails(ctx context.Context, wsID uuid.UUID) (*models.WorkspacePlanDetails, error) {
	if r == nil || r.client == nil {
		return nil, nil
	}

	// 1. Fetch active license
	lic, err := r.GetActiveLicense(ctx, wsID)
	if err != nil {
		return nil, err
	}

	activeTier := "COMMUNITY"
	orgName := "Community Workspace"
	totalSeats := 5
	allocatedSeats := 1
	expiresAt := time.Now().AddDate(10, 0, 0)
	features := []string{"automated_reviews", "custom_rules"}
	subscriptionStatus := "ACTIVE"
	isExpired := false

	if lic != nil {
		rawTier := strings.ToUpper(lic.PlanTier)
		switch rawTier {
		case "PLUS", "PRO", "TEAMS", "STARTER":
			rawTier = "TEAM"
		case "CUSTOM", "ENT":
			rawTier = "ENTERPRISE"
		default:
			rawTier = "COMMUNITY"
		}

		orgName = lic.OrganizationName
		totalSeats = lic.TotalSeats
		allocatedSeats = lic.AllocatedSeats
		expiresAt = lic.ExpiresAt
		features = lic.FeaturesEnabled

		now := time.Now().UTC()
		if rawTier != "COMMUNITY" && !lic.ExpiresAt.IsZero() && now.After(lic.ExpiresAt) {
			isExpired = true
			if now.Before(lic.ExpiresAt.Add(72 * time.Hour)) {
				// 72-hour grace period: keep tier features active, but flag grace period in UI
				subscriptionStatus = "GRACE_PERIOD"
				activeTier = rawTier
			} else {
				// Grace period exceeded: automatically downgrade to COMMUNITY tier quotas
				subscriptionStatus = "EXPIRED"
				activeTier = "COMMUNITY"
			}
		} else {
			activeTier = rawTier
		}
	}

	// 2. Fetch database plan configuration (DB as Single Source of Truth)
	planConfig, _ := r.GetPlanConfiguration(ctx, activeTier)
	defaultQuota := license.GetPlanQuota(license.LicenseTier(activeTier))
	monthlyLimit := defaultQuota.MonthlyTokens
	burstLimit := defaultQuota.BurstLimitPerMin
	maxConcurrent := defaultQuota.MaxConcurrentReviews
	allocatedModels := license.GetAllocatedModelsList(license.LicenseTier(activeTier))

	if planConfig != nil {
		monthlyLimit = planConfig.MonthlyTokens
		burstLimit = planConfig.BurstLimitPerMin
		allocatedModels = planConfig.AllocatedModels
		maxConcurrent = planConfig.MaxConcurrentReviews
		if len(planConfig.FeaturesEnabled) > 0 {
			features = planConfig.FeaturesEnabled
		}
	}

	// 3. Query real token consumption from PostgreSQL token_usage_records table
	startOfMonth := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.UTC)
	startOfMinute := time.Now().Add(-1 * time.Minute)

	promptMonth, compMonth, _, _ := r.GetWorkspaceUsage(ctx, wsID, startOfMonth)
	promptMin, compMin, _, _ := r.GetWorkspaceUsage(ctx, wsID, startOfMinute)

	return &models.WorkspacePlanDetails{
		WorkspaceID:          wsID,
		PlanTier:             activeTier,
		OrganizationName:     orgName,
		TotalSeats:           totalSeats,
		AllocatedSeats:       allocatedSeats,
		ExpiresAt:            expiresAt,
		MonthlyTokenLimit:    monthlyLimit,
		MonthlyTokensUsed:    promptMonth + compMonth,
		BurstLimitPerMin:     burstLimit,
		BurstTokensUsed:      promptMin + compMin,
		AllocatedModels:      allocatedModels,
		FeaturesEnabled:      features,
		MaxConcurrentReviews: maxConcurrent,
		BYOKAllowed:          true,
		SubscriptionStatus:   subscriptionStatus,
		IsExpired:            isExpired,
	}, nil
}


// PruneInactiveLicenseSeats downgrades inactive members to VIEWER to reclaim allocated seats.
func (r *Repository) PruneInactiveLicenseSeats(ctx context.Context, inactivityDays int) (int64, error) {
	if r == nil || r.client == nil {
		return 0, nil
	}

	query := `
		UPDATE account_profiles
		SET role = 'VIEWER', updated_at = NOW()
		WHERE UPPER(role) NOT IN ('OWNER', 'ADMIN', 'VIEWER')
		  AND COALESCE(last_active_at, updated_at) < NOW() - ($1 || ' days')::interval;
	`
	var rowsAffected int64
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, query, fmt.Sprintf("%d", inactivityDays))
		if err != nil {
			return err
		}
		rowsAffected = tag.RowsAffected()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("failed pruning inactive license seats: %w", err)
	}
	return rowsAffected, nil
}


// GetBillingTransaction retrieves a billing transaction record by workspace and order ID.
func (r *Repository) GetBillingTransaction(ctx context.Context, wsID uuid.UUID, orderID string) (*models.BillingTransaction, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, nil
	}
	query := `
		SELECT id, workspace_id, provider, order_id, COALESCE(payment_id, ''), COALESCE(signature, ''),
		       amount, currency, plan_tier, status, COALESCE(receipt, ''), created_at, updated_at
		FROM billing_transactions
		WHERE workspace_id = $1 AND order_id = $2
		LIMIT 1;
	`
	var tx models.BillingTransaction
	err := r.client.ExecWithTenant(ctx, wsID, func(pgTx pgx.Tx) error {
		return pgTx.QueryRow(ctx, query, wsID, orderID).Scan(
			&tx.ID, &tx.WorkspaceID, &tx.Provider, &tx.OrderID, &tx.PaymentID, &tx.Signature,
			&tx.Amount, &tx.Currency, &tx.PlanTier, &tx.Status, &tx.Receipt, &tx.CreatedAt, &tx.UpdatedAt,
		)
	})
	if err != nil {
		return nil, err
	}
	return &tx, nil
}


// ClaimBillingUpgrade atomically claims an order for upgrade processing, preventing duplicate webhook/checkout races.
func (r *Repository) ClaimBillingUpgrade(ctx context.Context, wsID uuid.UUID, orderID string) (bool, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return true, nil
	}
	query := `
		UPDATE billing_transactions
		SET status = 'processing_upgrade', updated_at = NOW()
		WHERE workspace_id = $1 AND order_id = $2 AND status != 'captured' AND status != 'processing_upgrade';
	`
	var rowsAffected int64
	err := r.client.ExecWithTenant(ctx, wsID, func(pgTx pgx.Tx) error {
		tag, err := pgTx.Exec(ctx, query, wsID, orderID)
		if err == nil {
			rowsAffected = tag.RowsAffected()
		}
		return err
	})
	return rowsAffected > 0, err
}


// ListPendingReconciliationTransactions retrieves billing transactions stuck in 'created' or 'processing_upgrade' older than the specified duration for automated ledger reconciliation.
func (r *Repository) ListPendingReconciliationTransactions(ctx context.Context, olderThan time.Duration, limit int) ([]models.BillingTransaction, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	cutoff := time.Now().UTC().Add(-olderThan)
	query := `
		SELECT id, workspace_id, provider, order_id, COALESCE(payment_id, ''), COALESCE(signature, ''),
		       amount, currency, plan_tier, status, COALESCE(receipt, ''), created_at, updated_at
		FROM billing_transactions
		WHERE (status = 'created' OR status = 'processing_upgrade')
		  AND created_at <= $1
		ORDER BY created_at ASC
		LIMIT $2;
	`
	var transactions []models.BillingTransaction
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, cutoff, limit)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var t models.BillingTransaction
			if err := rows.Scan(
				&t.ID, &t.WorkspaceID, &t.Provider, &t.OrderID, &t.PaymentID, &t.Signature,
				&t.Amount, &t.Currency, &t.PlanTier, &t.Status, &t.Receipt, &t.CreatedAt, &t.UpdatedAt,
			); err != nil {
				return err
			}
			transactions = append(transactions, t)
		}
		return rows.Err()
	})
	return transactions, err
}


// ReconcileTransactionLedger records an automated reconciliation audit entry and updates the transaction status in the ledger.
func (r *Repository) ReconcileTransactionLedger(ctx context.Context, wsID uuid.UUID, orderID, paymentID, reconciledStatus, notes string) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil
	}
	query := `
		UPDATE billing_transactions
		SET status = $1, payment_id = CASE WHEN $2 != '' THEN $2 ELSE payment_id END, updated_at = NOW()
		WHERE workspace_id = $3 AND order_id = $4;
	`
	auditQuery := `
		INSERT INTO audit_logs (id, workspace_id, actor_email, actor_name, action, target_type, target_id, payload, created_at)
		VALUES ($1, $2, 'system:reconciliation', 'Ledger Reconciliation Engine', 'billing.reconcile', 'billing_transaction', $3, $4, NOW());
	`
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, query, reconciledStatus, paymentID, wsID, orderID); err != nil {
			return err
		}
		payload := fmt.Sprintf(`{"order_id":"%s","status":"%s","notes":"%s"}`, orderID, reconciledStatus, notes)
		_, err := tx.Exec(ctx, auditQuery, uuid.New(), wsID, orderID, []byte(payload))
		return err
	})
}


// SpendLimitEvaluation holds monthly usage and configured spend limit for a workspace.
type SpendLimitEvaluation struct {
	WorkspaceID       uuid.UUID `json:"workspace_id"`
	OrganizationName  string    `json:"organization_name"`
	MonthlySpendLimit float64   `json:"monthly_spend_limit"`
	CurrentSpendUSD   float64   `json:"current_spend_usd"`
	UsagePercentage   float64   `json:"usage_percentage"`
	TotalTokensUsed   int64     `json:"total_tokens_used"`
	OwnerEmail        string    `json:"owner_email"`
}


// GetWorkspacesSpendEvaluation evaluates current month spend against limits across all workspaces.
func (r *Repository) GetWorkspacesSpendEvaluation(ctx context.Context) ([]SpendLimitEvaluation, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return []SpendLimitEvaluation{}, nil
	}

	startOfMonth := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.UTC)

	query := `
		SELECT 
			w.id AS workspace_id,
			w.name AS organization_name,
			COALESCE(sl.monthly_spend_limit_usd, 50.0) AS spend_limit,
			COALESCE(SUM(t.cost_usd), 0.0) AS current_spend,
			COALESCE(SUM(t.prompt_tokens + t.completion_tokens), 0) AS total_tokens,
			COALESCE((
				SELECT email FROM account_profiles 
				WHERE workspace_id = w.id 
				ORDER BY CASE WHEN UPPER(role) = 'OWNER' THEN 1 WHEN UPPER(role) = 'ADMIN' THEN 2 ELSE 3 END 
				LIMIT 1
			), '') AS owner_email
		FROM workspaces w
		LEFT JOIN workspace_spend_limits sl ON sl.workspace_id = w.id
		LEFT JOIN token_usage_records t ON t.workspace_id = w.id AND t.created_at >= $1
		WHERE w.status = 'ACTIVE'
		GROUP BY w.id, w.name, sl.monthly_spend_limit_usd;
	`

	rows, err := r.client.Pool.Query(ctx, query, startOfMonth)
	if err != nil {
		return nil, fmt.Errorf("failed querying workspaces spend evaluation: %w", err)
	}
	defer rows.Close()

	var evaluations []SpendLimitEvaluation
	for rows.Next() {
		var e SpendLimitEvaluation
		if err := rows.Scan(
			&e.WorkspaceID, &e.OrganizationName, &e.MonthlySpendLimit,
			&e.CurrentSpendUSD, &e.TotalTokensUsed, &e.OwnerEmail,
		); err != nil {
			return nil, err
		}
		if e.MonthlySpendLimit > 0 {
			e.UsagePercentage = (e.CurrentSpendUSD / e.MonthlySpendLimit) * 100.0
		} else {
			e.UsagePercentage = 0
		}
		evaluations = append(evaluations, e)
	}
	return evaluations, nil
}


// LiveQuotaStatus represents multi-tenant token consumption and quota thresholds.
type LiveQuotaStatus struct {
	WorkspaceID         uuid.UUID `json:"workspace_id"`
	Tier                string    `json:"tier"`
	BYOKEnabled         bool      `json:"byok_enabled"`
	MonthlyTokenLimit   int64     `json:"monthly_token_limit"`
	TokensUsedThisMonth int64     `json:"tokens_used_this_month"`
	TokensRemaining     int64     `json:"tokens_remaining"`
	PercentUsed         float64   `json:"percent_used"`
	EstimatedCostUSD    float64   `json:"estimated_cost_usd"`
	BurstLimitPerMin    int64     `json:"burst_limit_per_min"`
	BillingPeriodStart  time.Time `json:"billing_period_start"`
	BillingPeriodEnd    time.Time `json:"billing_period_end"`
}


// GetLiveTokenQuota evaluates real-time token usage against active plan allocation and spend caps.
func (r *Repository) GetLiveTokenQuota(ctx context.Context, wsID uuid.UUID) (*LiveQuotaStatus, error) {
	now := time.Now().UTC()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	endOfMonth := startOfMonth.AddDate(0, 1, 0)

	if r == nil || r.client == nil {
		return &LiveQuotaStatus{
			WorkspaceID:         wsID,
			Tier:                "Free",
			BYOKEnabled:         false,
			MonthlyTokenLimit:   500_000,
			TokensUsedThisMonth: 0,
			TokensRemaining:     500_000,
			PercentUsed:         0,
			EstimatedCostUSD:    0,
			BurstLimitPerMin:    50_000,
			BillingPeriodStart:  startOfMonth,
			BillingPeriodEnd:    endOfMonth,
		}, nil
	}

	promptTokens, compTokens, costUSD, err := r.GetWorkspaceUsage(ctx, wsID, startOfMonth)
	if err != nil {
		return nil, err
	}
	totalUsed := promptTokens + compTokens

	// Query allocation and BYOK flag
	var tier string
	var byokEnabled bool
	allocQuery := `SELECT tier, byok_enabled FROM organization_billing_seats WHERE workspace_id = $1 LIMIT 1;`
	_ = r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, allocQuery, wsID).Scan(&tier, &byokEnabled)
	})

	if tier == "" {
		tier = "Free"
	}

	var monthlyLimit int64
	var burstLimit int64

	plan, err := r.GetPlanConfiguration(ctx, tier)
	if err == nil && plan != nil {
		monthlyLimit = plan.MonthlyTokens
		burstLimit = plan.BurstLimitPerMin
	} else {
		fallbackQuota := license.GetPlanQuota(license.LicenseTier(tier))
		monthlyLimit = fallbackQuota.MonthlyTokens
		burstLimit = fallbackQuota.BurstLimitPerMin
	}

	remaining := monthlyLimit - totalUsed
	if remaining < 0 {
		remaining = 0
	}

	pctUsed := 0.0
	if monthlyLimit > 0 {
		pctUsed = (float64(totalUsed) / float64(monthlyLimit)) * 100.0
	}

	return &LiveQuotaStatus{
		WorkspaceID:         wsID,
		Tier:                tier,
		BYOKEnabled:         byokEnabled,
		MonthlyTokenLimit:   monthlyLimit,
		TokensUsedThisMonth: totalUsed,
		TokensRemaining:     remaining,
		PercentUsed:         pctUsed,
		EstimatedCostUSD:    costUSD,
		BurstLimitPerMin:    burstLimit,
		BillingPeriodStart:  startOfMonth,
		BillingPeriodEnd:    endOfMonth,
	}, nil
}


// DailyUsageSummary holds daily token metrics for usage graphs.
type DailyUsageSummary struct {
	Date             string  `json:"date"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	CostUSD          float64 `json:"cost_usd"`
}


// GetWorkspaceDailyUsageHistory aggregates daily token metrics for a lookback window.
func (r *Repository) GetWorkspaceDailyUsageHistory(ctx context.Context, wsID uuid.UUID, days int) ([]DailyUsageSummary, error) {
	if r == nil || r.client == nil {
		return []DailyUsageSummary{}, nil
	}
	if days <= 0 || days > 90 {
		days = 30
	}

	since := time.Now().UTC().AddDate(0, 0, -days)
	query := `
		SELECT 
			TO_CHAR(created_at, 'YYYY-MM-DD') AS day,
			COALESCE(SUM(prompt_tokens), 0) AS p_tokens,
			COALESCE(SUM(completion_tokens), 0) AS c_tokens,
			COALESCE(SUM(cost_usd), 0.0) AS daily_cost
		FROM token_usage_records
		WHERE workspace_id = $1 AND created_at >= $2
		GROUP BY TO_CHAR(created_at, 'YYYY-MM-DD')
		ORDER BY day ASC;
	`

	var summaries []DailyUsageSummary
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, wsID, since)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var s DailyUsageSummary
			if err := rows.Scan(&s.Date, &s.PromptTokens, &s.CompletionTokens, &s.CostUSD); err != nil {
				return err
			}
			s.TotalTokens = s.PromptTokens + s.CompletionTokens
			summaries = append(summaries, s)
		}
		return rows.Err()
	})
	return summaries, err
}

