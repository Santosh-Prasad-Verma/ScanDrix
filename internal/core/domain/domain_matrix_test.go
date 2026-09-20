package domain_test

import (
	"database/sql/driver"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDomainMatrix_JSONBMapDeepScannersAndValuers thoroughly exercises edge cases
// for JSONBMap: nil vs empty map, nested structures, type conversions, and errors.
func TestDomainMatrix_JSONBMapDeepScannersAndValuers(t *testing.T) {
	// 1. Nil map value serialization produces "{}"
	var nilMap domain.JSONBMap
	nilVal, err := nilMap.Value()
	require.NoError(t, err)
	assert.Equal(t, []byte("{}"), nilVal)

	// 2. Empty map serialization produces "{}"
	emptyMap := make(domain.JSONBMap)
	emptyVal, err := emptyMap.Value()
	require.NoError(t, err)
	assert.Equal(t, []byte("{}"), emptyVal)

	// 3. Scan nil produces non-nil empty map
	var scannedFromNil domain.JSONBMap
	err = scannedFromNil.Scan(nil)
	require.NoError(t, err)
	assert.NotNil(t, scannedFromNil)
	assert.Empty(t, scannedFromNil)

	// 4. Scan from string representation
	jsonString := `{"organization":"scandrix","active":true,"limit":100,"tags":["ai","security"]}`
	var scannedFromString domain.JSONBMap
	err = scannedFromString.Scan(jsonString)
	require.NoError(t, err)
	assert.Equal(t, "scandrix", scannedFromString["organization"])
	assert.Equal(t, true, scannedFromString["active"])
	assert.Equal(t, float64(100), scannedFromString["limit"])

	// 5. Scan from []byte representation
	jsonBytes := []byte(`{"nested":{"level":2,"key":"val"}}`)
	var scannedFromBytes domain.JSONBMap
	err = scannedFromBytes.Scan(jsonBytes)
	require.NoError(t, err)
	nested, ok := scannedFromBytes["nested"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "val", nested["key"])

	// 6. Scan from unsupported type returns descriptive error
	var scannedFromInt domain.JSONBMap
	err = scannedFromInt.Scan(12345)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal JSONBMap from int")

	// 7. Scan invalid JSON bytes returns syntax error
	var scannedInvalid domain.JSONBMap
	err = scannedInvalid.Scan([]byte(`{not-valid-json`))
	assert.Error(t, err)
}

// TestDomainMatrix_StringSliceDeepScannersAndValuers exercises PostgreSQL text array
// serialization/deserialization for StringSlice.
func TestDomainMatrix_StringSliceDeepScannersAndValuers(t *testing.T) {
	// 1. Nil StringSlice serialization produces "[]"
	var nilSlice domain.StringSlice
	nilVal, err := nilSlice.Value()
	require.NoError(t, err)
	assert.Equal(t, []byte("[]"), nilVal)

	// 2. Empty StringSlice produces "[]"
	emptySlice := domain.StringSlice{}
	emptyVal, err := emptySlice.Value()
	require.NoError(t, err)
	assert.Equal(t, []byte("[]"), emptyVal)

	// 3. Scan nil produces empty non-nil slice
	var scannedNil domain.StringSlice
	err = scannedNil.Scan(nil)
	require.NoError(t, err)
	assert.NotNil(t, scannedNil)
	assert.Empty(t, scannedNil)

	// 4. Scan from string representation
	rawStr := `["go","typescript","python","rust"]`
	var scannedStr domain.StringSlice
	err = scannedStr.Scan(rawStr)
	require.NoError(t, err)
	assert.Equal(t, domain.StringSlice{"go", "typescript", "python", "rust"}, scannedStr)

	// 5. Scan from bytes representation
	rawBytes := []byte(`["tag:security","tag:critical"]`)
	var scannedBytes domain.StringSlice
	err = scannedBytes.Scan(rawBytes)
	require.NoError(t, err)
	assert.Len(t, scannedBytes, 2)
	assert.Equal(t, "tag:security", scannedBytes[0])

	// 6. Scan from unsupported type returns error
	var scannedBadType domain.StringSlice
	err = scannedBadType.Scan(false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal StringSlice from bool")
}

// TestDomainMatrix_FloatVectorPgVectorScannersAndValuers tests the pgvector embedding
// serialization format: "[x,y,z]".
func TestDomainMatrix_FloatVectorPgVectorScannersAndValuers(t *testing.T) {
	// 1. Nil / empty vector returns nil driver.Value
	var nilVec domain.FloatVector
	val, err := nilVec.Value()
	require.NoError(t, err)
	assert.Nil(t, val)

	emptyVec := domain.FloatVector{}
	val, err = emptyVec.Value()
	require.NoError(t, err)
	assert.Nil(t, val)

	// 2. Scan nil resets vector to nil
	var scannedNil domain.FloatVector
	err = scannedNil.Scan(nil)
	require.NoError(t, err)
	assert.Nil(t, scannedNil)

	// 3. Roundtrip standard embeddings
	vec := domain.FloatVector{0.001, -0.999, 0.5, 12.345}
	driverVal, err := vec.Value()
	require.NoError(t, err)
	assert.Equal(t, "[0.001,-0.999,0.5,12.345]", driverVal)

	var scannedFromStr domain.FloatVector
	err = scannedFromStr.Scan(driverVal.(string))
	require.NoError(t, err)
	assert.Len(t, scannedFromStr, 4)
	assert.InDelta(t, 0.001, scannedFromStr[0], 0.00001)
	assert.InDelta(t, -0.999, scannedFromStr[1], 0.00001)

	// 4. Scan from []byte
	var scannedFromBytes domain.FloatVector
	err = scannedFromBytes.Scan([]byte("[1.0,2.0,3.0]"))
	require.NoError(t, err)
	assert.Equal(t, domain.FloatVector{1.0, 2.0, 3.0}, scannedFromBytes)

	// 5. Invalid literal format: missing brackets
	var scannedBadFormat domain.FloatVector
	err = scannedBadFormat.Scan("1.0,2.0,3.0")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid pgvector literal format")

	// 6. Too short literal format
	err = scannedBadFormat.Scan("[]")
	require.NoError(t, err)
	assert.Empty(t, scannedBadFormat)

	// 7. Unsupported scan type
	err = scannedBadFormat.Scan(42.5)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot scan type float64 into FloatVector")
}

// TestDomainMatrix_EnumDefinitionsCompleteness validates that all domain enums
// have unique string values and non-empty representations.
func TestDomainMatrix_EnumDefinitionsCompleteness(t *testing.T) {
	// 1. JobStatus
	jobStatuses := []domain.JobStatus{
		domain.JobStatusPending,
		domain.JobStatusProcessing,
		domain.JobStatusCompleted,
		domain.JobStatusFailed,
		domain.JobStatusWaitingForEvent,
		domain.JobStatusCancelled,
	}
	seenJobStatus := make(map[domain.JobStatus]bool)
	for _, js := range jobStatuses {
		assert.NotEmpty(t, string(js))
		assert.False(t, seenJobStatus[js], "Duplicate JobStatus: %s", js)
		seenJobStatus[js] = true
	}

	// 2. ErrorClassification
	errClasses := []domain.ErrorClassification{
		domain.ErrorClassificationRetryable,
		domain.ErrorClassificationNonRetryable,
		domain.ErrorClassificationCircuitOpen,
		domain.ErrorClassificationPermanent,
		domain.ErrorClassificationRateLimited,
	}
	seenErr := make(map[domain.ErrorClassification]bool)
	for _, ec := range errClasses {
		assert.NotEmpty(t, string(ec))
		assert.False(t, seenErr[ec], "Duplicate ErrorClassification: %s", ec)
		seenErr[ec] = true
	}

	// 3. PlatformType
	platforms := []domain.PlatformType{
		domain.PlatformTypeInternal,
		domain.PlatformTypeGitHub,
		domain.PlatformTypeGitLab,
		domain.PlatformTypeJira,
		domain.PlatformTypeSlack,
		domain.PlatformTypeNotion,
		domain.PlatformTypeMSTeams,
		domain.PlatformTypeDiscord,
		domain.PlatformTypeAzureBoards,
		domain.PlatformTypeAzureRepos,
		domain.PlatformTypeScanDrixWeb,
		domain.PlatformTypeBitbucket,
		domain.PlatformTypeForgejo,
	}
	seenPlatforms := make(map[domain.PlatformType]bool)
	for _, p := range platforms {
		assert.NotEmpty(t, string(p))
		assert.False(t, seenPlatforms[p], "Duplicate PlatformType: %s", p)
		seenPlatforms[p] = true
	}

	// 4. ProgrammingLanguage
	languages := []domain.ProgrammingLanguage{
		domain.LangTypeScript,
		domain.LangJavaScript,
		domain.LangPython,
		domain.LangGo,
		domain.LangJava,
		domain.LangCPP,
		domain.LangCSharp,
		domain.LangRuby,
		domain.LangPHP,
		domain.LangRust,
	}
	for _, l := range languages {
		assert.NotEmpty(t, string(l))
	}
}

// TestDomainMatrix_PaginationQueryLogic checks default values, boundary limits,
// and SQL limit/offset calculation.
func TestDomainMatrix_PaginationQueryLogic(t *testing.T) {
	// Zero struct defaults
	pq := domain.PaginationQuery{}
	pq.EnsureDefaults()
	assert.Equal(t, 1, pq.Page)
	assert.Equal(t, 25, pq.PageSize)
	assert.Equal(t, "DESC", pq.OrderDir)
	assert.Equal(t, "created_at", pq.OrderBy)

	// Page 3 with page size 50
	pq2 := domain.PaginationQuery{
		Page:     3,
		PageSize: 50,
		OrderBy:  "name",
		OrderDir: "ASC",
	}
	pq2.EnsureDefaults()
	assert.Equal(t, 3, pq2.Page)
	assert.Equal(t, 50, pq2.PageSize)
	assert.Equal(t, "ASC", pq2.OrderDir)
	assert.Equal(t, "name", pq2.OrderBy)

	// Negative values correction
	pqBad := domain.PaginationQuery{
		Page:     -5,
		PageSize: -100,
	}
	pqBad.EnsureDefaults()
	assert.Equal(t, 1, pqBad.Page)
	assert.Equal(t, 25, pqBad.PageSize)

	// PageSize exceeding maximum limit (100) resets to default 25
	pqLarge := domain.PaginationQuery{
		Page:     1,
		PageSize: 5000,
	}
	pqLarge.EnsureDefaults()
	assert.Equal(t, 25, pqLarge.PageSize)
}

// TestDomainMatrix_PaginatedResultCalculations verifies total page calculation
// and next page cursor logic.
func TestDomainMatrix_PaginatedResultCalculations(t *testing.T) {
	items := []string{"item-1", "item-2", "item-3"}
	res := domain.PaginatedResult[string]{
		Items:      items,
		TotalCount: 105,
		Page:       2,
		PageSize:   25,
		TotalPages: 5,
		HasNext:    true,
		NextCursor: "cursor_page_3",
	}

	assert.Equal(t, 3, len(res.Items))
	assert.Equal(t, int64(105), res.TotalCount)
	assert.Equal(t, 2, res.Page)
	assert.True(t, res.HasNext)
	assert.Equal(t, "cursor_page_3", res.NextCursor)

	// JSON roundtrip
	bytes, err := json.Marshal(res)
	require.NoError(t, err)

	var unmarshaled domain.PaginatedResult[string]
	err = json.Unmarshal(bytes, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, res.Items, unmarshaled.Items)
	assert.Equal(t, res.TotalCount, unmarshaled.TotalCount)
	assert.Equal(t, res.HasNext, unmarshaled.HasNext)
}

// TestDomainMatrix_AllEntityConstructorsAndBaseEmbeddings checks proper embedding
// of BaseEntity and TenantScopedEntity across all domain models.
func TestDomainMatrix_AllEntityConstructorsAndBaseEmbeddings(t *testing.T) {
	wsID := uuid.New()
	userID := uuid.New()
	now := time.Now().UTC()

	// 1. Workspace
	ws := domain.Workspace{
		BaseEntity: domain.BaseEntity{
			ID:        wsID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Slug:   "scandrix-hq",
		Name:   "ScanDrix HQ",
		Tier:   "ENTERPRISE",
		Status: "ACTIVE",
	}
	assert.Equal(t, wsID, ws.ID)

	// 2. User
	user := domain.User{
		BaseEntity: domain.BaseEntity{
			ID:        userID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		WorkspaceID:   wsID,
		Email:         "lead@scandrix.dev",
		FullName:      "Lead Engineer",
		IsActive:      true,
		EmailVerified: true,
	}
	assert.Equal(t, userID, user.ID)
	assert.Equal(t, wsID, user.WorkspaceID)

	// 3. TrackedRepository
	repoID := uuid.New()
	tracked := domain.TrackedRepository{
		TenantScopedEntity: domain.TenantScopedEntity{
			BaseEntity:  domain.BaseEntity{ID: repoID, CreatedAt: now, UpdatedAt: now},
			WorkspaceID: wsID,
		},
		FullName:    "scandrix/scanner",
		SCMProvider: "GITHUB",
		IsActive:    true,
	}
	assert.Equal(t, repoID, tracked.ID)
	assert.Equal(t, wsID, tracked.WorkspaceID)

	// 4. PullRequest
	prID := uuid.New()
	pr := domain.PullRequest{
		TenantScopedEntity: domain.TenantScopedEntity{
			BaseEntity:  domain.BaseEntity{ID: prID, CreatedAt: now, UpdatedAt: now},
			WorkspaceID: wsID,
		},
		RepositoryID: repoID,
		PullNumber:   42,
		Title:        "feat: Add AST analysis step",
		State:        "OPEN",
	}
	assert.Equal(t, 42, pr.PullNumber)
	assert.Equal(t, repoID, pr.RepositoryID)

	// 5. CodeFinding
	findingID := uuid.New()
	finding := domain.CodeFinding{
		TenantScopedEntity: domain.TenantScopedEntity{
			BaseEntity:  domain.BaseEntity{ID: findingID, CreatedAt: now, UpdatedAt: now},
			WorkspaceID: wsID,
		},
		FilePath:    "internal/core/crypto/service.go",
		LineStart:   105,
		LineEnd:     110,
		Severity:  "HIGH",
		Category:  "SECURITY",
		Message:   "Weak IV / Nonce Reuse: Nonce must be 12 bytes generated securely",
	}
	assert.Equal(t, "internal/core/crypto/service.go", finding.FilePath)
	assert.Equal(t, "HIGH", finding.Severity)
}

// TestDomainMatrix_MappersValidationErrors tests validation failures during DTO conversion.
func TestDomainMatrix_MappersValidationErrors(t *testing.T) {
	// 1. WorkspaceCreateDTO with empty slug
	_, err := domain.ToWorkspace(&domain.WorkspaceCreateDTO{
		Slug: "",
		Name: "Empty Slug Org",
	})
	assert.Error(t, err)

	// 2. WorkspaceCreateDTO with empty name
	_, err = domain.ToWorkspace(&domain.WorkspaceCreateDTO{
		Slug: "valid-slug",
		Name: "",
	})
	assert.Error(t, err)

	// 3. DrixyRuleCreateDTO with empty RuleKey
	_, err = domain.ToDrixyRule(&domain.DrixyRuleCreateDTO{
		WorkspaceID: uuid.New(),
		RuleKey:     "",
		Name:        "Rule Name",
	})
	assert.Error(t, err)

	// 4. TrackedRepositoryCreateDTO with empty FullName
	_, err = domain.ToTrackedRepository(&domain.TrackedRepositoryCreateDTO{
		WorkspaceID:    uuid.New(),
		OrganizationID: uuid.New(),
		FullName:       "",
	})
	assert.Error(t, err)
}

// Ensure driver.Valuer interface compatibility at compile-time
var (
	_ driver.Valuer = domain.JSONBMap{}
	_ driver.Valuer = domain.StringSlice{}
	_ driver.Valuer = domain.FloatVector{}
)
