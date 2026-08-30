package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

const (
	// DefaultBcryptCost matches Kodus salt rounds (cost factor 10)
	DefaultBcryptCost = 10
)

// HashPassword computes a bcrypt hash of the plaintext password using the standard cost factor.
func HashPassword(password string) (string, error) {
	if len(password) == 0 {
		return "", fmt.Errorf("password cannot be empty")
	}
	hashBytes, err := bcrypt.GenerateFromPassword([]byte(password), DefaultBcryptCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hashBytes), nil
}

// VerifyPassword performs a constant-time comparison of the plaintext password against the bcrypt hash.
func VerifyPassword(password, hash string) bool {
	if len(password) == 0 || len(hash) == 0 {
		return false
	}
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
