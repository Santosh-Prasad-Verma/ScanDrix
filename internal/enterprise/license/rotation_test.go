package license_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/enterprise/license"
)

func newKeyPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generating keypair: %v", err)
	}
	return pub, priv
}

func validPayload(t *testing.T) license.LicensePayload {
	t.Helper()
	return license.LicensePayload{
		LicenseID:    uuid.New(),
		CustomerName: "Rotation Test Corp",
		CustomerID:   uuid.New().String(),
		Tier:         license.TierEnterprise,
		IssuedAt:     time.Now().UTC(),
		ExpiresAt:    time.Now().UTC().Add(24 * time.Hour),
		MaxSeats:     50,
		Features:     []string{string(license.FeatureSSOSAML)},
	}
}

// A license minted before KeyID existed carries no key ID and must keep
// verifying against the primary key.
func TestLegacyPayloadWithoutKeyIDStillLoads(t *testing.T) {
	pub, priv := newKeyPair(t)
	mgr := license.NewLicenseManager(pub)

	token, err := license.IssueLicense(validPayload(t), priv)
	if err != nil {
		t.Fatalf("issuing license: %v", err)
	}

	loaded, err := mgr.LoadLicense(token)
	if err != nil {
		t.Fatalf("legacy token without key_id must still load: %v", err)
	}
	if loaded.KeyID != "" {
		t.Fatalf("expected empty key_id, got %q", loaded.KeyID)
	}
}

// The rotation scenario: an old key signs existing licenses, a new key signs
// renewals. While both are registered every license in the field must verify.
func TestRotationRingAcceptsBothOldAndNewKeys(t *testing.T) {
	oldPub, oldPriv := newKeyPair(t)
	newPub, newPriv := newKeyPair(t)

	mgr := license.NewLicenseManager(oldPub)
	if err := mgr.AddVerificationKey("2026-10", newPub); err != nil {
		t.Fatalf("registering rotation key: %v", err)
	}

	// License issued under the outgoing key, with no key ID.
	oldToken, err := license.IssueLicense(validPayload(t), oldPriv)
	if err != nil {
		t.Fatalf("issuing old-key license: %v", err)
	}
	if _, err := mgr.LoadLicense(oldToken); err != nil {
		t.Fatalf("old-key license must verify during the overlap: %v", err)
	}

	// Renewal issued under the incoming key, naming it.
	renewal := validPayload(t)
	renewal.KeyID = "2026-10"
	newToken, err := license.IssueLicense(renewal, newPriv)
	if err != nil {
		t.Fatalf("issuing new-key license: %v", err)
	}
	loaded, err := mgr.LoadLicense(newToken)
	if err != nil {
		t.Fatalf("new-key license must verify once the ring is registered: %v", err)
	}
	if loaded.KeyID != "2026-10" {
		t.Fatalf("expected key_id 2026-10, got %q", loaded.KeyID)
	}

	if ids := mgr.VerificationKeyIDs(); len(ids) != 1 || ids[0] != "2026-10" {
		t.Fatalf("expected ring [2026-10], got %v", ids)
	}
}

// Retiring the old key must stop the licenses it signed from verifying, which
// is what makes rotation able to revoke an old keypair deliberately.
func TestRetiringOldKeyStopsItsLicensesVerifying(t *testing.T) {
	oldPub, oldPriv := newKeyPair(t)
	newPub, newPriv := newKeyPair(t)

	overlap := license.NewLicenseManager(oldPub)
	if err := overlap.AddVerificationKey("2026-10", newPub); err != nil {
		t.Fatalf("registering rotation key: %v", err)
	}

	oldToken, err := license.IssueLicense(validPayload(t), oldPriv)
	if err != nil {
		t.Fatalf("issuing old-key license: %v", err)
	}

	// After retirement the outgoing key is gone: only the new key remains.
	postRetirement := license.NewLicenseManager(newPub)
	if err := postRetirement.AddVerificationKey("2026-10", newPub); err != nil {
		t.Fatalf("registering rotation key: %v", err)
	}
	if _, err := postRetirement.LoadLicense(oldToken); err == nil {
		t.Fatal("license signed by the retired key must no longer verify")
	}

	renewal := validPayload(t)
	renewal.KeyID = "2026-10"
	newToken, err := license.IssueLicense(renewal, newPriv)
	if err != nil {
		t.Fatalf("issuing new-key license: %v", err)
	}
	if _, err := postRetirement.LoadLicense(newToken); err != nil {
		t.Fatalf("new-key license must still verify after retirement: %v", err)
	}
}

