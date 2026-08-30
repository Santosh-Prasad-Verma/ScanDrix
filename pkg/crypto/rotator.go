package crypto

import (
	"fmt"
)

// ReEncryptSecret decrypts ciphertext with oldKey and re-encrypts it under newKey using fresh random nonces.
func ReEncryptSecret(oldKey, newKey, ciphertext, nonce []byte) (newCiphertext, newNonce []byte, err error) {
	plaintext, err := DecryptAESGCM(oldKey, ciphertext, nonce)
	if err != nil {
		return nil, nil, fmt.Errorf("failed decrypting with previous key: %w", err)
	}

	newCiphertext, newNonce, err = EncryptAESGCM(newKey, plaintext)
	if err != nil {
		return nil, nil, fmt.Errorf("failed encrypting with new key: %w", err)
	}

	return newCiphertext, newNonce, nil
}

// EncryptedRecord represents a database row containing an AES-256-GCM encrypted payload.
type EncryptedRecord struct {
	ID         string
	Ciphertext []byte
	Nonce      []byte
}

// BatchReEncrypt processes a slice of encrypted records, decrypting each under oldKey and re-encrypting with newKey.
func BatchReEncrypt(oldKey, newKey []byte, records []EncryptedRecord) ([]EncryptedRecord, error) {
	results := make([]EncryptedRecord, 0, len(records))

	for _, rec := range records {
		newCipher, newNonce, err := ReEncryptSecret(oldKey, newKey, rec.Ciphertext, rec.Nonce)
		if err != nil {
			return nil, fmt.Errorf("record %s re-encryption failed: %w", rec.ID, err)
		}
		results = append(results, EncryptedRecord{
			ID:         rec.ID,
			Ciphertext: newCipher,
			Nonce:      newNonce,
		})
	}

	return results, nil
}
