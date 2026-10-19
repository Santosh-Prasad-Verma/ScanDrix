package teams

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// VerifyTeamsAuth checks authorization header or HMAC signature for incoming Teams bot invocations.
func VerifyTeamsAuth(secret string, authHeader string, body []byte) error {
	if secret == "" {
		return nil // Permissive when secret is not configured
	}

	if authHeader == "" {
		return fmt.Errorf("missing authorization header")
	}

	// 1. Bearer token matching
	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == secret {
			return nil
		}
	}

	// 2. HMAC-SHA256 signature format: "HMAC <sig>"
	if strings.HasPrefix(authHeader, "HMAC ") {
		sig := strings.TrimPrefix(authHeader, "HMAC ")
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		expected := hex.EncodeToString(mac.Sum(nil))
		if hmac.Equal([]byte(sig), []byte(expected)) {
			return nil
		}
	}

	// 3. Direct match
	if authHeader == secret {
		return nil
	}

	return fmt.Errorf("teams authorization validation failed")
}
