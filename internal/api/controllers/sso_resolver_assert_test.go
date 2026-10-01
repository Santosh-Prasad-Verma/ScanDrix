package controllers

import (
	"testing"

	"github.com/scandrix/backend/internal/database"
)

// Guards the SSO domain-routing path: if *database.Repository stops satisfying
// SSODomainResolver, /sso/check silently falls back and stops routing domains
// to the right IdP.
func TestRepositorySatisfiesSSODomainResolver(t *testing.T) {
	var repo any = &database.Repository{}
	if _, ok := repo.(SSODomainResolver); !ok {
		t.Fatal("*database.Repository must satisfy SSODomainResolver")
	}
}
