package domain_test

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Comprehensive Domain Validation & Invariant Stress Test Suite
// ============================================================================

// TestDomainValidation_AstGraphInvariants validates syntax node trees, edges,
// and language property structures.
func TestDomainValidation_AstGraphInvariants(t *testing.T) {
	wsID := uuid.New()
	repoID := uuid.New()
	commit := "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678"

	// 1. Build AST Nodes
	nodes := make([]*domain.AstNode, 50)
	for i := 0; i < 50; i++ {
		sig := fmt.Sprintf("func Symbol_%02d(ctx context.Context, id int) (string, error)", i)
		nodes[i] = &domain.AstNode{
			TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsID},
			RepositoryID:       repoID,
			CommitSHA:          commit,
			FilePath:           fmt.Sprintf("internal/service/module_%02d.go", i/5),
			NodeType:           "FUNCTION_DECLARATION",
			Identifier:         fmt.Sprintf("Symbol_%02d", i),
			PackageName:        "service",
			LineStart:          10 + (i * 15),
			LineEnd:            24 + (i * 15),
			Signature:          &sig,
			Properties: domain.JSONBMap{
				"exported":   true,
				"async":      false,
				"cyclomatic": float64(3 + (i % 5)),
				"tokens":     float64(45 + i),
			},
		}
		require.NotEqual(t, uuid.Nil, nodes[i].WorkspaceID)
		assert.True(t, nodes[i].LineEnd > nodes[i].LineStart)
	}

	// 2. Build AST Call Graph Edges between consecutive nodes
	edges := make([]*domain.AstEdge, 49)
	for i := 0; i < 49; i++ {
		edges[i] = &domain.AstEdge{
			TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsID},
			RepositoryID:       repoID,
			SourceNodeID:       nodes[i].ID,
			TargetNodeID:       nodes[i+1].ID,
			EdgeType:           "CALLS",
			Weight:             1.0 + float32(i)*0.05,
			Metadata: domain.JSONBMap{
				"callSiteLine": float64(nodes[i].LineStart + 5),
				"isIndirect":   false,
			},
		}
		assert.Equal(t, wsID, edges[i].WorkspaceID)
		assert.Equal(t, "CALLS", edges[i].EdgeType)
		assert.True(t, edges[i].Weight >= 1.0)
	}

	// 3. JSON roundtrip serialization
	marshaledNode, err := json.Marshal(nodes[0])
	require.NoError(t, err)

	var unmarshaledNode domain.AstNode
	err = json.Unmarshal(marshaledNode, &unmarshaledNode)
	require.NoError(t, err)
	assert.Equal(t, nodes[0].Identifier, unmarshaledNode.Identifier)
	assert.Equal(t, nodes[0].PackageName, unmarshaledNode.PackageName)
	assert.Equal(t, float64(3), unmarshaledNode.Properties["cyclomatic"])

	// 4. Context References attachment
	revID := uuid.New()
	ref := &domain.ContextReference{
		TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsID},
		ReviewID:           revID,
		Provider:           "JIRA",
		ReferenceID:        "PROJ-1042",
		ReferenceURL:       "https://jira.example.com/browse/PROJ-1042",
		Title:              "Migrate database schema to PostgreSQL 16",
		Status:             "IN_PROGRESS",
		RawPayload: domain.JSONBMap{
			"priority": "High",
			"assignee": "tarun",
		},
	}
	assert.Equal(t, "JIRA", ref.Provider)
	assert.Equal(t, "PROJ-1042", ref.ReferenceID)
	assert.Equal(t, "tarun", ref.RawPayload["assignee"])
}

