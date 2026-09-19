package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/scandrix/backend/internal/notifications/domain/contracts"
	"github.com/scandrix/backend/internal/notifications/domain/entities"
	"github.com/scandrix/backend/internal/notifications/domain/enums"
)

// PostgresNotificationDeliveryRepository stores and queries delivery audits in PostgreSQL.
type PostgresNotificationDeliveryRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresNotificationDeliveryRepository creates a delivery repository backed by pgxpool.
func NewPostgresNotificationDeliveryRepository(pool *pgxpool.Pool) *PostgresNotificationDeliveryRepository {
	return &PostgresNotificationDeliveryRepository{pool: pool}
}

// Create inserts a new notification delivery row.
func (r *PostgresNotificationDeliveryRepository) Create(ctx context.Context, del *entities.NotificationDelivery) error {
	query := `
		INSERT INTO notification_deliveries (
			uuid, organization_id, recipient_user_id, event, criticality, channel,
			title, body, cta_url, category, recipient_email, recipient_role,
			delivery_status, metadata, correlation_id, last_error, delivered_at,
			attempts, next_attempt_at, locked_at, locked_by, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17,
			$18, $19, $20, $21, $22, $23
		)
	`
	metaBytes, err := json.Marshal(del.Metadata)
	if err != nil {
		metaBytes = []byte("{}")
	}

	_, err = r.pool.Exec(ctx, query,
		del.UUID, del.OrganizationID, del.RecipientUserID, del.Event, del.Criticality, del.Channel,
		del.Title, del.Body, del.CtaURL, del.Category, del.RecipientEmail, del.RecipientRole,
		del.DeliveryStatus, metaBytes, del.CorrelationID, del.LastError, del.DeliveredAt,
		del.Attempts, del.NextAttemptAt, del.LockedAt, del.LockedBy, del.CreatedAt, del.UpdatedAt,
	)
	return err
}

// UpdateStatus updates the deliveryStatus, deliveredAt, and error fields.
func (r *PostgresNotificationDeliveryRepository) UpdateStatus(
	ctx context.Context,
	deliveryID uuid.UUID,
	status enums.DeliveryStatus,
	errorMsg *string,
) error {
	now := time.Now().UTC()
	var deliveredAt *time.Time
	if status == enums.StatusDelivered {
		deliveredAt = &now
	}

	query := `
		UPDATE notification_deliveries
		SET delivery_status = $1, delivered_at = $2, last_error = $3, locked_by = NULL, locked_at = NULL, updated_at = $4
		WHERE uuid = $5
	`
	_, err := r.pool.Exec(ctx, query, status, deliveredAt, errorMsg, now, deliveryID)
	return err
}

// ScheduleRetry increments attempts and sets nextAttemptAt while clearing locks.
func (r *PostgresNotificationDeliveryRepository) ScheduleRetry(
	ctx context.Context,
	deliveryID uuid.UUID,
	nextAttemptAt time.Time,
	errorMsg string,
) error {
	now := time.Now().UTC()
	query := `
		UPDATE notification_deliveries
		SET attempts = attempts + 1, next_attempt_at = $1, last_error = $2, locked_by = NULL, locked_at = NULL, updated_at = $3
		WHERE uuid = $4
	`
	_, err := r.pool.Exec(ctx, query, nextAttemptAt, errorMsg, now, deliveryID)
	return err
}

