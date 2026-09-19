package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/pkg/models"
)

func TestPasswordHashingAndVerification(t *testing.T) {
	password := "CorrectHorseBatteryStaple123!"

	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("failed hashing password: %v", err)
	}

	if !auth.VerifyPassword(password, hash) {
		t.Fatal("expected correct password to verify successfully")
	}

	if auth.VerifyPassword("WrongPassword456!", hash) {
		t.Fatal("expected incorrect password verification to fail")
	}

	if auth.VerifyPassword("", hash) {
		t.Fatal("expected empty password to fail")
	}
}

func TestPasswordCostAndNeedsRehash(t *testing.T) {
	origAlgo := auth.GetHashAlgo()
	defer auth.SetHashAlgo(origAlgo)
	auth.SetHashAlgo(auth.AlgoBcrypt)

	// Set to MinCost during test for speed, then restore
	origCost := auth.GetBcryptCost()
	defer auth.SetBcryptCost(origCost)

	auth.SetBcryptCost(12)
	if auth.GetBcryptCost() != 12 {
		t.Fatalf("expected cost 12, got %d", auth.GetBcryptCost())
	}

	// Speed up hash creation for test using cost 4 (MinCost) and 5
	auth.SetBcryptCost(5)
	newHash, err := auth.HashPassword("TestPassword123!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Now raise configured cost to 6 -> newHash should need rehash
	auth.SetBcryptCost(6)
	if !auth.NeedsRehash(newHash) {
		t.Fatal("expected hash with cost 5 to need rehash when configured cost is 6")
	}

	// Lower configured cost to 4 -> newHash should NOT need rehash
	auth.SetBcryptCost(4)
	if auth.NeedsRehash(newHash) {
		t.Fatal("expected hash with cost 5 to not need rehash when configured cost is 4")
	}

	// Empty hash or invalid string should return false safely
	if auth.NeedsRehash("") {
		t.Fatal("empty hash should not need rehash")
	}
	if auth.NeedsRehash("invalid-not-bcrypt") {
		t.Fatal("invalid hash should not need rehash")
	}
}

func TestPasswordMaxLengthExceeded(t *testing.T) {
	origAlgo := auth.GetHashAlgo()
	defer auth.SetHashAlgo(origAlgo)
	auth.SetHashAlgo(auth.AlgoBcrypt)

	// Exactly 72 bytes should succeed in bcrypt mode
	pwd72 := "123456789012345678901234567890123456789012345678901234567890123456789012"
	_, err72 := auth.HashPassword(pwd72)
	if err72 != nil {
		t.Fatalf("expected 72-byte password to hash successfully, got %v", err72)
	}

	// 73 bytes must return ErrPasswordTooLong in bcrypt mode
	pwd73 := pwd72 + "x"
	_, err73 := auth.HashPassword(pwd73)
	if err73 == nil {
		t.Fatal("expected 73-byte password to be rejected")
	}
	if !errors.Is(err73, auth.ErrPasswordTooLong) {
		t.Fatalf("expected ErrPasswordTooLong, got %v", err73)
	}
}

func TestArgon2idHashingAndRehash(t *testing.T) {
	origAlgo := auth.GetHashAlgo()
	defer auth.SetHashAlgo(origAlgo)
	auth.SetHashAlgo(auth.AlgoArgon2id)

	origParams := auth.GetArgon2Params()
	defer auth.SetArgon2Params(origParams)

	// Configure fast test parameters
	auth.SetArgon2Params(auth.Argon2Params{
		Memory:      8 * 1024,
		Iterations:  1,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	})

	pw := "MySuperSecurePassword2026!"
	hash, err := auth.HashPassword(pw)
	if err != nil {
		t.Fatalf("failed generating Argon2id hash: %v", err)
	}

	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("expected $argon2id$ prefix, got %s", hash)
	}

	if !auth.VerifyPassword(pw, hash) {
		t.Fatal("expected Argon2id hash to verify correctly")
	}
	if auth.VerifyPassword("WrongPassword!", hash) {
		t.Fatal("expected wrong password to fail Argon2id verification")
	}

	// Upgrading memory should flag NeedsRehash
	auth.SetArgon2Params(auth.Argon2Params{
		Memory:      16 * 1024,
		Iterations:  1,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	})
	if !auth.NeedsRehash(hash) {
		t.Fatal("expected hash to need rehash when configured memory is higher")
	}
}