// TestDomainValidation_AutomationRuleLifecycles exercises trigger conditions,
// execution status transitions, and action payload schemas.
func TestDomainValidation_AutomationRuleLifecycles(t *testing.T) {
	wsID := uuid.New()
	orgID := uuid.New()

	auto := &domain.Automation{
		TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsID},
		OrganizationID:     orgID,
		Name:               "Auto-flag security violations on PR",
		TriggerEvent:       "PULL_REQUEST_OPENED",
		ActionType:         "SLACK_NOTIFY_AND_BLOCK",
		FilterConditions: domain.JSONBMap{
			"branches":     []any{"main", "release/*"},
			"minSeverity":  "HIGH",
			"excludeDraft": true,
		},
		ActionConfiguration: domain.JSONBMap{
			"slackChannel": "#sec-alerts",
			"blockMerge":   true,
			"mentionTeam":  "@sec-ops",
		},
		IsActive: true,
	}

	assert.True(t, auto.IsActive)
	assert.Equal(t, "PULL_REQUEST_OPENED", auto.TriggerEvent)

	// Status transitions for AutomationExecution
	states := []string{
		string(domain.AutomationStatusPending),
		string(domain.AutomationStatusRunning),
		string(domain.AutomationStatusSuccess),
		string(domain.AutomationStatusFailure),
		string(domain.AutomationStatusSkipped),
	}

	for _, s := range states {
		exec := &domain.AutomationExecution{
			TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsID},
			AutomationID:       auto.ID,
			TriggerPayload: domain.JSONBMap{
				"prNumber":  float64(42),
				"repoOwner": "scandrix",
			},
			Status:     s,
			DurationMs: 350,
			ExecutedAt: time.Now().UTC(),
		}
		assert.Equal(t, s, exec.Status)
		assert.Equal(t, int64(350), exec.DurationMs)
	}

	// Team Automation squad mapping
	teamAuto := &domain.TeamAutomation{
		TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsID},
		TeamID:             uuid.New(),
		AutomationID:       auto.ID,
	}
	assert.Equal(t, wsID, teamAuto.WorkspaceID)
	assert.Equal(t, auto.ID, teamAuto.AutomationID)
}

// TestDomainValidation_IntegrationsAndIssueTracking tests third-party platforms.
func TestDomainValidation_IntegrationsAndIssueTracking(t *testing.T) {
	wsID := uuid.New()
	orgID := uuid.New()
	repoID := uuid.New()

	platforms := []domain.PlatformType{
		domain.PlatformTypeGitHub,
		domain.PlatformTypeGitLab,
		domain.PlatformTypeBitbucket,
		domain.PlatformTypeAzureRepos,
		domain.PlatformTypeAzureBoards,
		domain.PlatformTypeJira,
		domain.PlatformTypeSlack,
		domain.PlatformTypeMSTeams,
		domain.PlatformTypeDiscord,
		domain.PlatformTypeNotion,
		domain.PlatformTypeForgejo,
	}

	for idx, p := range platforms {
		integration := &domain.Integration{
			TenantScopedEntity:   domain.TenantScopedEntity{WorkspaceID: wsID},
			OrganizationID:       orgID,
			Provider:             string(p),
			DisplayName:          fmt.Sprintf("Integration-%s", p),
			IsActive:             true,
			EncryptedAuthPayload: "cipher:aes-gcm:enc_key_payload_xyz",
			Settings: domain.JSONBMap{
				"webhookRegistered": true,
				"rateLimitTier":     "standard",
			},
		}

		assert.Equal(t, string(p), integration.Provider)
		assert.True(t, integration.IsActive)

		// Config mapping
		cfg := &domain.IntegrationConfig{
			TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsID},
			IntegrationID:      integration.ID,
			ConfigKey:          "channel_info",
			ConfigValue:        "#dev-reviews",
			IsEncrypted:        false,
		}
		assert.Equal(t, integration.ID, cfg.IntegrationID)

		// Create associated Issue
		desc := fmt.Sprintf("Automated issue tracking for %s", p)
		assignee := "security@scandrix.dev"
		issue := &domain.Issue{
			TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsID},
			RepositoryID:       repoID,
			ExternalID:         fmt.Sprintf("EXT-%03d", idx),
			Provider:           string(p),
			Title:              fmt.Sprintf("Vulnerability remediation task for %s", p),
			Description:        &desc,
			Severity:           "HIGH",
			Status:             "OPEN",
			Assignee:           &assignee,
			URL:                fmt.Sprintf("https://issue-tracker.example.com/%s/%d", p, idx),
			Metadata:           domain.JSONBMap{"automated": true},
		}

		assert.Equal(t, repoID, issue.RepositoryID)
		assert.Equal(t, "OPEN", issue.Status)
		assert.Equal(t, "HIGH", issue.Severity)
		assert.Equal(t, string(p), issue.Provider)
	}
}

