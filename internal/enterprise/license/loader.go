package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

// Environment variables that carry the enterprise license configuration.
const (
	// EnvPublicKey holds the base64-encoded Ed25519 vendor public key used to
	// verify license tokens. Without it no signed license can be trusted.
	EnvPublicKey = "SCANDRIX_LICENSE_PUBLIC_KEY"
	// EnvLicenseKey holds the signed license token issued by the vendor.
	EnvLicenseKey = "SCANDRIX_LICENSE_KEY"
	// EnvLicenseFile points at a license token stored on disk, for deployments
	// that mount secrets as files rather than environment variables.
	EnvLicenseFile = "SCANDRIX_LICENSE_FILE"
	// EnvKeyRing lists additional verification keys that stay valid during a
	// signing-key rotation, formatted as comma-separated "keyID:base64Key"
	// pairs. The primary key in EnvPublicKey is always valid and is not listed
	// here. A malformed entry fails startup rather than being skipped.
	EnvKeyRing = "SCANDRIX_LICENSE_KEY_RING"
	// EnvHardwareFingerprint declares the hardware or cluster identity this
	// server must match against a license's hardware_fingerprint claim. Leave
	// it empty to disable binding enforcement; licenses then load whether or
	// not they carry a fingerprint.
	EnvHardwareFingerprint = "SCANDRIX_HARDWARE_FINGERPRINT"
)

// PublicKeyFromBase64 decodes a base64-encoded Ed25519 public key. It accepts
// both standard and URL-safe alphabets, with or without PEM armour, so an
// operator can paste whichever form their keyring tool emitted.
func PublicKeyFromBase64(encoded string) (ed25519.PublicKey, error) {
	cleaned := strings.TrimSpace(encoded)
	cleaned = strings.ReplaceAll(cleaned, "\r", "")
	cleaned = strings.ReplaceAll(cleaned, "\n", "")
	cleaned = strings.TrimPrefix(cleaned, "-----BEGIN PUBLIC KEY-----")
	cleaned = strings.TrimSuffix(cleaned, "-----END PUBLIC KEY-----")
	cleaned = strings.TrimSpace(cleaned)

	if cleaned == "" {
		return nil, fmt.Errorf("%s is empty", EnvPublicKey)
	}

	var raw []byte
	var err error
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		if raw, err = enc.DecodeString(cleaned); err == nil {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("%s is not valid base64", EnvPublicKey)
	}

	switch len(raw) {
	case ed25519.PublicKeySize:
		return ed25519.PublicKey(raw), nil
	case ed25519.PrivateKeySize:
		priv := ed25519.PrivateKey(raw)
		return priv.Public().(ed25519.PublicKey), nil
	default:
		return nil, fmt.Errorf("%s decoded to %d bytes, expected %d", EnvPublicKey, len(raw), ed25519.PublicKeySize)
	}
}

// NewManagerFromEnv builds a LicenseManager from the environment.
//
// An unlicensed community deployment is a legitimate configuration: with no
// verification key and no license token, the manager is returned with a nil key
// and no error. Every other combination is treated as a licensed deployment and
// resolved strictly:
//
//   - A key or a ring means the deployment is licensed, so both are always
//     parsed. A ring on its own is valid: the primary key may already be retired
//     while ring-keyed licenses are still in the field.
//   - A token with no verification material is a misconfiguration and fails
//     startup, because silently downgrading to Community would leave an operator
//     believing the deployment is licensed when it is not.
//   - A malformed key, a malformed ring entry, or a token that fails
//     verification all fail startup loudly.
func NewManagerFromEnv() (*LicenseManager, error) {
	encodedKey := strings.TrimSpace(os.Getenv(EnvPublicKey))
	encodedRing := strings.TrimSpace(os.Getenv(EnvKeyRing))

	token, err := licenseTokenFromEnv()
	if err != nil {
		return nil, err
	}

	// No licensing material of any kind: a community deployment.
	if encodedKey == "" && encodedRing == "" {
		if token != "" {
			return nil, fmt.Errorf("%s is set but no verification key is configured (%s and %s are empty)",
				EnvLicenseKey, EnvPublicKey, EnvKeyRing)
		}
		return NewLicenseManager(nil), nil
	}

	var pubKey ed25519.PublicKey
	if encodedKey != "" {
		decoded, err := PublicKeyFromBase64(encodedKey)
		if err != nil {
			return nil, err
		}
		pubKey = decoded
	}

	manager := NewLicenseManager(pubKey)
	manager.SetExpectedFingerprint(os.Getenv(EnvHardwareFingerprint))

	if err := applyKeyRing(manager, encodedRing); err != nil {
		return nil, err
	}

	if token == "" {
		return manager, nil
	}

	if _, err := manager.LoadLicense(token); err != nil {
		return nil, fmt.Errorf("%s: %w", EnvLicenseKey, err)
	}
	return manager, nil
}

// applyKeyRing registers the rotation-ring keys from a comma-separated
// "keyID:base64Key" list. Entries are separated by commas and key IDs by the
// first colon; base64 never contains either character, so the split is
// unambiguous. A malformed entry is an error: silently dropping a ring key
// would look like a rotation outage much later, when an old license stops
// verifying in production.
func applyKeyRing(manager *LicenseManager, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		keyID, encoded, found := strings.Cut(entry, ":")
		if !found {
			return fmt.Errorf("%s: entry %q is not in \"keyID:base64Key\" format", EnvKeyRing, entry)
		}
		keyID = strings.TrimSpace(keyID)
		if keyID == "" {
			return fmt.Errorf("%s: entry %q has an empty key ID", EnvKeyRing, entry)
		}

		pubKey, err := PublicKeyFromBase64(encoded)
		if err != nil {
			return fmt.Errorf("%s: key %q: %w", EnvKeyRing, keyID, err)
		}
		if err := manager.AddVerificationKey(keyID, pubKey); err != nil {
			return fmt.Errorf("%s: %w", EnvKeyRing, err)
		}
	}
	return nil
}

func licenseTokenFromEnv() (string, error) {
	if inline := strings.TrimSpace(os.Getenv(EnvLicenseKey)); inline != "" {
		return inline, nil
	}

	path := strings.TrimSpace(os.Getenv(EnvLicenseFile))
	if path == "" {
		return "", nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("%s: %w", EnvLicenseFile, err)
	}
	return strings.TrimSpace(string(raw)), nil
}
