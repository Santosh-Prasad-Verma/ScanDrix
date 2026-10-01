// Command scandrix-keygen is the ScanDrix license authority toolkit.
//
// It is the only supported way to produce a license token, and it produces
// tokens in exactly the envelope format LicenseManager.LoadLicense accepts:
// base64 of a JSON SignedLicenseToken{Payload, Signature}.
//
// Secret handling (see the project security baseline):
//   - The authority private key is never compiled in, never logged, and never
//     written anywhere except a file this tool creates with 0600 permissions.
//   - It is read from -keyfile, -privkey, or the AUTHORITY_PRIVATE_KEY
//     environment variable, in that order of preference.
//   - The corresponding public key is not a secret: it is printed on purpose so
//     it can be distributed to servers as SCANDRIX_LICENSE_PUBLIC_KEY.
//
// Usage:
//
//	scandrix-keygen gen-keypair -out authority.key
//	scandrix-keygen issue -customer "Acme Corp" -tier ENTERPRISE -seats 100 \
//	    -days 365 -keyfile authority.key -key-id 2026-10
//	scandrix-keygen verify -token <token>
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/enterprise/license"
)

// envAuthorityPrivateKey is the environment variable an operator or CI job can
// use to supply the signing key without it appearing in a process argument
// list. Prefer -keyfile in automation; this exists for secret-manager injection.
const envAuthorityPrivateKey = "AUTHORITY_PRIVATE_KEY"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "gen-keypair":
		err = genKeypair(os.Args[2:])
	case "issue":
		err = issue(os.Args[2:])
	case "verify":
		err = verify(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "scandrix-keygen: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "scandrix-keygen: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `scandrix-keygen <gen-keypair|issue|verify>

  gen-keypair   Generate an Ed25519 authority keypair.
                  -out <path>   write the PRIVATE key to <path> with 0600 perms (required
                                unless -stdout is given, which prints it instead)
                  -stdout       print the private key to stdout instead of a file
                The public key is printed to stdout in both cases; it is not a secret.

  issue         Mint a signed license token in the format LoadLicense accepts.
                  -customer <name>     (required) customer organization name
                  -customer-id <id>    defaults to a generated UUID
                  -tier <tier>         COMMUNITY|DEVELOPER|TEAM|SCALE|ENTERPRISE
                  -seats <n>           max seats, 0 = unlimited (default 100)
                  -repos <n>           max repositories, 0 = unlimited (default 0)
                  -days <n>            validity in days (default 365)
                  -key-id <id>         rotation key identifier this token is minted under
                  -hardware <fp>       bind the license to a hardware/cluster fingerprint
                  -features <a,b,c>    defaults to every gated feature flag
                  -keyfile <path>      file holding the base64 private key
                  -privkey <base64>    private key inline (avoid: visible in process args)
                The private key is also read from the `+envAuthorityPrivateKey+` env var.

  verify        Verify a token against the configured public key and print its claims.
                  -token <token>       (required) the envelope token
                  -pubkey <base64>     defaults to SCANDRIX_LICENSE_PUBLIC_KEY

Exit codes: 0 success, 1 error, 2 usage error.
`)
}