// An unknown key ID must fail closed with a message naming it, so an operator
// can tell "key not distributed yet" apart from "signature is wrong".
func TestUnknownKeyIDFailsClosed(t *testing.T) {
	pub, priv := newKeyPair(t)
	mgr := license.NewLicenseManager(pub)

	payload := validPayload(t)
	payload.KeyID = "2099-01"
	token, err := license.IssueLicense(payload, priv)
	if err != nil {
		t.Fatalf("issuing license: %v", err)
	}

	_, err = mgr.LoadLicense(token)
	if err == nil {
		t.Fatal("expected failure for an unregistered key ID")
	}
	if !strings.Contains(err.Error(), "2099-01") {
		t.Fatalf("error should name the missing key ID, got: %v", err)
	}
}

// Selecting a key by an unverified key ID must never let an attacker skip the
// signature check: swapping the payload while keeping a valid key ID fails.
func TestKeyIDSelectionDoesNotBypassSignature(t *testing.T) {
	attackerPub, attackerPriv := newKeyPair(t)
	vendorPub, _ := newKeyPair(t)

	mgr := license.NewLicenseManager(vendorPub)
	if err := mgr.AddVerificationKey("2026-10", attackerPub); err != nil {
		t.Fatalf("registering rotation key: %v", err)
	}

	// Attacker signs with their own key but claims the vendor's ring key ID.
	payload := validPayload(t)
	payload.KeyID = "2026-10"
	payload.MaxSeats = 99999
	token, err := license.IssueLicense(payload, attackerPriv)
	if err != nil {
		t.Fatalf("issuing forged license: %v", err)
	}

	// Registering the ring key with the vendor's public key is what a real
	// deployment does; the forged signature must not verify.
	if err := mgr.AddVerificationKey("2026-10", vendorPub); err != nil {
		t.Fatalf("registering rotation key: %v", err)
	}
	if _, err := mgr.LoadLicense(token); err == nil {
		t.Fatal("forged license must not verify after the ring key is corrected")
	}
}

func TestHardwareFingerprintEnforcedOnlyWhenExpected(t *testing.T) {
	pub, priv := newKeyPair(t)

	bound := validPayload(t)
	bound.HardwareFingerprint = "machine-a"
	boundToken, err := license.IssueLicense(bound, priv)
	if err != nil {
		t.Fatalf("issuing bound license: %v", err)
	}

	unbound := validPayload(t)
	unboundToken, err := license.IssueLicense(unbound, priv)
	if err != nil {
		t.Fatalf("issuing unbound license: %v", err)
	}

	t.Run("matching fingerprint loads", func(t *testing.T) {
		mgr := license.NewLicenseManager(pub)
		mgr.SetExpectedFingerprint("machine-a")
		if _, err := mgr.LoadLicense(boundToken); err != nil {
			t.Fatalf("matching fingerprint must load: %v", err)
		}
	})

	t.Run("mismatched fingerprint is rejected", func(t *testing.T) {
		mgr := license.NewLicenseManager(pub)
		mgr.SetExpectedFingerprint("machine-b")
		_, err := mgr.LoadLicense(boundToken)
		if err == nil {
			t.Fatal("license bound to another machine must be rejected")
		}
		if !strings.Contains(err.Error(), "hardware fingerprint") {
			t.Fatalf("error should mention the fingerprint binding, got: %v", err)
		}
	})

	t.Run("unbound license rejected when a binding is expected", func(t *testing.T) {
		mgr := license.NewLicenseManager(pub)
		mgr.SetExpectedFingerprint("machine-a")
		if _, err := mgr.LoadLicense(unboundToken); err == nil {
			t.Fatal("an unbound license must not satisfy a required binding")
		}
	})

	t.Run("no expected fingerprint leaves binding unenforced", func(t *testing.T) {
		mgr := license.NewLicenseManager(pub)
		if _, err := mgr.LoadLicense(boundToken); err != nil {
			t.Fatalf("unconfigured deployments must still load bound licenses: %v", err)
		}
		if _, err := mgr.LoadLicense(unboundToken); err != nil {
			t.Fatalf("unconfigured deployments must still load unbound licenses: %v", err)
		}
	})
}

