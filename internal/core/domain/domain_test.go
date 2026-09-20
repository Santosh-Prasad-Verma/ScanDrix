package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONBMapSerialization(t *testing.T) {
	m := domain.JSONBMap{
		"strictness": "HIGH",
		"max_files":  100,
		"tags":       []any{"security", "owasp"},
	}

	val, err := m.Value()
	require.NoError(t, err)
	assert.NotEmpty(t, val)

	var scanned domain.JSONBMap
	err = scanned.Scan(val)
	require.NoError(t, err)
	assert.Equal(t, "HIGH", scanned["strictness"])
	assert.Equal(t, float64(100), scanned["max_files"])
}

func TestFloatVectorSerialization(t *testing.T) {
	v := domain.FloatVector{0.123, -0.456, 0.789}
	val, err := v.Value()
	require.NoError(t, err)
	assert.Equal(t, "[0.123,-0.456,0.789]", val)

	var scanned domain.FloatVector
	err = scanned.Scan(val)
	require.NoError(t, err)
	require.Len(t, scanned, 3)
	assert.InDelta(t, 0.123, scanned[0], 0.0001)
	assert.InDelta(t, -0.456, scanned[1], 0.0001)
	assert.InDelta(t, 0.789, scanned[2], 0.0001)
}

func TestWorkspaceMappers(t *testing.T) {
	dto := &domain.WorkspaceCreateDTO{
		Slug:                "Acme-Corp",
		Name:                "Acme Corporation",
		Tier:                "ENTERPRISE",
		SpendLimitUSD:       500.0,
		EnforceSpendLimit:   true,
		AllowedEmailDomains: domain.StringSlice{"acme.com"},
		Metadata:            domain.JSONBMap{"sector": "fintech"},
	}

	ws, err := domain.ToWorkspace(dto)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, ws.ID)
	assert.Equal(t, "acme-corp", ws.Slug)
	assert.Equal(t, "Acme Corporation", ws.Name)
	assert.Equal(t, "ENTERPRISE", ws.Tier)
	assert.Equal(t, 500.0, ws.SpendLimitUSD)
	assert.True(t, ws.EnforceSpendLimit)
	assert.Contains(t, ws.AllowedEmailDomains, "acme.com")

	newName := "Acme Global"
	newSpend := 1000.0
	updateDTO := &domain.WorkspaceUpdateDTO{
		Name:          &newName,
		SpendLimitUSD: &newSpend,
	}

	err = domain.ApplyWorkspaceUpdates(ws, updateDTO)
	require.NoError(t, err)
	assert.Equal(t, "Acme Global", ws.Name)
	assert.Equal(t, 1000.0, ws.SpendLimitUSD)
}

func TestTenantBoundaryVerification(t *testing.T) {
	wsA := uuid.New()
	wsB := uuid.New()

	assert.NoError(t, domain.VerifyTenantBoundary(wsA, wsA))
	assert.ErrorIs(t, domain.VerifyTenantBoundary(wsA, wsB), domain.ErrTenantMismatch)
	assert.ErrorIs(t, domain.VerifyTenantBoundary(wsA, uuid.Nil), domain.ErrInvalidWorkspaceID)
}

func TestPaginationDefaults(t *testing.T) {
	pq := domain.PaginationQuery{}
	pq.EnsureDefaults()

	assert.Equal(t, 1, pq.Page)
	assert.Equal(t, 25, pq.PageSize)
	assert.Equal(t, "DESC", pq.OrderDir)
	assert.Equal(t, "created_at", pq.OrderBy)
}

func TestDrixyRuleCreation(t *testing.T) {
	wsID := uuid.New()
	dto := &domain.DrixyRuleCreateDTO{
		WorkspaceID:         wsID,
		RuleKey:             "SEC_001_NO_HARDCODED_SECRETS",
		Name:                "No Hardcoded Secrets",
		Category:            "SECURITY",
		Severity:            "CRITICAL",
		Description:         "Disallows API keys and credentials in code",
		PromptInstructions:  "Inspect for JWTs, bearer tokens, or plaintext passwords.",
		ApplicableLanguages: domain.StringSlice{"go", "typescript", "python"},
		IsActive:            true,
		WeightMultiplier:    2.0,
	}

	rule, err := domain.ToDrixyRule(dto)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, rule.ID)
	assert.Equal(t, wsID, rule.WorkspaceID)
	assert.Equal(t, "sec_001_no_hardcoded_secrets", rule.RuleKey)
	assert.Equal(t, "SECURITY", rule.Category)
	assert.Equal(t, "CRITICAL", rule.Severity)
	assert.Equal(t, float32(2.0), rule.WeightMultiplier)
	assert.True(t, rule.IsActive)
}

func TestTrackedRepositoryCreation(t *testing.T) {
	wsID := uuid.New()
	orgID := uuid.New()
	dto := &domain.TrackedRepositoryCreateDTO{
		WorkspaceID:    wsID,
		OrganizationID: orgID,
		SCMProvider:    "GITHUB",
		ExternalRepoID: "12345678",
		FullName:       "scandrix/backend",
		DefaultBranch:  "main",
		IsPrivate:      true,
	}

	repo, err := domain.ToTrackedRepository(dto)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, repo.ID)
	assert.Equal(t, wsID, repo.WorkspaceID)
	assert.Equal(t, orgID, repo.OrganizationID)
	assert.Equal(t, "GITHUB", repo.SCMProvider)
	assert.Equal(t, "scandrix/backend", repo.FullName)
	assert.Equal(t, ".scandrix/config.json", repo.ConfigFilePath)
}

func TestOutboxRecordCreation(t *testing.T) {
	wsID := uuid.New()
	now := time.Now().UTC()
	outbox := &domain.OutboxRecord{
		TenantScopedEntity: domain.TenantScopedEntity{
			BaseEntity: domain.BaseEntity{
				ID:        uuid.New(),
				CreatedAt: now,
				UpdatedAt: now,
			},
			WorkspaceID: wsID,
		},
		EventType:  "PULL_REQUEST_REVIEW_COMPLETED",
		Payload:    `{"review_id":"123","status":"COMPLETED"}`,
		Status:     "PENDING",
		MaxRetries: 5,
	}

	assert.Equal(t, wsID, outbox.WorkspaceID)
	assert.Equal(t, "PENDING", outbox.Status)
	assert.Equal(t, 5, outbox.MaxRetries)
}