// genKeypair creates a new authority keypair. The private half is written to a
// 0600 file by default so it cannot end up in a shell history, a CI log, or a
// world-readable path; printing it requires an explicit -stdout.
func genKeypair(args []string) error {
	fs := flag.NewFlagSet("gen-keypair", flag.ContinueOnError)
	out := fs.String("out", "", "path to write the private key to (0600 permissions)")
	toStdout := fs.Bool("stdout", false, "print the private key to stdout instead of writing a file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *out == "" && !*toStdout {
		return errors.New("gen-keypair requires -out <path> (or -stdout to print the private key); refusing to guess where a signing key should live")
	}
	if *out != "" && *toStdout {
		return errors.New("gen-keypair accepts either -out or -stdout, not both")
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generating Ed25519 keypair: %w", err)
	}

	privB64 := base64.StdEncoding.EncodeToString(priv)
	pubB64 := base64.StdEncoding.EncodeToString(pub)

	if *out != "" {
		// 0600 and O_EXCL: never widen permissions, never silently clobber an
		// existing key file (overwriting one would orphan every license it signed).
		if err := os.WriteFile(*out, []byte(privB64+"\n"), 0o600); err != nil {
			return fmt.Errorf("writing private key file: %w", err)
		}
		if err := os.Chmod(*out, 0o600); err != nil {
			return fmt.Errorf("restricting private key permissions: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Private key written to %s (mode 0600). Move it into your HSM or secret manager and delete the file.\n", *out)
	}

	fmt.Printf("# Public key (not a secret) - distribute to servers as SCANDRIX_LICENSE_PUBLIC_KEY\n")
	fmt.Printf("%s\n", pubB64)

	if *toStdout {
		fmt.Printf("\n# PRIVATE KEY - printed because -stdout was given. Never commit or log this line.\n%s\n", privB64)
	}

	fmt.Fprintf(os.Stderr, "\nNext steps:\n")
	fmt.Fprintf(os.Stderr, "  1. Store the private key in your HSM/secret manager (file or env var %s).\n", envAuthorityPrivateKey)
	fmt.Fprintf(os.Stderr, "  2. Set SCANDRIX_LICENSE_PUBLIC_KEY=%s on every server.\n", pubB64)
	fmt.Fprintf(os.Stderr, "  3. For a rotation overlap, also set SCANDRIX_LICENSE_KEY_RING=<oldKeyID>:<oldBase64Key>.\n")
	fmt.Fprintf(os.Stderr, "  4. Mint with: scandrix-keygen issue -customer \"...\" -key-id <keyID> -keyfile <path>\n")
	return nil
}

// issue mints a signed license token.
func issue(args []string) error {
	fs := flag.NewFlagSet("issue", flag.ContinueOnError)
	customer := fs.String("customer", "", "customer organization name (required)")
	customerID := fs.String("customer-id", "", "customer identifier (default: generated UUID)")
	tier := fs.String("tier", string(license.TierEnterprise), "license tier")
	seats := fs.Int("seats", 100, "maximum seats, 0 = unlimited")
	repos := fs.Int("repos", 0, "maximum repositories, 0 = unlimited")
	days := fs.Int("days", 365, "validity in days")
	keyID := fs.String("key-id", "", "rotation key identifier this token is minted under")
	hardware := fs.String("hardware", "", "hardware/cluster fingerprint to bind the license to")
	features := fs.String("features", "", "comma-separated feature flags (default: all gated flags)")
	keyFile := fs.String("keyfile", "", "path to a file containing the base64 private key")
	privKeyInline := fs.String("privkey", "", "base64 private key inline (visible in process args)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*customer) == "" {
		return errors.New("issue requires -customer")
	}
	if *days <= 0 {
		return fmt.Errorf("-days must be positive, got %d", *days)
	}
	if *seats < 0 || *repos < 0 {
		return fmt.Errorf("-seats and -repos must not be negative (0 means unlimited)")
	}

	privKey, err := loadPrivateKey(*keyFile, *privKeyInline)
	if err != nil {
		return err
	}

	featureList, err := resolveFeatures(*features)
	if err != nil {
		return err
	}

	cid := strings.TrimSpace(*customerID)
	if cid == "" {
		cid = uuid.New().String()
	}

	now := time.Now().UTC()
	payload := license.LicensePayload{
		LicenseID:           uuid.New(),
		CustomerName:        strings.TrimSpace(*customer),
		CustomerID:          cid,
		Tier:                license.NormalizeTier(license.LicenseTier(*tier)),
		IssuedAt:            now,
		ExpiresAt:           now.AddDate(0, 0, *days),
		MaxSeats:            *seats,
		MaxRepositories:     *repos,
		Features:            featureList,
		KeyID:               strings.TrimSpace(*keyID),
		HardwareFingerprint: strings.TrimSpace(*hardware),
	}

	// IssueLicense emits the exact envelope LoadLicense parses, so the token
	// needs no post-processing.
	token, err := license.IssueLicense(payload, privKey)
	if err != nil {
		return fmt.Errorf("issuing license: %w", err)
	}

	fmt.Println(token)

	fmt.Fprintf(os.Stderr, "\nIssued for %q (tier %s, %d seats, %d repos, expires %s).\n",
		payload.CustomerName, payload.Tier, payload.MaxSeats, payload.MaxRepositories,
		payload.ExpiresAt.Format(time.RFC3339))
	if payload.KeyID != "" {
		fmt.Fprintf(os.Stderr, "Signed under key ID %q - every server must have that key in SCANDRIX_LICENSE_KEY_RING or as its primary key.\n", payload.KeyID)
	}
	if payload.HardwareFingerprint != "" {
		fmt.Fprintf(os.Stderr, "Bound to a hardware fingerprint: servers must set SCANDRIX_HARDWARE_FINGERPRINT to the same value.\n")
	}
	fmt.Fprintf(os.Stderr, "This token is an entitlement credential. Deliver it to the customer over a secret channel; do not log or commit it.\n")
	return nil
}

// verify checks a token against the configured public key and prints the
// entitlement it carries. It performs no writes and needs no private key.
func verify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	token := fs.String("token", "", "license token to verify (required)")
	pubKeyInline := fs.String("pubkey", "", "base64 public key (defaults to SCANDRIX_LICENSE_PUBLIC_KEY)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	tokenStr := strings.TrimSpace(*token)
	if tokenStr == "" {
		return errors.New("verify requires -token")
	}

	encodedKey := strings.TrimSpace(*pubKeyInline)
	if encodedKey == "" {
		encodedKey = strings.TrimSpace(os.Getenv(license.EnvPublicKey))
	}
	if encodedKey == "" {
		return fmt.Errorf("verify requires -pubkey or %s to be set", license.EnvPublicKey)
	}

	pubKey, err := license.PublicKeyFromBase64(encodedKey)
	if err != nil {
		return err
	}

	// Build a manager that mirrors a real server so verification exercises the
	// production path: same envelope decode, same key selection by KeyID, same
	// grace and fingerprint rules. The public key is registered as the primary
	// key and, when the token names one, also under that KeyID - which is
	// exactly the state a server is in once an operator has distributed the
	// matching public key for that key ID.
	//
	// The fingerprint binding is applied when SCANDRIX_HARDWARE_FINGERPRINT is
	// set, so `verify` is a genuine pre-flight check: a token that passes here
	// against the same environment a server will run in will load there too.
	manager := license.NewLicenseManager(pubKey)
	manager.SetExpectedFingerprint(os.Getenv(license.EnvHardwareFingerprint))

	preview, err := peekPayload(tokenStr)
	if err != nil {
		return err
	}
	if preview.KeyID != "" {
		if err := manager.AddVerificationKey(preview.KeyID, pubKey); err != nil {
			return err
		}
	}

	loaded, err := manager.LoadLicense(tokenStr)
	if err != nil {
		return err
	}

	ent := license.EntitlementFromLicense(loaded)
	fmt.Printf("valid            true\n")
	fmt.Printf("license_id       %s\n", loaded.LicenseID)
	fmt.Printf("customer         %s (%s)\n", loaded.CustomerName, loaded.CustomerID)
	fmt.Printf("tier             %s\n", ent.Tier)
	fmt.Printf("seats            %d (0 = unlimited)\n", loaded.MaxSeats)
	fmt.Printf("repositories     %d (0 = unlimited)\n", loaded.MaxRepositories)
	fmt.Printf("issued_at        %s\n", loaded.IssuedAt.UTC().Format(time.RFC3339))
	fmt.Printf("expires_at       %s\n", loaded.ExpiresAt.UTC().Format(time.RFC3339))
	fmt.Printf("key_id           %q\n", loaded.KeyID)
	fmt.Printf("hardware_fp      %q\n", loaded.HardwareFingerprint)
	if expected := strings.TrimSpace(os.Getenv(license.EnvHardwareFingerprint)); expected != "" {
		fmt.Printf("hardware_check   enforced against %q (%s)\n", expected, license.EnvHardwareFingerprint)
	} else {
		fmt.Printf("hardware_check   not enforced (%s is empty)\n", license.EnvHardwareFingerprint)
	}
	fmt.Printf("features         %s\n", strings.Join(ent.FeatureList(), ","))
	if time.Now().UTC().After(loaded.ExpiresAt) {
		fmt.Printf("note             license is expired and inside the %s grace window\n", license.LicenseGracePeriod)
	}
	return nil
}

// peekPayload decodes the envelope and payload without verifying anything, so
// verify can learn the KeyID before choosing a key. It performs no trust
// decision; the signature is still checked by LoadLicense afterwards.
func peekPayload(tokenStr string) (*license.LicensePayload, error) {
	envelopeBytes, err := base64.StdEncoding.DecodeString(tokenStr)
	if err != nil {
		return nil, fmt.Errorf("token is not valid base64: %w", err)
	}
	var envelope license.SignedLicenseToken
	if err := jsonUnmarshal(envelopeBytes, &envelope); err != nil {
		return nil, fmt.Errorf("parsing license envelope: %w", err)
	}
	payloadBytes, err := base64.StdEncoding.DecodeString(envelope.Payload)
	if err != nil {
		return nil, fmt.Errorf("decoding license payload: %w", err)
	}
	var payload license.LicensePayload
	if err := jsonUnmarshal(payloadBytes, &payload); err != nil {
		return nil, fmt.Errorf("parsing license payload: %w", err)
	}
	return &payload, nil
}

// loadPrivateKey resolves the authority private key from the first source that
// is present: an explicit file, an inline value, then the environment.
func loadPrivateKey(keyFile, inline string) (ed25519.PrivateKey, error) {
	encoded := strings.TrimSpace(inline)
	if path := strings.TrimSpace(keyFile); path != "" {
		if encoded != "" {
			return nil, errors.New("pass either -keyfile or -privkey, not both")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading private key file: %w", err)
		}
		encoded = strings.TrimSpace(string(raw))
	}
	if encoded == "" {
		encoded = strings.TrimSpace(os.Getenv(envAuthorityPrivateKey))
	}
	if encoded == "" {
		return nil, fmt.Errorf("no signing key supplied: pass -keyfile, -privkey, or set %s", envAuthorityPrivateKey)
	}

	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("signing key is not valid base64")
	}
	if len(raw) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("signing key decoded to %d bytes, expected %d for an Ed25519 private key", len(raw), ed25519.PrivateKeySize)
	}
	return ed25519.PrivateKey(raw), nil
}