func TestAddVerificationKeyRejectsBadInput(t *testing.T) {
	mgr := license.NewLicenseManager(nil)

	if err := mgr.AddVerificationKey("", make([]byte, ed25519.PublicKeySize)); err == nil {
		t.Fatal("an empty key ID must be rejected: it is reserved for the primary key")
	}
	if err := mgr.AddVerificationKey("2026-10", []byte("too-short")); err == nil {
		t.Fatal("a wrong-sized key must be rejected at configuration time")
	}
}

func TestLoadLicenseWithoutConfiguredKeyFailsClosed(t *testing.T) {
	_, priv := newKeyPair(t)
	token, err := license.IssueLicense(validPayload(t), priv)
	if err != nil {
		t.Fatalf("issuing license: %v", err)
	}

	mgr := license.NewLicenseManager(nil)
	if _, err := mgr.LoadLicense(token); err == nil {
		t.Fatal("a server with no verification key must not activate a license")
	}
}

// Grace is enforced from the single shared constant, and the window is the
// documented 7 days.
func TestGracePeriodIsSevenDays(t *testing.T) {
	if license.LicenseGracePeriod != 7*24*time.Hour {
		t.Fatalf("grace period is %s, expected 168h0m0s (7 days)", license.LicenseGracePeriod)
	}

	pub, priv := newKeyPair(t)
	mgr := license.NewLicenseManager(pub)

	expiredInsideGrace := validPayload(t)
	expiredInsideGrace.ExpiresAt = time.Now().UTC().Add(-license.LicenseGracePeriod + time.Hour)
	insideToken, err := license.IssueLicense(expiredInsideGrace, priv)
	if err != nil {
		t.Fatalf("issuing license: %v", err)
	}
	if _, err := mgr.LoadLicense(insideToken); err != nil {
		t.Fatalf("a license inside the grace window must still load: %v", err)
	}

	expiredBeyondGrace := validPayload(t)
	expiredBeyondGrace.ExpiresAt = time.Now().UTC().Add(-license.LicenseGracePeriod - time.Hour)
	beyondToken, err := license.IssueLicense(expiredBeyondGrace, priv)
	if err != nil {
		t.Fatalf("issuing license: %v", err)
	}
	if _, err := mgr.LoadLicense(beyondToken); err == nil {
		t.Fatal("a license past the grace window must be rejected")
	}
}

