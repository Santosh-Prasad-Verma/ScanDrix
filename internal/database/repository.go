package database

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/scandrix/backend/internal/security/kms"
	"github.com/scandrix/backend/pkg/crypto"
)

// tokenEncryptionKey is derived from the INTEGRATION_ENCRYPTION_KEY env var
// (or KMS_MASTER_KEY as fallback). It must NEVER be hardcoded in source.
// Generate with: openssl rand -hex 32
var (
	tokenEncryptionKey [32]byte
	kmsEnvelopeEngine  *kms.KMSEnvelopeEngine
)

func init() {
	_ = godotenv.Load()
	_ = godotenv.Load(".env")
	_ = godotenv.Load("ScanDrix/.env")
	_ = godotenv.Load("../ScanDrix/.env")
	_ = godotenv.Load("../.env")
	_ = godotenv.Load("../../.env")
	_ = godotenv.Load("../../../.env")
	if home, err := os.UserHomeDir(); err == nil {
		_ = godotenv.Load(filepath.Join(home, ".config", "scandrix", ".env"))
		_ = godotenv.Load(filepath.Join(home, ".scandrix", ".env"))
	}
	raw := os.Getenv("INTEGRATION_ENCRYPTION_KEY")
	if raw == "" {
		raw = os.Getenv("KMS_MASTER_KEY")
	}
	if raw != "" {
		tokenEncryptionKey = sha256.Sum256([]byte(raw))
		if staticKMS, err := kms.NewStaticKeyKMS(tokenEncryptionKey[:], "scandrix-db-master-key"); err == nil {
			kmsEnvelopeEngine = kms.NewKMSEnvelopeEngine(staticKMS)
		}
	}
}

// Repository provides clean-room, parameterized database operations adhering to Master Rule 5.3.
type Repository struct {
	client *Client
}

// NewRepository initializes a new data access layer.
func NewRepository(client *Client) *Repository {
	return &Repository{client: client}
}

// Ping checks the underlying database connection pool health.
func (r *Repository) Ping(ctx context.Context) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("repository database client is uninitialized")
	}
	return r.client.Ping(ctx)
}

// Client returns the underlying database client connection.
func (r *Repository) Client() *Client {
	if r == nil {
		return nil
	}
	return r.client
}



func decryptStoredSecret(ctx context.Context, wsID uuid.UUID, cipherText string) string {
	if cipherText == "" {
		return ""
	}
	// 1. Check for 2-tier KMS Envelope JSON (NIST SP 800-57 envelope encryption)
	if strings.HasPrefix(strings.TrimSpace(cipherText), "{") && kmsEnvelopeEngine != nil {
		if dec, err := kmsEnvelopeEngine.DecryptString(ctx, cipherText); err == nil && dec != "" {
			return dec
		}
	}
	// 2. Fall back to tenant key AES-GCM
	tenantKey := deriveTenantIntegrationKey(wsID)
	if dec, err := crypto.DecryptStringAESGCM(tenantKey, cipherText); err == nil && dec != "" {
		return dec
	}
	// 3. Fall back to master tokenEncryptionKey AES-GCM
	if dec, err := crypto.DecryptStringAESGCM(tokenEncryptionKey[:], cipherText); err == nil && dec != "" {
		return dec
	}
	// 4. Fall back to legacy tenant key
	legacyKey := deriveLegacyTenantIntegrationKey(wsID)
	if dec, err := crypto.DecryptStringAESGCM(legacyKey, cipherText); err == nil && dec != "" {
		return dec
	}
	return cipherText
}

func deriveTenantIntegrationKey(wsID uuid.UUID) []byte {
	masterKey := tokenEncryptionKey[:]
	if raw := os.Getenv("SCANDRIX_MASTER_ENCRYPTION_KEY"); raw != "" {
		k := sha256.Sum256([]byte(raw))
		masterKey = k[:]
	} else if raw := os.Getenv("DATABASE_ENCRYPTION_KEY"); raw != "" {
		k := sha256.Sum256([]byte(raw))
		masterKey = k[:]
	}
	mac := hmac.New(sha256.New, masterKey)
	mac.Write([]byte("scandrix_integration_kdf_v2:" + wsID.String()))
	return mac.Sum(nil)
}

func deriveLegacyTenantIntegrationKey(wsID uuid.UUID) []byte {
	k := sha256.Sum256([]byte("scandrix_integration_kdf_v1:" + wsID.String()))
	return k[:]
}
