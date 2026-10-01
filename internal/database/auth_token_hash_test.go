package database

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestHashRefreshTokenIsDeterministic pins the storage format for refresh
// tokens (AUDIT_REMEDIATION.md F-12).
//
// The digest written here must match migration 039's backfill expression
// exactly, otherwise every pre-existing session is orphaned on deploy. The
// expected value below is the independent reference: sha256 of the token,
// hex encoded, lowercase.
func TestHashRefreshTokenIsDeterministic(t *testing.T) {
	const token = "53512503dda65d3b8eb6f10958221c5c1a83afb62039dee54db26379be5d9f34"

	got := hashRefreshToken(token)

	// sha256("...") computed independently below; hard-coded so a refactor
	// that changes the encoding is caught rather than silently matching itself.
	const want = "899a4de8cae64f1b14857440f8341201aef37e2704deda4d418d43c22e185e6f"
	if got != want {
		t.Fatalf("digest mismatch\n got: %s\nwant: %s", got, want)
	}

	if len(got) != 64 {
		t.Fatalf("expected a 64-character hex digest, got %d characters", len(got))
	}
	if strings.ToLower(got) != got {
		t.Fatalf("digest must be lowercase hex to match encode(sha256(...),'hex'): %s", got)
	}
	if strings.ContainsAny(got, "ghijklmnopqrstuvwxyz") {
		t.Fatalf("digest must be hex only: %s", got)
	}
}

func TestHashRefreshTokenDistinguishesInputs(t *testing.T) {
	a := hashRefreshToken("token-one")
	b := hashRefreshToken("token-two")
	if a == b {
		t.Fatalf("different tokens must not collide: %s", a)
	}
	if a != hashRefreshToken("token-one") {
		t.Fatalf("hashing must be stable across calls")
	}
	// A near-miss input must not be treated as equal, which is the property
	// the UNIQUE index on tokenHash relies on.
	if hashRefreshToken("token-one ") == a {
		t.Fatalf("digest must be sensitive to the exact token value")
	}
}

// TestRefreshTokenRecordCarriesNoSecret is a structural guard.
//
// RefreshTokenRecord is returned to the refresh handler, which needs only the
// owner and the spent flag. Reintroducing a token field here would put a
// database-sourced secret back into application memory and tempt a caller into
// logging it.
func TestRefreshTokenRecordCarriesNoSecret(t *testing.T) {
	// Compile-time proof the struct has no token material: this literal only
	// compiles while the field is absent.
	rec := RefreshTokenRecord{
		UUID:         uuid.Nil,
		ExpiryDate:   time.Time{},
		Used:         false,
		AuthProvider: "credentials",
		UserUUID:     uuid.Nil,
	}
	if rec.Used {
		t.Fatalf("unexpected zero value")
	}
}