// TestDomainValidation_PaginationGenericContracts verifies generic pagination mathematical invariants.
func TestDomainValidation_PaginationGenericContracts(t *testing.T) {
	type SampleItem struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	items := make([]SampleItem, 100)
	for i := 0; i < 100; i++ {
		items[i] = SampleItem{ID: i + 1, Name: fmt.Sprintf("Item-%03d", i+1)}
	}

	paginateItems := func(all []SampleItem, page, pageSize int) domain.PaginatedResult[SampleItem] {
		if pageSize <= 0 {
			pageSize = 10
		}
		if page <= 0 {
			page = 1
		}
		totalCount := int64(len(all))
		totalPages := int((totalCount + int64(pageSize) - 1) / int64(pageSize))

		start := (page - 1) * pageSize
		if start >= len(all) {
			return domain.PaginatedResult[SampleItem]{
				Items:      []SampleItem{},
				TotalCount: totalCount,
				Page:       page,
				PageSize:   pageSize,
				TotalPages: totalPages,
				HasNext:    false,
			}
		}

		end := start + pageSize
		if end > len(all) {
			end = len(all)
		}

		return domain.PaginatedResult[SampleItem]{
			Items:      all[start:end],
			TotalCount: totalCount,
			Page:       page,
			PageSize:   pageSize,
			TotalPages: totalPages,
			HasNext:    page < totalPages,
		}
	}

	// Page 1 of 10
	res1 := paginateItems(items, 1, 10)
	assert.Len(t, res1.Items, 10)
	assert.Equal(t, int64(100), res1.TotalCount)
	assert.Equal(t, 10, res1.TotalPages)
	assert.True(t, res1.HasNext)
	assert.Equal(t, 1, res1.Items[0].ID)
	assert.Equal(t, 10, res1.Items[9].ID)

	// Page 10 of 10 (Last page)
	res10 := paginateItems(items, 10, 10)
	assert.Len(t, res10.Items, 10)
	assert.False(t, res10.HasNext)
	assert.Equal(t, 91, res10.Items[0].ID)
	assert.Equal(t, 100, res10.Items[9].ID)

	// Page 11 of 10 (Out of range)
	res11 := paginateItems(items, 11, 10)
	assert.Empty(t, res11.Items)
	assert.False(t, res11.HasNext)

	// PaginationQuery EnsureDefaults
	pq := domain.PaginationQuery{}
	pq.EnsureDefaults()
	assert.Equal(t, 1, pq.Page)
	assert.Equal(t, 25, pq.PageSize)
	assert.Equal(t, "DESC", pq.OrderDir)
	assert.Equal(t, "created_at", pq.OrderBy)

	// PaginationDto Normalize
	pd := domain.PaginationDto{Page: -1, Limit: 5000, Skip: -5}
	pd.Normalize()
	assert.Equal(t, 1, pd.Page)
	assert.Equal(t, 1000, pd.Limit)
	assert.Equal(t, 0, pd.Skip)
}