// ClaimRetryBatch locks a batch of ready retry rows using SELECT ... FOR UPDATE SKIP LOCKED.
func (r *PostgresNotificationDeliveryRepository) ClaimRetryBatch(
	ctx context.Context,
	limit int,
	lockedBy string,
) ([]*entities.NotificationDelivery, error) {
	now := time.Now().UTC()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	selectQuery := `
		SELECT uuid, organization_id, recipient_user_id, event, criticality, channel,
		       title, body, cta_url, category, recipient_email, recipient_role,
		       delivery_status, metadata, correlation_id, last_error, delivered_at,
		       attempts, next_attempt_at, locked_at, locked_by, created_at, updated_at
		FROM notification_deliveries
		WHERE delivery_status = 'pending'
		  AND attempts > 0
		  AND next_attempt_at <= $1
		  AND (locked_at IS NULL OR locked_at < $2)
		ORDER BY next_attempt_at ASC
		LIMIT $3
		FOR UPDATE SKIP LOCKED
	`
	staleLockCutoff := now.Add(-2 * time.Minute)
	rows, err := tx.Query(ctx, selectQuery, now, staleLockCutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*entities.NotificationDelivery
	var claimedIDs []uuid.UUID

	for rows.Next() {
		del := &entities.NotificationDelivery{}
		var metaBytes []byte
		err := rows.Scan(
			&del.UUID, &del.OrganizationID, &del.RecipientUserID, &del.Event, &del.Criticality, &del.Channel,
			&del.Title, &del.Body, &del.CtaURL, &del.Category, &del.RecipientEmail, &del.RecipientRole,
			&del.DeliveryStatus, &metaBytes, &del.CorrelationID, &del.LastError, &del.DeliveredAt,
			&del.Attempts, &del.NextAttemptAt, &del.LockedAt, &del.LockedBy, &del.CreatedAt, &del.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		if len(metaBytes) > 0 {
			_ = json.Unmarshal(metaBytes, &del.Metadata)
		}
		results = append(results, del)
		claimedIDs = append(claimedIDs, del.UUID)
	}

	if len(claimedIDs) > 0 {
		updateQuery := `
			UPDATE notification_deliveries
			SET locked_by = $1, locked_at = $2, updated_at = $2
			WHERE uuid = ANY($3)
		`
		_, err = tx.Exec(ctx, updateQuery, lockedBy, now, claimedIDs)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return results, nil
}

// FindByCorrelationID retrieves delivery logs by correlation ID.
func (r *PostgresNotificationDeliveryRepository) FindByCorrelationID(ctx context.Context, correlationID string) ([]*entities.NotificationDelivery, error) {
	query := `
		SELECT uuid, organization_id, recipient_user_id, event, criticality, channel,
		       title, body, cta_url, category, recipient_email, recipient_role,
		       delivery_status, metadata, correlation_id, last_error, delivered_at,
		       attempts, next_attempt_at, locked_at, locked_by, created_at, updated_at
		FROM notification_deliveries
		WHERE correlation_id = $1
		ORDER BY created_at ASC
	`
	return r.scanDeliveries(ctx, query, correlationID)
}

// FindByID retrieves a single delivery log by ID.
func (r *PostgresNotificationDeliveryRepository) FindByID(ctx context.Context, id uuid.UUID) (*entities.NotificationDelivery, error) {
	query := `
		SELECT uuid, organization_id, recipient_user_id, event, criticality, channel,
		       title, body, cta_url, category, recipient_email, recipient_role,
		       delivery_status, metadata, correlation_id, last_error, delivered_at,
		       attempts, next_attempt_at, locked_at, locked_by, created_at, updated_at
		FROM notification_deliveries
		WHERE uuid = $1
	`
	deliveries, err := r.scanDeliveries(ctx, query, id)
	if err != nil {
		return nil, err
	}
	if len(deliveries) == 0 {
		return nil, nil
	}
	return deliveries[0], nil
}

// FindByOrganization returns paginated deliveries for an organization.
func (r *PostgresNotificationDeliveryRepository) FindByOrganization(
	ctx context.Context,
	orgID uuid.UUID,
	limit, offset int,
) ([]*entities.NotificationDelivery, int, error) {
	countQuery := `SELECT COUNT(*) FROM notification_deliveries WHERE organization_id = $1`
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, orgID).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT uuid, organization_id, recipient_user_id, event, criticality, channel,
		       title, body, cta_url, category, recipient_email, recipient_role,
		       delivery_status, metadata, correlation_id, last_error, delivered_at,
		       attempts, next_attempt_at, locked_at, locked_by, created_at, updated_at
		FROM notification_deliveries
		WHERE organization_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
	deliveries, err := r.scanDeliveries(ctx, query, orgID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	return deliveries, total, nil
}

func (r *PostgresNotificationDeliveryRepository) scanDeliveries(ctx context.Context, sql string, args ...interface{}) ([]*entities.NotificationDelivery, error) {
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*entities.NotificationDelivery
	for rows.Next() {
		del := &entities.NotificationDelivery{}
		var metaBytes []byte
		err := rows.Scan(
			&del.UUID, &del.OrganizationID, &del.RecipientUserID, &del.Event, &del.Criticality, &del.Channel,
			&del.Title, &del.Body, &del.CtaURL, &del.Category, &del.RecipientEmail, &del.RecipientRole,
			&del.DeliveryStatus, &metaBytes, &del.CorrelationID, &del.LastError, &del.DeliveredAt,
			&del.Attempts, &del.NextAttemptAt, &del.LockedAt, &del.LockedBy, &del.CreatedAt, &del.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		if len(metaBytes) > 0 {
			_ = json.Unmarshal(metaBytes, &del.Metadata)
		}
		results = append(results, del)
	}
	return results, nil
}

// PostgresRoutingRuleRepository implements routing matrix persistence.
type PostgresRoutingRuleRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRoutingRuleRepository creates a routing rule repository.
func NewPostgresRoutingRuleRepository(pool *pgxpool.Pool) *PostgresRoutingRuleRepository {
	return &PostgresRoutingRuleRepository{pool: pool}
}

// FindByOrganization returns all customized routing rules for an organization.
func (r *PostgresRoutingRuleRepository) FindByOrganization(ctx context.Context, orgID uuid.UUID) ([]*entities.RoutingRule, error) {
	query := `
		SELECT uuid, organization_id, event, category, role, channels, created_at, updated_at
		FROM notification_routing_rules
		WHERE organization_id = $1
	`
	rows, err := r.pool.Query(ctx, query, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*entities.RoutingRule
	for rows.Next() {
		rule := &entities.RoutingRule{}
		var channelsBytes []byte
		err := rows.Scan(&rule.UUID, &rule.OrganizationID, &rule.Event, &rule.Category, &rule.Role, &channelsBytes, &rule.CreatedAt, &rule.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if len(channelsBytes) > 0 {
			_ = json.Unmarshal(channelsBytes, &rule.Channels)
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

// Resolve returns the rule matching (org, event, role) or falls back to (org, event, '*').
func (r *PostgresRoutingRuleRepository) Resolve(ctx context.Context, orgID uuid.UUID, event string, role string) (*entities.RoutingRule, error) {
	query := `
		SELECT uuid, organization_id, event, category, role, channels, created_at, updated_at
		FROM notification_routing_rules
		WHERE organization_id = $1 AND event = $2 AND role IN ($3, '*')
		ORDER BY CASE WHEN role = $3 THEN 0 ELSE 1 END
		LIMIT 1
	`
	row := r.pool.QueryRow(ctx, query, orgID, event, role)
	rule := &entities.RoutingRule{}
	var channelsBytes []byte
	err := row.Scan(&rule.UUID, &rule.OrganizationID, &rule.Event, &rule.Category, &rule.Role, &channelsBytes, &rule.CreatedAt, &rule.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if len(channelsBytes) > 0 {
		_ = json.Unmarshal(channelsBytes, &rule.Channels)
	}
	return rule, nil
}

// Upsert inserts or updates a single routing rule row.
func (r *PostgresRoutingRuleRepository) Upsert(ctx context.Context, rule *entities.RoutingRule) error {
	query := `
		INSERT INTO notification_routing_rules (uuid, organization_id, event, category, role, channels, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (organization_id, event, role)
		DO UPDATE SET channels = EXCLUDED.channels, updated_at = EXCLUDED.updated_at
	`
	channelsBytes, err := json.Marshal(rule.Channels)
	if err != nil {
		channelsBytes = []byte("{}")
	}
	now := time.Now().UTC()
	if rule.UUID == uuid.Nil {
		rule.UUID = uuid.New()
	}
	rule.UpdatedAt = now

	_, err = r.pool.Exec(ctx, query, rule.UUID, rule.OrganizationID, rule.Event, rule.Category, rule.Role, channelsBytes, rule.CreatedAt, rule.UpdatedAt)
	return err
}

// UpsertBatch saves multiple routing rules in a single transaction.
func (r *PostgresRoutingRuleRepository) UpsertBatch(ctx context.Context, rules []*entities.RoutingRule) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	query := `
		INSERT INTO notification_routing_rules (uuid, organization_id, event, category, role, channels, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (organization_id, event, role)
		DO UPDATE SET channels = EXCLUDED.channels, updated_at = EXCLUDED.updated_at
	`
	now := time.Now().UTC()
	for _, rule := range rules {
		if rule.UUID == uuid.Nil {
			rule.UUID = uuid.New()
		}
		rule.UpdatedAt = now
		channelsBytes, _ := json.Marshal(rule.Channels)
		_, err := tx.Exec(ctx, query, rule.UUID, rule.OrganizationID, rule.Event, rule.Category, rule.Role, channelsBytes, rule.CreatedAt, rule.UpdatedAt)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// DeleteByOrganization clears all rules for an organization.
func (r *PostgresRoutingRuleRepository) DeleteByOrganization(ctx context.Context, orgID uuid.UUID) (int64, error) {
	tag, err := r.pool.Exec(ctx, "DELETE FROM notification_routing_rules WHERE organization_id = $1", orgID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// DeleteByOrgEventRole removes a specific role rule override.
func (r *PostgresRoutingRuleRepository) DeleteByOrgEventRole(ctx context.Context, orgID uuid.UUID, event string, role string) (int64, error) {
	tag, err := r.pool.Exec(ctx, "DELETE FROM notification_routing_rules WHERE organization_id = $1 AND event = $2 AND role = $3", orgID, event, role)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// PostgresUserNotificationRepository manages user-facing in-app notifications.
type PostgresUserNotificationRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresUserNotificationRepository creates a user notification repository.
func NewPostgresUserNotificationRepository(pool *pgxpool.Pool) *PostgresUserNotificationRepository {
	return &PostgresUserNotificationRepository{pool: pool}
}

// Create inserts a user notification record.
func (r *PostgresUserNotificationRepository) Create(ctx context.Context, un *entities.UserNotification) error {
	query := `
		INSERT INTO user_notifications (uuid, user_id, delivery_id, read_at, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (delivery_id) DO NOTHING
	`
	_, err := r.pool.Exec(ctx, query, un.UUID, un.UserID, un.DeliveryID, un.ReadAt, un.CreatedAt)
	return err
}

// FindByUser lists user notifications joined with delivery metadata.
func (r *PostgresUserNotificationRepository) FindByUser(
	ctx context.Context,
	userID uuid.UUID,
	limit, offset int,
	unreadOnly bool,
) ([]*contracts.UserNotificationWithDelivery, int, error) {
	filterClause := ""
	if unreadOnly {
		filterClause = " AND un.read_at IS NULL"
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM user_notifications un WHERE un.user_id = $1%s", filterClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, userID).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`
		SELECT un.uuid, un.user_id, un.delivery_id, un.read_at, un.created_at,
		       nd.uuid, nd.event, nd.criticality, nd.title, nd.body, nd.cta_url, nd.category, nd.metadata, nd.created_at
		FROM user_notifications un
		JOIN notification_deliveries nd ON un.delivery_id = nd.uuid
		WHERE un.user_id = $1%s
		ORDER BY un.created_at DESC
		LIMIT $2 OFFSET $3
	`, filterClause)

	rows, err := r.pool.Query(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list []*contracts.UserNotificationWithDelivery
	for rows.Next() {
		item := &contracts.UserNotificationWithDelivery{}
		var metaBytes []byte
		err := rows.Scan(
			&item.UUID, &item.UserID, &item.DeliveryID, &item.ReadAt, &item.CreatedAt,
			&item.Delivery.UUID, &item.Delivery.Event, &item.Delivery.Criticality, &item.Delivery.Title,
			&item.Delivery.Body, &item.Delivery.CtaURL, &item.Delivery.Category, &metaBytes, &item.Delivery.CreatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		if len(metaBytes) > 0 {
			_ = json.Unmarshal(metaBytes, &item.Delivery.Metadata)
		}
		list = append(list, item)
	}

	return list, total, nil
}

// CountUnread counts unread notifications for a user.
func (r *PostgresUserNotificationRepository) CountUnread(ctx context.Context, userID uuid.UUID) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM user_notifications WHERE user_id = $1 AND read_at IS NULL", userID).Scan(&count)
	return count, err
}

// MarkAsRead marks a notification as read.
func (r *PostgresUserNotificationRepository) MarkAsRead(ctx context.Context, notificationID, userID uuid.UUID) error {
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, "UPDATE user_notifications SET read_at = $1 WHERE uuid = $2 AND user_id = $3", now, notificationID, userID)
	return err
}

// MarkAllAsRead marks all unread notifications for a user as read.
func (r *PostgresUserNotificationRepository) MarkAllAsRead(ctx context.Context, userID uuid.UUID) (int, error) {
	now := time.Now().UTC()
	tag, err := r.pool.Exec(ctx, "UPDATE user_notifications SET read_at = $1 WHERE user_id = $2 AND read_at IS NULL", now, userID)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// In-Memory Repository Implementations (For Fast Unit Testing & Staging)

// MemoryDeliveryRepository provides an in-memory delivery store for testing.
type MemoryDeliveryRepository struct {
	mu         sync.RWMutex
	deliveries map[uuid.UUID]*entities.NotificationDelivery
}

func NewMemoryDeliveryRepository() *MemoryDeliveryRepository {
	return &MemoryDeliveryRepository{deliveries: make(map[uuid.UUID]*entities.NotificationDelivery)}
}

func (m *MemoryDeliveryRepository) Create(ctx context.Context, del *entities.NotificationDelivery) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deliveries[del.UUID] = del
	return nil
}

func (m *MemoryDeliveryRepository) UpdateStatus(ctx context.Context, deliveryID uuid.UUID, status enums.DeliveryStatus, errorMsg *string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	del, ok := m.deliveries[deliveryID]
	if !ok {
		return fmt.Errorf("delivery not found")
	}
	del.DeliveryStatus = status
	del.LastError = errorMsg
	if status == enums.StatusDelivered {
		now := time.Now().UTC()
		del.DeliveredAt = &now
	}
	return nil
}

func (m *MemoryDeliveryRepository) ScheduleRetry(ctx context.Context, deliveryID uuid.UUID, nextAttemptAt time.Time, errorMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	del, ok := m.deliveries[deliveryID]
	if !ok {
		return fmt.Errorf("delivery not found")
	}
	del.Attempts++
	del.NextAttemptAt = &nextAttemptAt
	del.LastError = &errorMsg
	return nil
}

func (m *MemoryDeliveryRepository) ClaimRetryBatch(ctx context.Context, limit int, lockedBy string) ([]*entities.NotificationDelivery, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	var batch []*entities.NotificationDelivery
	for _, del := range m.deliveries {
		if del.DeliveryStatus == enums.StatusPending && del.Attempts > 0 && del.NextAttemptAt != nil && !del.NextAttemptAt.After(now) {
			del.LockedBy = &lockedBy
			del.LockedAt = &now
			batch = append(batch, del)
			if len(batch) >= limit {
				break
			}
		}
	}
	return batch, nil
}

func (m *MemoryDeliveryRepository) FindByCorrelationID(ctx context.Context, correlationID string) ([]*entities.NotificationDelivery, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*entities.NotificationDelivery
	for _, del := range m.deliveries {
		if del.CorrelationID == correlationID {
			list = append(list, del)
		}
	}
	return list, nil
}

func (m *MemoryDeliveryRepository) FindByID(ctx context.Context, id uuid.UUID) (*entities.NotificationDelivery, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.deliveries[id], nil
}

func (m *MemoryDeliveryRepository) FindByOrganization(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]*entities.NotificationDelivery, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var all []*entities.NotificationDelivery
	for _, del := range m.deliveries {
		if del.OrganizationID == orgID {
			all = append(all, del)
		}
	}
	total := len(all)
	if offset >= total {
		return []*entities.NotificationDelivery{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total, nil
}

// MemoryRoutingRuleRepository provides in-memory routing rule storage for tests.
type MemoryRoutingRuleRepository struct {
	mu    sync.RWMutex
	rules map[string]*entities.RoutingRule // key: org:event:role
}

func NewMemoryRoutingRuleRepository() *MemoryRoutingRuleRepository {
	return &MemoryRoutingRuleRepository{rules: make(map[string]*entities.RoutingRule)}
}

func (m *MemoryRoutingRuleRepository) FindByOrganization(ctx context.Context, orgID uuid.UUID) ([]*entities.RoutingRule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*entities.RoutingRule
	for _, r := range m.rules {
		if r.OrganizationID == orgID {
			list = append(list, r)
		}
	}
	return list, nil
}

func (m *MemoryRoutingRuleRepository) Resolve(ctx context.Context, orgID uuid.UUID, event string, role string) (*entities.RoutingRule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	specificKey := fmt.Sprintf("%s:%s:%s", orgID, event, role)
	if r, ok := m.rules[specificKey]; ok {
		return r, nil
	}
	wildcardKey := fmt.Sprintf("%s:%s:*", orgID, event)
	return m.rules[wildcardKey], nil
}

func (m *MemoryRoutingRuleRepository) Upsert(ctx context.Context, rule *entities.RoutingRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s:%s:%s", rule.OrganizationID, rule.Event, rule.Role)
	m.rules[key] = rule
	return nil
}

func (m *MemoryRoutingRuleRepository) UpsertBatch(ctx context.Context, rules []*entities.RoutingRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, rule := range rules {
		key := fmt.Sprintf("%s:%s:%s", rule.OrganizationID, rule.Event, rule.Role)
		m.rules[key] = rule
	}
	return nil
}

func (m *MemoryRoutingRuleRepository) DeleteByOrganization(ctx context.Context, orgID uuid.UUID) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var deleted int64
	for k, r := range m.rules {
		if r.OrganizationID == orgID {
			delete(m.rules, k)
			deleted++
		}
	}
	return deleted, nil
}

func (m *MemoryRoutingRuleRepository) DeleteByOrgEventRole(ctx context.Context, orgID uuid.UUID, event string, role string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s:%s:%s", orgID, event, role)
	if _, ok := m.rules[key]; ok {
		delete(m.rules, key)
		return 1, nil
	}
	return 0, nil
}

// MemoryUserNotificationRepository provides an in-memory user notification store.
type MemoryUserNotificationRepository struct {
	mu           sync.RWMutex
	notifs       map[uuid.UUID]*entities.UserNotification
	deliveryRepo contracts.NotificationDeliveryRepository
}

func NewMemoryUserNotificationRepository(deliveryRepo contracts.NotificationDeliveryRepository) *MemoryUserNotificationRepository {
	return &MemoryUserNotificationRepository{
		notifs:       make(map[uuid.UUID]*entities.UserNotification),
		deliveryRepo: deliveryRepo,
	}
}

func (m *MemoryUserNotificationRepository) Create(ctx context.Context, un *entities.UserNotification) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.notifs {
		if existing.DeliveryID == un.DeliveryID {
			return nil // idempotent
		}
	}
	m.notifs[un.UUID] = un
	return nil
}

func (m *MemoryUserNotificationRepository) FindByUser(ctx context.Context, userID uuid.UUID, limit, offset int, unreadOnly bool) ([]*contracts.UserNotificationWithDelivery, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var filtered []*entities.UserNotification
	for _, un := range m.notifs {
		if un.UserID == userID {
			if unreadOnly && un.ReadAt != nil {
				continue
			}
			filtered = append(filtered, un)
		}
	}

	total := len(filtered)
	if offset >= total {
		return []*contracts.UserNotificationWithDelivery{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}

	var results []*contracts.UserNotificationWithDelivery
	for _, un := range filtered[offset:end] {
		item := &contracts.UserNotificationWithDelivery{
			UUID:       un.UUID,
			UserID:     un.UserID,
			DeliveryID: un.DeliveryID,
			ReadAt:     un.ReadAt,
			CreatedAt:  un.CreatedAt,
		}
		if m.deliveryRepo != nil {
			del, _ := m.deliveryRepo.FindByID(ctx, un.DeliveryID)
			if del != nil {
				item.Delivery = contracts.DeliverySummary{
					UUID:        del.UUID,
					Event:       del.Event,
					Criticality: del.Criticality,
					Title:       del.Title,
					Body:        del.Body,
					CtaURL:      del.CtaURL,
					Category:    del.Category,
					Metadata:    del.Metadata,
					CreatedAt:   del.CreatedAt,
				}
			}
		}
		results = append(results, item)
	}

	return results, total, nil
}

func (m *MemoryUserNotificationRepository) CountUnread(ctx context.Context, userID uuid.UUID) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, un := range m.notifs {
		if un.UserID == userID && un.ReadAt == nil {
			count++
		}
	}
	return count, nil
}

func (m *MemoryUserNotificationRepository) MarkAsRead(ctx context.Context, notificationID, userID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	un, ok := m.notifs[notificationID]
	if !ok || un.UserID != userID {
		return nil
	}
	now := time.Now().UTC()
	un.ReadAt = &now
	return nil
}

func (m *MemoryUserNotificationRepository) MarkAllAsRead(ctx context.Context, userID uuid.UUID) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	updated := 0
	for _, un := range m.notifs {
		if un.UserID == userID && un.ReadAt == nil {
			un.ReadAt = &now
			updated++
		}
	}
	return updated, nil
}
