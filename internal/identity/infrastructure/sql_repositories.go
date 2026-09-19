package infrastructure

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/identity/domain"
)

// SQLUserRepository implements domain.UserRepository using a standard database/sql connection.
type SQLUserRepository struct {
	db *sql.DB
}

// NewSQLUserRepository creates a new SQLUserRepository.
func NewSQLUserRepository(db *sql.DB) *SQLUserRepository {
	return &SQLUserRepository{db: db}
}

func (r *SQLUserRepository) Find(ctx context.Context, filter map[string]any) ([]domain.User, error) {
	query := "SELECT id, email, name, organization_id, team_id, status, is_admin, avatar_url, created_at, updated_at FROM users WHERE deleted_at IS NULL"
	var args []any
	idx := 1

	for k, v := range filter {
		query += fmt.Sprintf(" AND %s = $%d", k, idx)
		args = append(args, v)
		idx++
	}
	query += " ORDER BY created_at DESC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		var u domain.User
		var idStr, statusStr string
		var orgID, teamID, avatar sql.NullString
		var createdAt, updatedAt time.Time

		err := rows.Scan(&idStr, &u.Email, &u.Name, &orgID, &teamID, &statusStr, &u.IsAdmin, &avatar, &createdAt, &updatedAt)
		if err != nil {
			return nil, err
		}

		u.UUID, _ = uuid.Parse(idStr)
		u.Status = domain.UserStatus(statusStr)
		if orgID.Valid {
			parsed, _ := uuid.Parse(orgID.String)
			u.OrganizationUUID = &parsed
		}
		if teamID.Valid {
			parsed, _ := uuid.Parse(teamID.String)
			u.TeamUUID = &parsed
		}
		if avatar.Valid {
			u.AvatarURL = &avatar.String
		}
		u.CreatedAt = createdAt
		u.UpdatedAt = updatedAt
		users = append(users, u)
	}

	return users, nil
}

func (r *SQLUserRepository) FindOne(ctx context.Context, filter map[string]any) (*domain.User, error) {
	users, err := r.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, errors.New("user not found")
	}
	return &users[0], nil
}

func (r *SQLUserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	return r.FindOne(ctx, map[string]any{"email": strings.ToLower(strings.TrimSpace(email))})
}

func (r *SQLUserRepository) FindByUUID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	return r.FindOne(ctx, map[string]any{"id": id.String()})
}