// TestDomainValidation_ReviewFindingConfidenceAndScoring tests security finding structures.
func TestDomainValidation_ReviewFindingConfidenceAndScoring(t *testing.T) {
	wsID := uuid.New()
	repoID := uuid.New()
	revID := uuid.New()

	severities := []string{"INFO", "LOW", "MEDIUM", "HIGH", "CRITICAL"}

	for idx, sev := range severities {
		cwe := fmt.Sprintf("CWE-%d", 79+idx*10)
		owasp := fmt.Sprintf("A0%d:2021", idx+1)
		snippet := "func unsafeQuery(input string) { db.Exec(\"SELECT * FROM users WHERE name = '\" + input + \"'\") }"
		fix := "func safeQuery(input string) { db.Exec(\"SELECT * FROM users WHERE name = $1\", input) }"
		diff := "@@ -1,2 +1,2 @@\n- db.Exec(\"SELECT * FROM users WHERE name = '\" + input + \"'\")\n+ db.Exec(\"SELECT * FROM users WHERE name = $1\", input)"

		finding := &domain.CodeFinding{
			TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsID},
			ReviewID:           revID,
			RepositoryID:       repoID,
			RuleID:             fmt.Sprintf("SEC-%03d", idx),
			Category:           "SECURITY",
			Severity:           sev,
			FilePath:           fmt.Sprintf("cmd/server/handler_%02d.go", idx),
			LineStart:          25 + idx,
			LineEnd:            30 + idx,
			Message:            fmt.Sprintf("SQL injection vulnerability detected in handler_%02d", idx),
			CodeSnippet:        &snippet,
			SuggestedFix:       &fix,
			DiffHunk:           &diff,
			CWE:                &cwe,
			OWASP:              &owasp,
			ConfidenceScore:    0.85 + float32(idx)*0.03,
			IsResolved:         false,
		}

		assert.Equal(t, sev, finding.Severity)
		assert.Equal(t, "SECURITY", finding.Category)
		assert.True(t, finding.ConfidenceScore >= 0.85 && finding.ConfidenceScore <= 1.0)
		assert.Equal(t, &cwe, finding.CWE)
		assert.Equal(t, &owasp, finding.OWASP)

		// Thumbs-up feedback
		feedback := &domain.FindingFeedback{
			TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsID},
			FindingID:          finding.ID,
			UserID:             uuid.New(),
			Reaction:           "THUMBS_UP",
			IsActioned:         true,
		}
		assert.Equal(t, "THUMBS_UP", feedback.Reaction)
		assert.True(t, feedback.IsActioned)
	}
}

// TestDomainValidation_DrixyRulesPacksAndMultipliers tests rule definitions and regex validation.
func TestDomainValidation_DrixyRulesPacksAndMultipliers(t *testing.T) {
	wsID := uuid.New()

	rules := []struct {
		key       string
		category  string
		severity  string
		pattern   string
		weight    float32
		languages []string
	}{
		{
			key:       "GO-NO-NIL-PANIC",
			category:  "RELIABILITY",
			severity:  "HIGH",
			pattern:   `panic\(`,
			weight:    1.5,
			languages: []string{"GO"},
		},
		{
			key:       "SEC-NO-RAW-EVAL",
			category:  "SECURITY",
			severity:  "CRITICAL",
			pattern:   `eval\(`,
			weight:    2.0,
			languages: []string{"JAVASCRIPT", "TYPESCRIPT"},
		},
		{
			key:       "PERF-N-PLUS-ONE",
			category:  "PERFORMANCE",
			severity:  "MEDIUM",
			pattern:   `for.*SELECT`,
			weight:    1.2,
			languages: []string{"GO", "PYTHON", "JAVA"},
		},
		{
			key:       "STYLE-NO-COMMENTED-CODE",
			category:  "STYLE",
			severity:  "LOW",
			pattern:   `//.*function\s*\(`,
			weight:    0.5,
			languages: []string{"GO", "TYPESCRIPT", "PYTHON"},
		},
	}

	for _, r := range rules {
		// Verify regex compilation
		compiled, err := regexp.Compile(r.pattern)
		require.NoError(t, err, "regex pattern must be valid")
		assert.NotNil(t, compiled)

		patternStr := r.pattern
		ruleEntity := &domain.DrixyRules{
			TenantScopedEntity:  domain.TenantScopedEntity{WorkspaceID: wsID},
			RuleKey:             r.key,
			Name:                fmt.Sprintf("Rule: %s", r.key),
			Category:            r.category,
			Severity:            r.severity,
			Description:         fmt.Sprintf("Enforces %s across codebases", r.key),
			Pattern:             &patternStr,
			PromptInstructions:  fmt.Sprintf("Scan code for %s violations and suggest idiomatic alternatives.", r.key),
			ApplicableLanguages: domain.StringSlice(r.languages),
			IsActive:            true,
			IsSystemDefault:     false,
			WeightMultiplier:    r.weight,
			LikesCount:          42,
		}

		assert.Equal(t, r.key, ruleEntity.RuleKey)
		assert.Equal(t, r.weight, ruleEntity.WeightMultiplier)
		assert.True(t, ruleEntity.IsActive)
		assert.Len(t, ruleEntity.ApplicableLanguages, len(r.languages))

		// Audit Log
		settingsLog := &domain.CodeReviewSettingsLog{
			TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsID},
			ModifiedByUserID:   uuid.New(),
			ScopeType:          "WORKSPACE",
			ScopeID:            wsID,
			PreviousValues:     domain.JSONBMap{"active": false},
			UpdatedValues:      domain.JSONBMap{"active": true, "weight": float64(r.weight)},
		}
		assert.Equal(t, "WORKSPACE", settingsLog.ScopeType)
	}
}

