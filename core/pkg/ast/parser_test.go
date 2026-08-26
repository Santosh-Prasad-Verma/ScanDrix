package ast_test

import (
	"testing"

	"github.com/codehound/codehound/core/pkg/ast"
	"github.com/google/uuid"
)

func TestParseGoFile(t *testing.T) {
	p := ast.NewParser()
	fileID := uuid.New()
	code := []byte(`
package auth

type SessionManager struct {
	token string
}

func (s *SessionManager) ValidateToken(t string) bool {
	return len(t) > 0
}

func HashPassword(pw string) string {
	return "hash"
}
`)

	symbols, err := p.ParseFile(fileID, "pkg/auth/session.go", "go", code)
	if err != nil {
		t.Fatalf("failed to parse Go file: %v", err)
	}

	if len(symbols) < 3 {
		t.Fatalf("expected at least 3 symbols (Struct, Method, Function), got %d", len(symbols))
	}

	names := make(map[string]string)
	for _, s := range symbols {
		names[s.Name] = s.Kind
	}

	if names["SessionManager"] != "STRUCT" {
		t.Errorf("expected SessionManager to be STRUCT, got %s", names["SessionManager"])
	}
	if names["ValidateToken"] != "METHOD" {
		t.Errorf("expected ValidateToken to be METHOD, got %s", names["ValidateToken"])
	}
	if names["HashPassword"] != "FUNCTION" {
		t.Errorf("expected HashPassword to be FUNCTION, got %s", names["HashPassword"])
	}
}

func TestParseTypeScriptAndRoutes(t *testing.T) {
	p := ast.NewParser()
	fileID := uuid.New()
	code := []byte(`
import express from 'express';
const router = express.Router();

export class UserController {
	getUser() {}
}

export function authMiddleware(req, res, next) {
	next();
}

router.get('/api/v1/users', (req, res) => {
	res.send([]);
});
`)

	symbols, err := p.ParseFile(fileID, "src/routes/users.ts", "typescript", code)
	if err != nil {
		t.Fatalf("failed to parse TS file: %v", err)
	}

	foundRoute := false
	for _, s := range symbols {
		if s.Kind == "API_ROUTE" && s.Name == "GET /api/v1/users" {
			foundRoute = true
		}
	}

	if !foundRoute {
		t.Errorf("expected to extract API_ROUTE 'GET /api/v1/users'")
	}
}