func (r *SQLUserRepository) Create(ctx context.Context, user domain.User) (*domain.User, error) {
	if user.UUID == uuid.Nil {
		user.UUID = uuid.New()
	}
	now := time.Now().UTC()
	user.CreatedAt = now
	user.UpdatedAt = now

	query := `
INSERT INTO users (id, email, name, organization_id, team_id, status, is_admin, avatar_url, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	var orgID, teamID, avatar any
	if user.OrganizationUUID != nil {
		orgID = user.OrganizationUUID.String()
	}
	if user.TeamUUID != nil {
		teamID = user.TeamUUID.String()
	}
	if user.AvatarURL != nil {
		avatar = *user.AvatarURL
	}

	status := string(user.Status)
	if status == "" {
		status = string(domain.UserStatusActive)
	}

	_, err := r.db.ExecContext(ctx, query,
		user.UUID.String(),
		strings.ToLower(strings.TrimSpace(user.Email)),
		user.Name,
		orgID,
		teamID,
		status,
		user.IsAdmin,
		avatar,
		now,
		now,
	)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *SQLUserRepository) Update(ctx context.Context, id uuid.UUID, updates map[string]any) (*domain.User, error) {
	if len(updates) == 0 {
		return r.FindByUUID(ctx, id)
	}

	setClauses := []string{"updated_at = NOW()"}
	var args []any
	idx := 1

	for k, v := range updates {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", k, idx))
		args = append(args, v)
		idx++
	}

	query := fmt.Sprintf("UPDATE users SET %s WHERE id = $%d AND deleted_at IS NULL", strings.Join(setClauses, ", "), idx)
	args = append(args, id.String())

	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return r.FindByUUID(ctx, id)
}

func (r *SQLUserRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := "UPDATE users SET deleted_at = NOW() WHERE id = $1"
	_, err := r.db.ExecContext(ctx, query, id.String())
	return err
}

func (r *SQLUserRepository) Count(ctx context.Context, filter map[string]any) (int, error) {
	query := "SELECT COUNT(*) FROM users WHERE deleted_at IS NULL"
	var args []any
	idx := 1

	for k, v := range filter {
		query += fmt.Sprintf(" AND %s = $%d", k, idx)
		args = append(args, v)
		idx++
	}

	var count int
	err := r.db.QueryRowContext(ctx, query, args...).Scan(&count)
	return count, err
}

// SQLAuthRepository implements domain.AuthRepository using SQL storage.
type SQLAuthRepository struct {
	db *sql.DB
}

// NewSQLAuthRepository creates a new SQLAuthRepository.
func NewSQLAuthRepository(db *sql.DB) *SQLAuthRepository {
	return &SQLAuthRepository{db: db}
}

func (r *SQLAuthRepository) SaveRefreshToken(ctx context.Context, session domain.AuthSession) (*domain.AuthSession, error) {
	if session.UUID == uuid.Nil {
		session.UUID = uuid.New()
	}
	now := time.Now().UTC()
	session.CreatedAt = now

	query := `
INSERT INTO auth (id, user_id, refresh_token, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (id) DO UPDATE SET refresh_token = EXCLUDED.refresh_token, updated_at = NOW()`

	_, err := r.db.ExecContext(ctx, query,
		session.UUID.String(),
		session.UserUUID.String(),
		session.RefreshToken,
		now,
		now,
	)
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *SQLAuthRepository) FindRefreshToken(ctx context.Context, token string) (*domain.AuthSession, error) {
	query := `SELECT id, user_id, refresh_token, created_at FROM auth WHERE refresh_token = $1`
	var s domain.AuthSession
	var idStr, userStr string
	err := r.db.QueryRowContext(ctx, query, token).Scan(&idStr, &userStr, &s.RefreshToken, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	s.UUID, _ = uuid.Parse(idStr)
	s.UserUUID, _ = uuid.Parse(userStr)
	return &s, nil
}

func (r *SQLAuthRepository) UpdateRefreshToken(ctx context.Context, session domain.AuthSession) (*domain.AuthSession, error) {
	query := `UPDATE auth SET refresh_token = $1, updated_at = NOW() WHERE user_id = $2`
	_, err := r.db.ExecContext(ctx, query, session.RefreshToken, session.UserUUID.String())
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *SQLAuthRepository) DeactivateRefreshToken(ctx context.Context, userUUID uuid.UUID) error {
	query := `UPDATE auth SET refresh_token = NULL, updated_at = NOW() WHERE user_id = $1`
	_, err := r.db.ExecContext(ctx, query, userUUID.String())
	return err
}

// SQLCLIAuthSessionRepository implements domain.CliAuthSessionRepository.
type SQLCLIAuthSessionRepository struct {
	db *sql.DB
}

// NewSQLCLIAuthSessionRepository creates a new SQLCLIAuthSessionRepository.
func NewSQLCLIAuthSessionRepository(db *sql.DB) *SQLCLIAuthSessionRepository {
	return &SQLCLIAuthSessionRepository{db: db}
}

func (r *SQLCLIAuthSessionRepository) Create(ctx context.Context, session domain.CliAuthSession) (*domain.CliAuthSession, error) {
	if session.UUID == uuid.Nil {
		session.UUID = uuid.New()
	}
	now := time.Now().UTC()
	session.CreatedAt = now

	query := `
INSERT INTO cli_auth_sessions (id, session_id, user_code, device_code, status, expires_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	var uCode, dCode any
	if session.UserCode != nil {
		uCode = *session.UserCode
	}
	if session.DeviceCode != nil {
		dCode = *session.DeviceCode
	}

	_, err := r.db.ExecContext(ctx, query,
		session.UUID.String(),
		session.SessionID,
		uCode,
		dCode,
		string(session.Status),
		session.ExpiresAt,
		now,
		now,
	)
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *SQLCLIAuthSessionRepository) FindByState(ctx context.Context, state string) (*domain.CliAuthSession, error) {
	query := `SELECT id, session_id, user_code, device_code, status, expires_at, created_at FROM cli_auth_sessions WHERE session_id = $1`
	return r.scanSession(r.db.QueryRowContext(ctx, query, state))
}

func (r *SQLCLIAuthSessionRepository) FindByDeviceCode(ctx context.Context, code string) (*domain.CliAuthSession, error) {
	query := `SELECT id, session_id, user_code, device_code, status, expires_at, created_at FROM cli_auth_sessions WHERE device_code = $1`
	return r.scanSession(r.db.QueryRowContext(ctx, query, code))
}

func (r *SQLCLIAuthSessionRepository) FindByUserCode(ctx context.Context, code string) (*domain.CliAuthSession, error) {
	query := `SELECT id, session_id, user_code, device_code, status, expires_at, created_at FROM cli_auth_sessions WHERE user_code = $1`
	return r.scanSession(r.db.QueryRowContext(ctx, query, strings.ToUpper(strings.TrimSpace(code))))
}

func (r *SQLCLIAuthSessionRepository) scanSession(row *sql.Row) (*domain.CliAuthSession, error) {
	var s domain.CliAuthSession
	var idStr, statusStr string
	var uCode, dCode sql.NullString

	err := row.Scan(&idStr, &s.SessionID, &uCode, &dCode, &statusStr, &s.ExpiresAt, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	s.UUID, _ = uuid.Parse(idStr)
	s.Status = domain.CliAuthSessionStatus(statusStr)
	if uCode.Valid {
		s.UserCode = &uCode.String
	}
	if dCode.Valid {
		s.DeviceCode = &dCode.String
	}
	return &s, nil
}

func (r *SQLCLIAuthSessionRepository) Complete(ctx context.Context, id uuid.UUID, tokens domain.TokenResponse, userUUID uuid.UUID, userEmail string) (*domain.CliAuthSession, error) {
	query := `
UPDATE cli_auth_sessions 
SET status = 'approved', user_id = $1, token = $2, refresh_token = $3, updated_at = NOW()
WHERE id = $4`
	_, err := r.db.ExecContext(ctx, query, userUUID.String(), tokens.AccessToken, tokens.RefreshToken, id.String())
	if err != nil {
		return nil, err
	}
	return r.FindByState(ctx, id.String())
}

func (r *SQLCLIAuthSessionRepository) MarkConsumed(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE cli_auth_sessions SET status = 'consumed', updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id.String())
	return err
}

func (r *SQLCLIAuthSessionRepository) MarkDenied(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE cli_auth_sessions SET status = 'denied', updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id.String())
	return err
}

func (r *SQLCLIAuthSessionRepository) ExpirePending(ctx context.Context, now time.Time) (int, error) {
	query := `UPDATE cli_auth_sessions SET status = 'expired', updated_at = NOW() WHERE status = 'pending' AND expires_at < $1`
	res, err := r.db.ExecContext(ctx, query, now)
	if err != nil {
		return 0, err
	}
	affected, _ := res.RowsAffected()
	return int(affected), nil
}

// SQLProfileConfigRepository implements domain.ProfileConfigRepository.
type SQLProfileConfigRepository struct {
	db *sql.DB
}

// NewSQLProfileConfigRepository creates a new SQLProfileConfigRepository.
func NewSQLProfileConfigRepository(db *sql.DB) *SQLProfileConfigRepository {
	return &SQLProfileConfigRepository{db: db}
}

func (r *SQLProfileConfigRepository) Find(ctx context.Context, profileUUID uuid.UUID) ([]domain.ProfileConfig, error) {
	query := `SELECT id, user_id, config_key, config_value, created_at, updated_at FROM profile_configs WHERE user_id = $1`
	rows, err := r.db.QueryContext(ctx, query, profileUUID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []domain.ProfileConfig
	for rows.Next() {
		var c domain.ProfileConfig
		var idStr, userStr, keyStr string
		var valJSON []byte
		err := rows.Scan(&idStr, &userStr, &keyStr, &valJSON, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			return nil, err
		}
		c.UUID, _ = uuid.Parse(idStr)
		c.ProfileUUID, _ = uuid.Parse(userStr)
		c.ConfigKey = domain.ProfileConfigKey(keyStr)
		_ = json.Unmarshal(valJSON, &c.ConfigValue)
		configs = append(configs, c)
	}
	return configs, nil
}

func (r *SQLProfileConfigRepository) FindOne(ctx context.Context, profileUUID uuid.UUID, key domain.ProfileConfigKey) (*domain.ProfileConfig, error) {
	query := `SELECT id, user_id, config_key, config_value, created_at, updated_at FROM profile_configs WHERE user_id = $1 AND config_key = $2`
	var c domain.ProfileConfig
	var idStr, userStr, keyStr string
	var valJSON []byte
	err := r.db.QueryRowContext(ctx, query, profileUUID.String(), string(key)).Scan(&idStr, &userStr, &keyStr, &valJSON, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	c.UUID, _ = uuid.Parse(idStr)
	c.ProfileUUID, _ = uuid.Parse(userStr)
	c.ConfigKey = domain.ProfileConfigKey(keyStr)
	_ = json.Unmarshal(valJSON, &c.ConfigValue)
	return &c, nil
}

func (r *SQLProfileConfigRepository) Create(ctx context.Context, config domain.ProfileConfig) (*domain.ProfileConfig, error) {
	if config.UUID == uuid.Nil {
		config.UUID = uuid.New()
	}
	now := time.Now().UTC()
	config.CreatedAt = now
	config.UpdatedAt = now

	valBytes, _ := json.Marshal(config.ConfigValue)
	query := `
INSERT INTO profile_configs (id, user_id, config_key, config_value, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (user_id, config_key) DO UPDATE SET config_value = EXCLUDED.config_value, updated_at = NOW()`

	_, err := r.db.ExecContext(ctx, query,
		config.UUID.String(),
		config.ProfileUUID.String(),
		string(config.ConfigKey),
		valBytes,
		now,
		now,
	)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *SQLProfileConfigRepository) Update(ctx context.Context, id uuid.UUID, value any, status bool) (*domain.ProfileConfig, error) {
	valBytes, _ := json.Marshal(value)
	query := `UPDATE profile_configs SET config_value = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.ExecContext(ctx, query, valBytes, id.String())
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func (r *SQLProfileConfigRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM profile_configs WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id.String())
	return err
}