// TestDomainValidation_TeamMemberRolesAndPermissions exercises squad RBAC models.
func TestDomainValidation_TeamMemberRolesAndPermissions(t *testing.T) {
	wsID := uuid.New()
	teamID := uuid.New()

	roles := []struct {
		role              string
		notificationOptIn bool
	}{
		{"OWNER", true},
		{"LEAD", true},
		{"MAINTAINER", true},
		{"REVIEWER", true},
		{"CONTRIBUTOR", false},
	}

	for _, tc := range roles {
		uID := uuid.New()
		member := &domain.TeamMember{
			TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsID},
			TeamID:             teamID,
			UserID:             uID,
			Role:               tc.role,
			NotificationOptIn: tc.notificationOptIn,
		}

		assert.Equal(t, tc.role, member.Role)
		assert.Equal(t, tc.notificationOptIn, member.NotificationOptIn)
		assert.Equal(t, teamID, member.TeamID)
	}

	// Team CLI Key generation with scandrix_ prefix
	keyPrefix := "scandrix_"
	cliKey := &domain.TeamCliKey{
		TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsID},
		TeamID:             teamID,
		CreatedByUserID:    uuid.New(),
		KeyPrefix:          keyPrefix,
		KeyHash:            "sha256_mock_hash_val_99887766",
		Name:               "CI Review Bot Key",
		Scopes:             domain.StringSlice{"repos:read", "review:write"},
	}

	assert.True(t, strings.HasPrefix(cliKey.KeyPrefix, "scandrix_"))
	assert.Contains(t, cliKey.Scopes, "review:write")
}