// Environment wiring: absent public key is a legitimate Community boot, while a
// malformed ring or an unverifiable token must fail startup loudly.
func TestNewManagerFromEnv(t *testing.T) {
	pub, priv := newKeyPair(t)
	pubB64 := base64Std(pub)

	t.Run("absent public key boots on community", func(t *testing.T) {
		clearLicenseEnv(t)
		mgr, err := license.NewManagerFromEnv()
		if err != nil {
			t.Fatalf("an unlicensed deployment must boot: %v", err)
		}
		if mgr.Entitlement().Tier != license.TierCommunity {
			t.Fatalf("expected community tier, got %s", mgr.Entitlement().Tier)
		}
	})

	t.Run("malformed public key fails startup", func(t *testing.T) {
		clearLicenseEnv(t)
		t.Setenv(license.EnvPublicKey, "not-a-real-key")
		if _, err := license.NewManagerFromEnv(); err == nil {
			t.Fatal("a malformed public key must fail startup rather than run unlicensed")
		}
	})

	t.Run("rotation ring registers", func(t *testing.T) {
		clearLicenseEnv(t)
		t.Setenv(license.EnvPublicKey, pubB64)
		t.Setenv(license.EnvKeyRing, "2026-10:"+pubB64)

		mgr, err := license.NewManagerFromEnv()
		if err != nil {
			t.Fatalf("building manager: %v", err)
		}
		if ids := mgr.VerificationKeyIDs(); len(ids) != 1 || ids[0] != "2026-10" {
			t.Fatalf("expected ring [2026-10], got %v", ids)
		}
	})

	// Regression: a ring-only configuration used to return early on the missing
	// primary key and silently boot on Community, ignoring the ring entirely.
	// That is a valid deployment shape - the primary key may already be retired
	// while licenses naming ring keys are still in the field.
	t.Run("ring without primary key is honoured", func(t *testing.T) {
		clearLicenseEnv(t)
		t.Setenv(license.EnvKeyRing, "2026-10:"+pubB64)

		payload := validPayload(t)
		payload.KeyID = "2026-10"
		token, err := license.IssueLicense(payload, priv)
		if err != nil {
			t.Fatalf("issuing license: %v", err)
		}
		t.Setenv(license.EnvLicenseKey, token)

		mgr, err := license.NewManagerFromEnv()
		if err != nil {
			t.Fatalf("building manager: %v", err)
		}
		if got := mgr.Entitlement().Tier; got != license.TierEnterprise {
			t.Fatalf("ring-only config must resolve the signed license, got tier %s", got)
		}
	})

	// A token configured with no verification material at all must fail boot
	// rather than quietly downgrade the deployment to Community.
	t.Run("token without any key fails startup", func(t *testing.T) {
		clearLicenseEnv(t)
		t.Setenv(license.EnvLicenseKey, "some-token")

		if _, err := license.NewManagerFromEnv(); err == nil {
			t.Fatal("a configured token with no verification key must fail startup")
		}
	})

	t.Run("malformed ring entry fails startup", func(t *testing.T) {
		clearLicenseEnv(t)
		t.Setenv(license.EnvPublicKey, pubB64)
		t.Setenv(license.EnvKeyRing, "missing-separator")

		if _, err := license.NewManagerFromEnv(); err == nil {
			t.Fatal("a malformed ring entry must fail startup")
		}
	})

	t.Run("ring entry with bad key fails startup", func(t *testing.T) {
		clearLicenseEnv(t)
		t.Setenv(license.EnvPublicKey, pubB64)
		t.Setenv(license.EnvKeyRing, "2026-10:garbage")

		if _, err := license.NewManagerFromEnv(); err == nil {
			t.Fatal("an undecodable ring key must fail startup")
		}
	})

	t.Run("invalid token fails startup", func(t *testing.T) {
		clearLicenseEnv(t)
		t.Setenv(license.EnvPublicKey, pubB64)
		t.Setenv(license.EnvLicenseKey, "clearly-not-a-valid-token")

		if _, err := license.NewManagerFromEnv(); err == nil {
			t.Fatal("an unverifiable token must fail startup, not silently degrade")
		}
	})

	t.Run("valid token and fingerprint load", func(t *testing.T) {
		clearLicenseEnv(t)
		t.Setenv(license.EnvPublicKey, pubB64)
		t.Setenv(license.EnvHardwareFingerprint, "cluster-7")

		payload := validPayload(t)
		payload.HardwareFingerprint = "cluster-7"
		token, err := license.IssueLicense(payload, priv)
		if err != nil {
			t.Fatalf("issuing license: %v", err)
		}
		t.Setenv(license.EnvLicenseKey, token)

		mgr, err := license.NewManagerFromEnv()
		if err != nil {
			t.Fatalf("building manager: %v", err)
		}
		if mgr.Entitlement().Tier != license.TierEnterprise {
			t.Fatalf("expected enterprise tier, got %s", mgr.Entitlement().Tier)
		}
	})
}

func TestLicenseFileMountIsHonoured(t *testing.T) {
	pub, priv := newKeyPair(t)

	clearLicenseEnv(t)
	t.Setenv(license.EnvPublicKey, base64Std(pub))

	token, err := license.IssueLicense(validPayload(t), priv)
	if err != nil {
		t.Fatalf("issuing license: %v", err)
	}

	path := filepath.Join(t.TempDir(), "scandrix.license")
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		t.Fatalf("writing license file: %v", err)
	}
	t.Setenv(license.EnvLicenseFile, path)

	if _, err := license.NewManagerFromEnv(); err != nil {
		t.Fatalf("a mounted license file must load: %v", err)
	}
}

func clearLicenseEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		license.EnvPublicKey,
		license.EnvLicenseKey,
		license.EnvLicenseFile,
		license.EnvKeyRing,
		license.EnvHardwareFingerprint,
	} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unsetting %s: %v", key, err)
		}
	}
}

func base64Std(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}