func TestTransparentBcryptToArgon2idUpgrade(t *testing.T) {
	origAlgo := auth.GetHashAlgo()
	defer auth.SetHashAlgo(origAlgo)

	// 1. Generate legacy bcrypt hash
	auth.SetHashAlgo(auth.AlgoBcrypt)
	pw := "EnterprisePassword123!"
	bcryptHash, err := auth.HashPassword(pw)
	if err != nil {
		t.Fatalf("failed generating bcrypt hash: %v", err)
	}

	// 2. Switch active system algorithm to Argon2id
	auth.SetHashAlgo(auth.AlgoArgon2id)

	// 3. Verify that old bcrypt hash STILL verifies successfully!
	if !auth.VerifyPassword(pw, bcryptHash) {
		t.Fatal("expected legacy bcrypt hash to verify successfully under Argon2id system")
	}

	// 4. Verify that NeedsRehash triggers transparent upgrade
	if !auth.NeedsRehash(bcryptHash) {
		t.Fatal("expected legacy bcrypt hash to trigger NeedsRehash=true under Argon2id system")
	}

	// 5. Upgrade hash to Argon2id
	upgradedHash, err := auth.HashPassword(pw)
	if err != nil {
		t.Fatalf("failed upgrading to Argon2id: %v", err)
	}
	if !strings.HasPrefix(upgradedHash, "$argon2id$") {
		t.Fatalf("expected upgraded hash to be Argon2id, got %s", upgradedHash)
	}
	if !auth.VerifyPassword(pw, upgradedHash) {
		t.Fatal("expected upgraded Argon2id hash to verify")
	}
}

func TestDummyVerifyAntiEnumeration(t *testing.T) {
	// Must return false without panic under both bcrypt and argon2id
	auth.SetHashAlgo(auth.AlgoBcrypt)
	if auth.DummyVerify("candidatePassword") {
		t.Fatal("expected DummyVerify to return false in bcrypt mode")
	}

	auth.SetHashAlgo(auth.AlgoArgon2id)
	if auth.DummyVerify("candidatePassword") {
		t.Fatal("expected DummyVerify to return false in argon2id mode")
	}
}

func TestCorruptedHashVerification(t *testing.T) {
	// Corrupted or malformed hash string must return false safely without panic
	corruptedHashes := []string{
		"not-a-valid-hash",
		"$2a$invalid-format",
		"$2a$12$tooshort",
	}

	for _, badHash := range corruptedHashes {
		if auth.VerifyPassword("password", badHash) {
			t.Fatalf("expected corrupted hash %q to fail verification", badHash)
		}
	}
}

func TestJWTSigningAndVerification(t *testing.T) {
	jwtSecret := "super-secure-test-jwt-secret-key-32b"
	authenticator := auth.NewAuthenticator(jwtSecret)

	userID := uuid.New()
	wsID := uuid.New()

	// 1. Generate valid token
	token, err := authenticator.GenerateToken(userID, wsID, models.RoleOwner)
	if err != nil {
		t.Fatalf("failed generating token: %v", err)
	}

	// 2. Verify valid token
	claims, err := authenticator.VerifyToken(token)
	if err != nil {
		t.Fatalf("expected valid token verification, got: %v", err)
	}

	if claims.UserID != userID || claims.WorkspaceID != wsID || claims.Role != models.RoleOwner {
		t.Fatalf("claims mismatch: got %+v", claims)
	}

	// 3. Reject tampered signature
	tamperedToken := token + "tamper"
	_, err = authenticator.VerifyToken(tamperedToken)
	if err == nil {
		t.Fatal("expected verification failure for tampered token")
	}

	// 4. Reject token signed with wrong secret
	otherAuth := auth.NewAuthenticator("wrong-jwt-secret-key-00000000000")
	_, err = otherAuth.VerifyToken(token)
	if err == nil {
		t.Fatal("expected verification failure for wrong secret")
	}

	// 5. Reject alg: none attack token
	noneHeader := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0"
	nonePayload := "eyJzdWIiOiIxMjM0NTY3OC0xMjM0LTEyMzQtMTIzNC0xMjM0NTY3ODkwMTIiLCJleHAiOjk5OTk5OTk5OTl9"
	noneToken := noneHeader + "." + nonePayload + "."
	_, err = authenticator.VerifyToken(noneToken)
	if err == nil {
		t.Fatal("expected verification failure for alg:none token")
	}
}

func TestGenerateTokenPair(t *testing.T) {
	authenticator := auth.NewAuthenticator("test-jwt-secret-key-123456789012")
	userID := uuid.New()
	wsID := uuid.New()

	access, refresh, err := authenticator.GenerateTokenPair(userID, wsID, models.RoleMember)
	if err != nil {
		t.Fatalf("failed generating token pair: %v", err)
	}

	if access == "" || refresh == "" {
		t.Fatal("expected non-empty access and refresh tokens")
	}

	if len(refresh) != 64 { // 32 bytes hex encoded
		t.Fatalf("expected 64-character hex refresh token, got %d chars", len(refresh))
	}

	claims, err := authenticator.VerifyToken(access)
	if err != nil {
		t.Fatalf("failed verifying access token from pair: %v", err)
	}
	if claims.Role != models.RoleMember {
		t.Fatalf("expected RoleMember, got %s", claims.Role)
	}
}