// TestDomainValidation_WorkspaceTenancyAndSpendLimits validates spend limits and billing intervals.
func TestDomainValidation_WorkspaceTenancyAndSpendLimits(t *testing.T) {
	tiers := []struct {
		tier         string
		spendLimit   float64
		currentSpend float64
		enforce      bool
		shouldExceed bool
	}{
		{"DEVELOPER", 50.0, 42.50, true, false},
		{"DEVELOPER", 50.0, 52.10, true, true},
		{"TEAM", 250.0, 180.00, true, false},
		{"ENTERPRISE", 10000.0, 8500.00, false, false},
	}

	for _, tc := range tiers {
		ws := &domain.Workspace{
			Slug:                 fmt.Sprintf("tenant-%s", strings.ToLower(tc.tier)),
			Name:                 fmt.Sprintf("Workspace %s", tc.tier),
			Status:               "ACTIVE",
			Tier:                 tc.tier,
			SpendLimitUSD:        tc.spendLimit,
			CurrentMonthSpendUSD: tc.currentSpend,
			EnforceSpendLimit:    tc.enforce,
			AllowedEmailDomains:  domain.StringSlice{"scandrix.dev", "enterprise.com"},
		}

		exceeded := ws.EnforceSpendLimit && (ws.CurrentMonthSpendUSD > ws.SpendLimitUSD)
		assert.Equal(t, tc.shouldExceed, exceeded)
		assert.Contains(t, ws.AllowedEmailDomains, "scandrix.dev")
	}

	// Organization Parameters
	prompt := "Focus strictly on OWASP Top 10 vulnerabilities"
	orgParams := &domain.OrganizationParameters{
		TenantScopedEntity:   domain.TenantScopedEntity{WorkspaceID: uuid.New()},
		OrganizationID:       uuid.New(),
		StrictnessLevel:      "STRICT",
		AutoReviewDrafts:     false,
		MinSeverity:          "HIGH",
		IgnoreDraftPRs:       true,
		MaxFilesPerReview:    150,
		MaxDiffBytes:         1024 * 1024,
		EnabledRules:         domain.StringSlice{"SEC-001", "SEC-002"},
		ExcludedPaths:        domain.StringSlice{"vendor/*", "node_modules/*"},
		CustomPrompt:         &prompt,
		TelemetryEnabled:     true,
		NotificationChannels: domain.JSONBMap{"slack": "#sec-reviews"},
	}
	assert.Equal(t, "STRICT", orgParams.StrictnessLevel)
	assert.Equal(t, 150, orgParams.MaxFilesPerReview)
	assert.Contains(t, orgParams.EnabledRules, "SEC-001")
	assert.True(t, orgParams.TelemetryEnabled)

	// Global Parameters & Presets
	preset := &domain.ParametersPreset{
		Slug:        "owasp-top-10",
		Name:        "OWASP Top 10 Preset",
		Description: "Enforces OWASP Top 10 security standards",
		Category:    "SECURITY",
		Configuration: domain.JSONBMap{
			"minSeverity": "MEDIUM",
			"rules":       []any{"A01", "A02", "A03"},
		},
		IsDefault: true,
	}
	assert.Equal(t, "owasp-top-10", preset.Slug)
	assert.True(t, preset.IsDefault)

	// Permissions model
	perm := &domain.Permissions{
		TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: uuid.New()},
		RoleName:           "DEVELOPER",
		Resource:           "code_review",
		Action:             "create",
		IsAllowed:          true,
		Conditions:         domain.JSONBMap{"ownPR": true},
	}
	assert.True(t, perm.IsAllowed)
	assert.Equal(t, "DEVELOPER", perm.RoleName)
}

