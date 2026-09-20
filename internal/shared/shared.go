// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Shared Domain Module
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

// Package shared provides common interfaces, blueprint execution runners,
// database connection managers, and infrastructure components across ScanDrix.
package shared

import (
	"github.com/scandrix/backend/internal/shared/blueprint"
	"github.com/scandrix/backend/internal/shared/database"
	"github.com/scandrix/backend/internal/shared/infrastructure"
)

// Re-export common types for seamless consumer access.
type (
	BlueprintContext            = blueprint.BlueprintContext
	DefaultBlueprintContext     = blueprint.DefaultContext
	StepType                    = blueprint.StepType
	StepExecutionStatus         = blueprint.StepExecutionStatus
	RunnerOptions[T any]        = blueprint.RunnerOptions[T]
	BlueprintResult[T any]      = blueprint.BlueprintResult[T]
	StepMetric                  = blueprint.StepMetric
	SharedPostgresOptions       = database.SharedPostgresOptions
	SharedMongoOptions          = database.SharedMongoOptions
	SharedDatabaseManager       = database.SharedDatabaseManager
	SharedConfig                = infrastructure.SharedConfig
	LoggerWrapperService        = infrastructure.LoggerWrapperService
	SharedObservabilityService  = infrastructure.SharedObservabilityService
)

// RunBlueprint executes a skill blueprint against an initial context.
var RunBlueprint = blueprint.RunBlueprint[*blueprint.DefaultContext]
