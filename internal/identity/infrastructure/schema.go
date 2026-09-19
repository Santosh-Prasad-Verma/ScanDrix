package infrastructure

// PostgreSQL DDL schemas and TypeORM-compatible table definitions for ScanDrix Identity domain.
const (
	// UsersTableDDL creates the primary users table.
	UsersTableDDL = `
CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(36) PRIMARY KEY,
    email VARCHAR(255) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    organization_id VARCHAR(36),
    team_id VARCHAR(36),
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    is_admin BOOLEAN NOT NULL DEFAULT FALSE,
    avatar_url TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE
);
CREATE INDEX IF NOT EXISTS idx_users_org_team ON users(organization_id, team_id);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
`

	// AuthTableDDL creates the credentials and auth session table.
	AuthTableDDL = `
CREATE TABLE IF NOT EXISTS auth (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    password_hash VARCHAR(255),
    provider VARCHAR(50) NOT NULL DEFAULT 'local',
    provider_id VARCHAR(255),
    access_token TEXT,
    refresh_token TEXT,
    email_confirmed BOOLEAN NOT NULL DEFAULT FALSE,
    confirmation_token VARCHAR(255),
    reset_password_token VARCHAR(255),
    reset_password_expires_at TIMESTAMP WITH TIME ZONE,
    helpdesk_token VARCHAR(255),
    helpdesk_token_expires_at TIMESTAMP WITH TIME ZONE,
    last_login_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_auth_user_id ON auth(user_id);
CREATE INDEX IF NOT EXISTS idx_auth_provider_id ON auth(provider, provider_id);
CREATE INDEX IF NOT EXISTS idx_auth_reset_token ON auth(reset_password_token);
`

	// ProfilesTableDDL creates user profile metadata.
	ProfilesTableDDL = `
CREATE TABLE IF NOT EXISTS profiles (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    job_title VARCHAR(255),
    company VARCHAR(255),
    experience_level VARCHAR(50),
    preferred_language VARCHAR(50) DEFAULT 'en',
    marketing_survey JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_profiles_user_id ON profiles(user_id);
`

	// ProfileConfigsTableDDL creates key-value profile configuration preferences.
	ProfileConfigsTableDDL = `
CREATE TABLE IF NOT EXISTS profile_configs (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    config_key VARCHAR(100) NOT NULL,
    config_value JSONB NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_user_config_key UNIQUE(user_id, config_key)
);
CREATE INDEX IF NOT EXISTS idx_profile_configs_user ON profile_configs(user_id);
`

	// CLIAuthSessionsTableDDL creates device-flow and browser login sessions for the ScanDrix CLI.
	CLIAuthSessionsTableDDL = `
CREATE TABLE IF NOT EXISTS cli_auth_sessions (
    id VARCHAR(36) PRIMARY KEY,
    session_id VARCHAR(64) NOT NULL UNIQUE,
    user_code VARCHAR(16) UNIQUE,
    device_code VARCHAR(64) UNIQUE,
    user_id VARCHAR(36) REFERENCES users(id) ON DELETE CASCADE,
    team_id VARCHAR(36),
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    token TEXT,
    refresh_token TEXT,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    ip_address VARCHAR(45),
    user_agent TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_cli_session_id ON cli_auth_sessions(session_id);
CREATE INDEX IF NOT EXISTS idx_cli_device_code ON cli_auth_sessions(device_code);
CREATE INDEX IF NOT EXISTS idx_cli_user_code ON cli_auth_sessions(user_code);
`

	// PermissionsTableDDL creates RBAC permission rules.
	PermissionsTableDDL = `
CREATE TABLE IF NOT EXISTS permissions (
    id VARCHAR(36) PRIMARY KEY,
    organization_id VARCHAR(36) NOT NULL,
    team_id VARCHAR(36),
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(50) NOT NULL,
    scope VARCHAR(50) NOT NULL DEFAULT 'team',
    actions TEXT[] NOT NULL DEFAULT '{}',
    subjects TEXT[] NOT NULL DEFAULT '{}',
    conditions JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_permissions_user_scope ON permissions(user_id, organization_id, team_id);
`

	// UserRepositoryAssignmentsTableDDL tracks per-repository user access limits.
	UserRepositoryAssignmentsTableDDL = `
CREATE TABLE IF NOT EXISTS user_repository_assignments (
    id VARCHAR(36) PRIMARY KEY,
    organization_id VARCHAR(36) NOT NULL,
    team_id VARCHAR(36) NOT NULL,
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    repository_id VARCHAR(64) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_user_repo_assignment UNIQUE(user_id, repository_id)
);
CREATE INDEX IF NOT EXISTS idx_user_repo_assignments ON user_repository_assignments(organization_id, team_id, user_id);
`
)

// GetAllIdentityDDL returns all migration statements needed for identity domain.
func GetAllIdentityDDL() []string {
	return []string{
		UsersTableDDL,
		AuthTableDDL,
		ProfilesTableDDL,
		ProfileConfigsTableDDL,
		CLIAuthSessionsTableDDL,
		PermissionsTableDDL,
		UserRepositoryAssignmentsTableDDL,
	}
}