func TestWorkspaceContextAndHashing(t *testing.T) {
	wsID := uuid.New()
	ctx := auth.WithWorkspaceContext(context.Background(), wsID)

	extractedID, err := auth.WorkspaceFromContext(ctx)
	if err != nil {
		t.Fatalf("expected valid workspace context, got error: %v", err)
	}
	if extractedID != wsID {
		t.Errorf("expected %s, got %s", wsID, extractedID)
	}

	rawKey := "scandrix_live_9f823a9e"
	hashed := auth.HashAPIKey(rawKey)
	if len(hashed) != 64 {
		t.Errorf("expected 64-char sha256 hex string, got len %d", len(hashed))
	}

	if !auth.ConstantTimeCompare("secret123", "secret123") {
		t.Error("expected equal strings to return true")
	}
	if auth.ConstantTimeCompare("secret123", "wrongsecret") {
		t.Error("expected unequal strings to return false")
	}
}

func TestRoleGuardHierarchy(t *testing.T) {
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	guardedMember := auth.RoleGuard(models.RoleMember, testHandler)
	guardedAdmin := auth.RoleGuard(models.RoleAdmin, testHandler)

	tests := []struct {
		name         string
		role         models.UserRole
		guard        http.HandlerFunc
		expectedCode int
	}{
		{"Owner accesses Member route", models.RoleOwner, guardedMember, http.StatusOK},
		{"Admin accesses Member route", models.RoleAdmin, guardedMember, http.StatusOK},
		{"Member accesses Member route", models.RoleMember, guardedMember, http.StatusOK},
		{"Viewer denied Member route", models.RoleViewer, guardedMember, http.StatusForbidden},
		{"Admin accesses Admin route", models.RoleAdmin, guardedAdmin, http.StatusOK},
		{"Owner accesses Admin route", models.RoleOwner, guardedAdmin, http.StatusOK},
		{"Member denied Admin route", models.RoleMember, guardedAdmin, http.StatusForbidden},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			ctx := auth.WithAccountContext(req.Context(), &models.AccountProfile{
				ID:   uuid.New(),
				Role: tc.role,
			})
			rec := httptest.NewRecorder()
			tc.guard.ServeHTTP(rec, req.WithContext(ctx))

			if rec.Code != tc.expectedCode {
				t.Fatalf("expected status %d, got %d", tc.expectedCode, rec.Code)
			}
		})
	}
}

func TestTokenRevocationCheck(t *testing.T) {
	secret := "test-revocation-secret-32-chars-long!"
	authenticator := auth.NewAuthenticator(secret)
	userID := uuid.New()
	wsID := uuid.New()

	token, err := authenticator.GenerateToken(userID, wsID, models.RoleMember)
	if err != nil {
		t.Fatalf("failed generating token: %v", err)
	}

	// 1. Without revocation checker, token is valid
	claims, err := authenticator.VerifyToken(token)
	if err != nil {
		t.Fatalf("expected valid token, got: %v", err)
	}

	// 2. Set revocation checker that revokes this user's tokens
	revokedUserID := userID
	authenticator.SetRevocationChecker(func(ctx context.Context, uID uuid.UUID, issuedAt int64) bool {
		return uID == revokedUserID
	})

	handler := authenticator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected HTTP 401 for revoked token, got %d", rec.Code)
	}

	// 3. For another user, token is not revoked
	otherUserID := uuid.New()
	tokenOther, _ := authenticator.GenerateToken(otherUserID, wsID, models.RoleMember)
	reqOther := httptest.NewRequest(http.MethodGet, "/api/v1/protected", nil)
	reqOther.Header.Set("Authorization", "Bearer "+tokenOther)
	recOther := httptest.NewRecorder()

	handler.ServeHTTP(recOther, reqOther)

	if recOther.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 for non-revoked token, got %d", recOther.Code)
	}
	_ = claims
}

func TestAuthenticatorEphemeralKeyOnEmptySecret(t *testing.T) {
	// When initialized with empty string, Authenticator must generate a secure 32-byte ephemeral key
	auth1 := auth.NewAuthenticator("")
	auth2 := auth.NewAuthenticator("")

	userID := uuid.New()
	wsID := uuid.New()
	token1, refresh1, err := auth1.GenerateTokenPairWithEmail(userID, wsID, models.RoleOwner, "dev@example.com")
	if err != nil {
		t.Fatalf("unexpected error generating token pair: %v", err)
	}

	// Token should verify on auth1
	claims, err := auth1.VerifyToken(token1)
	if err != nil {
		t.Fatalf("expected token1 to verify on auth1, got: %v", err)
	}
	if claims.UserID != userID || claims.Email != "dev@example.com" {
		t.Fatalf("unexpected claims: %+v", claims)
	}

	// Token should NOT verify on auth2 (each generates distinct cryptographically random ephemeral secrets)
	_, err = auth2.VerifyToken(token1)
	if err == nil {
		t.Fatal("expected token from auth1 to fail verification on auth2 due to distinct ephemeral keys")
	}

	// Refresh token should be 64-char hex string (32 bytes entropy)
	if len(refresh1) != 64 {
		t.Fatalf("expected 64-character refresh token hex string, got %d (%s)", len(refresh1), refresh1)
	}
}

