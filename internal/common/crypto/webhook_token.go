// Package crypto provides webhook token creation and validation for VCS provider verification.
package crypto

// GenerateWebhookToken creates an encrypted webhook token matching the standard CBC format.
func GenerateWebhookToken(secretHex, plainToken string) (string, error) {
	return Encrypt(plainToken, secretHex)
}

// ValidateWebhookToken decrypts and validates the webhook token against expected value.
func ValidateWebhookToken(encryptedText, secretHex, expectedToken string) bool {
	if encryptedText == "" || secretHex == "" || expectedToken == "" {
		return false
	}
	decrypted, err := Decrypt(encryptedText, secretHex)
	if err != nil {
		return false
	}
	return decrypted == expectedToken
}
