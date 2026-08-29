package kms

import (
	"time"
)

// KeyProvider specifies the KMS provider type.
type KeyProvider string

const (
	ProviderLocalKey KeyProvider = "LOCAL"
	ProviderAWSKMS   KeyProvider = "AWS_KMS"
	ProviderGCPKMS   KeyProvider = "GCP_KMS"
	ProviderVault    KeyProvider = "HASHICORP_VAULT"
)

// EncryptedEnvelope holds the envelope encryption metadata and payload.
// Follows NIST SP 800-57 recommendation for 2-tier key hierarchies (KEK + DEK).
type EncryptedEnvelope struct {
	KeyID        string    `json:"key_id"`
	KeyVersion   int       `json:"key_version"`
	EncryptedDEK []byte    `json:"encrypted_dek"` // Data Encryption Key encrypted by KEK
	Ciphertext   []byte    `json:"ciphertext"`    // Payload encrypted by DEK
	Nonce        []byte    `json:"nonce"`         // 12-byte GCM Nonce
	Algorithm    string    `json:"algorithm"`     // e.g. "AES-256-GCM"
	CreatedAt    time.Time `json:"created_at"`
}

// MasterKey represents a Key Encryption Key (KEK) managed by the KMS.
type MasterKey struct {
	ID        string    `json:"id"`
	Version   int       `json:"version"`
	RawSecret []byte    `json:"-"` // Never serialized
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	RotatedAt time.Time `json:"rotated_at,omitempty"`
}