// TestDomainValidation_UserCredentialsAndMFA validates authentication security invariants.
func TestDomainValidation_UserCredentialsAndMFA(t *testing.T) {
	wsID := uuid.New()
	userID := uuid.New()
	orgID := uuid.New()

	mfaSecret := "JBSWY3DPEHPK3PXP"
	now := time.Now().UTC()

	auth := &domain.Auth{
		TenantScopedEntity:  domain.TenantScopedEntity{WorkspaceID: wsID},
		UserID:              userID,
		PasswordHash:        "$argon2id$v=19$m=65536,t=3,p=4$c2FsdHNhbHQ$abcdef123456",
		Salt:                "c2FsdHNhbHQ",
		MFASecret:           &mfaSecret,
		MFAEnabled:          true,
		FailedLoginAttempts: 0,
		PasswordChangedAt:   now,
	}

	assert.True(t, auth.MFAEnabled)
	assert.NotEmpty(t, auth.PasswordHash)
	assert.NotEmpty(t, auth.Salt)
	assert.Equal(t, 0, auth.FailedLoginAttempts)

	// Simulate failed login attempts leading to account lock
	auth.FailedLoginAttempts = 5
	lockedUntil := now.Add(15 * time.Minute)
	auth.LockedUntil = &lockedUntil

	assert.True(t, auth.LockedUntil.After(time.Now().UTC()))
	assert.Equal(t, 5, auth.FailedLoginAttempts)

	// SSO Configuration test
	sso := &domain.SSOConfig{
		TenantScopedEntity:   domain.TenantScopedEntity{WorkspaceID: wsID},
		OrganizationID:       orgID,
		ProviderType:         "SAML2",
		IdpEntityID:          "http://www.okta.com/exk123456",
		IdpSSOURL:            "https://company.okta.com/app/scandrix/sso/saml",
		EncryptedCertificate: "cipher:aes-256-gcm:encrypted_cert_bytes",
		AttributeMapping: domain.JSONBMap{
			"email":     "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress",
			"firstName": "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/givenname",
		},
		AllowIdpInitiated: false,
		EnforceSSO:        true,
		IsActive:          true,
	}

	assert.Equal(t, "SAML2", sso.ProviderType)
	assert.True(t, sso.IsActive)
	assert.True(t, sso.EnforceSSO)
	assert.Equal(t, orgID, sso.OrganizationID)
}

// TestDomainValidation_WorkflowStateMachines validates lifecycle transitions for background jobs.
func TestDomainValidation_WorkflowStateMachines(t *testing.T) {
	wsID := uuid.New()
	jobID := uuid.New()

	job := &domain.WorkflowJob{
		TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsID},
		CorrelationID:      fmt.Sprintf("corr-%s", uuid.NewString()),
		WorkflowType:       string(domain.WorkflowTypeCodeReview),
		HandlerType:        string(domain.HandlerTypePipelineSync),
		Payload: domain.JSONBMap{
			"repoId":   uuid.NewString(),
			"prNumber": float64(88),
		},
		Status:     string(domain.JobStatusPending),
		Priority:   10,
		RetryCount: 0,
		MaxRetries: 5,
	}

	assert.Equal(t, string(domain.JobStatusPending), job.Status)
	assert.Equal(t, 5, job.MaxRetries)

	// Transition PENDING -> PROCESSING
	job.Status = string(domain.JobStatusProcessing)
	assert.Equal(t, string(domain.JobStatusProcessing), job.Status)

	// Transition PROCESSING -> WAITING_FOR_EVENT
	job.Status = string(domain.JobStatusWaitingForEvent)
	assert.Equal(t, string(domain.JobStatusWaitingForEvent), job.Status)

	// Transition WAITING_FOR_EVENT -> COMPLETED
	job.Status = string(domain.JobStatusCompleted)
	completedAt := time.Now().UTC()
	job.CompletedAt = &completedAt
	assert.Equal(t, string(domain.JobStatusCompleted), job.Status)
	assert.NotNil(t, job.CompletedAt)

	// Outbox & Inbox Record invariants
	outbox := &domain.OutboxRecord{
		TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsID},
		EventType:          "review.completed",
		Payload:            `{"jobId":"123","status":"COMPLETED"}`,
		Status:             string(domain.OutboxStatusReady),
		RetryCount:         0,
		MaxRetries:         10,
	}
	assert.Equal(t, string(domain.OutboxStatusReady), outbox.Status)

	inbox := &domain.InboxRecord{
		TenantScopedEntity: domain.TenantScopedEntity{WorkspaceID: wsID},
		MessageID:          fmt.Sprintf("msg-%s", jobID),
		HandlerType:        string(domain.HandlerTypePipelineSync),
		PayloadHash:        "sha256:abcdeffedcba",
		Status:             string(domain.InboxStatusReady),
	}
	assert.Equal(t, string(domain.InboxStatusReady), inbox.Status)
}
