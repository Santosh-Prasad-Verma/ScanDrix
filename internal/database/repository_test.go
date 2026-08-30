package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth/cliauth"
	"github.com/scandrix/backend/internal/automation"
	"github.com/scandrix/backend/internal/codeanalysis/graph"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/issues"
)

func TestRepositoryNilClientGuards(t *testing.T) {
	ctx := context.Background()
	repo := database.NewRepository(nil)

	// 1. AST Graph persistence nil guards
	repoID := uuid.New()
	err := repo.BatchInsertASTNodes(ctx, repoID, []graph.ASTNode{
		{ID: uuid.New(), SymbolName: "AuthHandler"},
	})
	if err != nil {
		t.Fatalf("expected nil error on nil repo client for batch insert nodes, got: %v", err)
	}

	err = repo.BatchInsertASTEdges(ctx, repoID, []graph.ASTEdge{
		{ID: uuid.New(), FromNodeID: uuid.New(), ToNodeID: uuid.New()},
	})
	if err != nil {
		t.Fatalf("expected nil error on nil repo client for batch insert edges, got: %v", err)
	}

	_, err = repo.GetASTNodesByRepository(ctx, repoID)
	if err == nil {
		t.Fatal("expected error querying nodes on nil repo client")
	}

	_, err = repo.GetASTCallers(ctx, repoID, uuid.New())
	if err == nil {
		t.Fatal("expected error querying callers on nil repo client")
	}

	// 2. Auth persistence nil guards
	_, err = repo.GetUserByEmail(ctx, "test@scandrix.dev")
	if err == nil {
		t.Fatalf("expected error for non-existent user, got nil")
	}

	_, err = repo.CreateUser(ctx, "test@scandrix.dev", "hash", "owner", nil)
	if err == nil {
		t.Fatalf("expected error for unmocked pool, got nil")
	}

	_, err = repo.GetRefreshToken(ctx, "token-123")
	if err == nil {
		t.Fatal("expected error getting refresh token on nil repo client")
	}

	err = repo.CreateRefreshToken(ctx, uuid.New(), "token-123", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("expected nil error on nil client for CreateRefreshToken, got: %v", err)
	}

	err = repo.MarkRefreshTokenUsed(ctx, "token-123")
	if err != nil {
		t.Fatalf("expected nil error on nil client for MarkRefreshTokenUsed, got: %v", err)
	}

	err = repo.SaveAPIKey(ctx, uuid.New(), uuid.New(), "CLI Key", "hash", "kodus_cli_", nil)
	if err != nil {
		t.Fatalf("expected nil error on nil client for SaveAPIKey, got: %v", err)
	}

	// 3. Issues & Automations nil guards
	err = repo.CreateTrackedIssue(ctx, &issues.TrackedIssue{
		ID:          uuid.New(),
		WorkspaceID: uuid.New(),
	})
	if err != nil {
		t.Fatalf("expected graceful nil on nil client for CreateTrackedIssue, got: %v", err)
	}

	err = repo.CreateAutomationRule(ctx, &automation.AutomationRule{
		ID:          uuid.New(),
		WorkspaceID: uuid.New(),
	})
	if err != nil {
		t.Fatalf("expected graceful nil on nil client for CreateAutomationRule, got: %v", err)
	}

	// 4. CLI session nil guards
	err = repo.CreateCLISession(ctx, &cliauth.CLIDeviceSession{UUID: uuid.New()})
	if err != nil {
		t.Fatalf("expected graceful nil on nil client for CreateCLISession, got: %v", err)
	}

	_, err = repo.GetCLISessionByDeviceCode(ctx, "dev123")
	if err == nil {
		t.Fatal("expected error getting CLI session on nil client")
	}

	_, err = repo.GetCLISessionByUserCode(ctx, "USER123")
	if err == nil {
		t.Fatal("expected error getting CLI session on nil client")
	}

	err = repo.CompleteCLISession(ctx, "USER123", "acc", "ref", uuid.New(), "test@scandrix.dev")
	if err != nil {
		t.Fatalf("expected graceful nil on nil client for CompleteCLISession, got: %v", err)
	}

	err = repo.ConsumeCLISession(ctx, uuid.New())
	if err != nil {
		t.Fatalf("expected graceful nil on nil client for ConsumeCLISession, got: %v", err)
	}

	// 5. Password reset nil guards
	err = repo.UpdateUserPassword(ctx, "test@scandrix.dev", "newhash")
	if err != nil {
		t.Fatalf("expected graceful nil on nil client for UpdateUserPassword, got: %v", err)
	}

	err = repo.InvalidateAllUserRefreshTokens(ctx, uuid.New())
	if err != nil {
		t.Fatalf("expected graceful nil on nil client for InvalidateAllUserRefreshTokens, got: %v", err)
	}
}
