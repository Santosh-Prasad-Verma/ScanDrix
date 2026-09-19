package migrations

// AllMigrations returns all compiled database migrations in ascending order.
func AllMigrations() []Migration {
	return []Migration{
		&Migration001InitialSchema{},
		&Migration002PgvectorSecurityMemory{},
		&Migration003IssuesAndAutomations{},
		&Migration004ExtendedWarehouseAndBilling{},
		&Migration005TeamsParametersAuditAndIntegrations{},
		&Migration006BillingTransactions{},
		&Migration007PlanConfigurations{},
		&Migration008FixRlsForceAndEmbeddings{},
		&Migration009ReviewAttestations{},
		&Migration010RlsForceHardening{},
		&Migration011TeamCliKeys{},
		&Migration012SystemWorkerOutboxRls{},
		&Migration013InboxEvents{},
		&Migration014CliAuthSessionsAlignment{},
		&Migration015AppUserRlsHardening{},
		&Migration016UsersAndAuthTables{},
		&Migration017AlignDrixyEmbeddings{},
		&Migration018WorkspacesRls{},
		&Migration019FixDrixyRlsAndHnsw{},
		&Migration020CliDevices{},
		&Migration021SystemBypassUsersRls{},
		&Migration022McpManagerSchema{},
		&Migration023OrganizationParameters{},
		&Migration024DrixyRulesAndLikes{},
		&Migration025UserRepositoryAssignments{},
		&Migration026SandboxLeases{},
		&Migration027GlobalParameters{},
		&Migration028PlatformPullRequests{},
		&Migration029ParametersAndTeamMembersAlignment{},
	}
}

// RegisterAll registers all compiled migrations into the supplied MigrationRunner.
func RegisterAll(runner *MigrationRunner) {
	for _, m := range AllMigrations() {
		runner.Register(m)
	}
}