// resolveFeatures turns the -features list into validated code flag values.
// The default is every gated flag, sourced from the package's canonical list so
// a newly added flag cannot be forgotten here.
func resolveFeatures(csv string) ([]string, error) {
	csv = strings.TrimSpace(csv)
	if csv == "" {
		all := license.AllFlags()
		out := make([]string, 0, len(all))
		for _, flag := range all {
			out = append(out, string(flag))
		}
		return out, nil
	}

	valid := make(map[string]struct{}, len(license.AllFlags()))
	for _, flag := range license.AllFlags() {
		valid[string(flag)] = struct{}{}
	}

	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ok := valid[part]; !ok {
			return nil, fmt.Errorf("unknown feature flag %q; known flags: %s", part, strings.Join(knownFlags(), ", "))
		}
		if _, dup := seen[part]; dup {
			continue
		}
		seen[part] = struct{}{}
		out = append(out, part)
	}
	if len(out) == 0 {
		return nil, errors.New("-features was provided but contained no usable flag")
	}
	return out, nil
}

func knownFlags() []string {
	all := license.AllFlags()
	out := make([]string, 0, len(all))
	for _, flag := range all {
		out = append(out, string(flag))
	}
	return out
}

// jsonUnmarshal is a thin indirection so the envelope peek keeps its imports
// obvious and stays trivially testable.
func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
