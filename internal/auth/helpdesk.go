package auth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

var (
	ErrHelpdeskKeyMissing   = errors.New("helpdesk RSA private key is not configured")
	ErrHelpdeskTokenExpired = errors.New("helpdesk SSO token expired")
	ErrHelpdeskInvalidToken = errors.New("invalid helpdesk SSO token")
)

// HelpdeskClaims contains identity claims delivered to the integrated support desk.
type HelpdeskClaims struct {
	Sub       string `json:"sub"`
	Email     string `json:"email"`
	Name      string `json:"name,omitempty"`
	Role      string `json:"role,omitempty"`
	Issuer    string `json:"iss"`
	Audience  string `json:"aud"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

// HelpdeskTokenService mints short-lived asymmetric RS256 JWTs for SSO into support helpdesks.
type HelpdeskTokenService struct {
	privateKey *rsa.PrivateKey
	issuer     string
	audience   string
}

// NewHelpdeskTokenService parses a PEM-encoded RSA private key (PKCS1 or PKCS8).
func NewHelpdeskTokenService(privateKeyPEM string) (*HelpdeskTokenService, error) {
	cleanPEM := strings.TrimSpace(privateKeyPEM)
	if cleanPEM == "" {
		return &HelpdeskTokenService{
			issuer:   "scandrix",
			audience: "scandrix-helpdesk",
		}, nil
	}

	// Normalize escaped newlines and outer quotes from environment variables
	cleanPEM = strings.Trim(cleanPEM, `"'`)
	cleanPEM = strings.ReplaceAll(cleanPEM, `\n`, "\n")

	block, _ := pem.Decode([]byte(cleanPEM))
	if block == nil {
		return nil, errors.New("failed decoding PEM block from helpdesk private key")
	}

	// Try PKCS1
	if priv, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return &HelpdeskTokenService{
			privateKey: priv,
			issuer:     "scandrix",
			audience:   "scandrix-helpdesk",
		}, nil
	}

	// Try PKCS8
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed parsing PKCS8/PKCS1 private key: %w", err)
	}

	rsaPriv, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("expected RSA private key for RS256 helpdesk token generation")
	}

	return &HelpdeskTokenService{
		privateKey: rsaPriv,
		issuer:     "scandrix",
		audience:   "scandrix-helpdesk",
	}, nil
}

// NewHelpdeskTokenServiceWithKey initializes HelpdeskTokenService with an existing rsa.PrivateKey instance.
func NewHelpdeskTokenServiceWithKey(privateKey *rsa.PrivateKey, issuer, audience string) *HelpdeskTokenService {
	if issuer == "" {
		issuer = "scandrix"
	}
	if audience == "" {
		audience = "scandrix-helpdesk"
	}
	return &HelpdeskTokenService{
		privateKey: privateKey,
		issuer:     issuer,
		audience:   audience,
	}
}

// GenerateHelpdeskToken creates a 5-minute asymmetric RS256 JWT delegation credential for the authenticated user.
func (s *HelpdeskTokenService) GenerateHelpdeskToken(user *models.AccountProfile) (string, error) {
	if s.privateKey == nil {
		return "", ErrHelpdeskKeyMissing
	}
	if user == nil || user.ID == uuid.Nil {
		return "", errors.New("authenticated user profile required for helpdesk SSO")
	}

	now := time.Now().UTC()
	header := map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	}

	claims := HelpdeskClaims{
		Sub:       user.ID.String(),
		Email:     user.Email,
		Name:      user.DisplayName,
		Role:      string(user.Role),
		Issuer:    s.issuer,
		Audience:  s.audience,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(5 * time.Minute).Unix(), // 5-minute security lifetime
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signedContent := headerB64 + "." + claimsB64

	hashed := sha256.Sum256([]byte(signedContent))
	sigBytes, err := rsa.SignPKCS1v15(rand.Reader, s.privateKey, crypto.SHA256, hashed[:])
	if err != nil {
		return "", fmt.Errorf("failed signing helpdesk token: %w", err)
	}

	sigB64 := base64.RawURLEncoding.EncodeToString(sigBytes)
	return signedContent + "." + sigB64, nil
}

// VerifyHelpdeskToken cryptographically verifies the RS256 token and returns the claims.
func (s *HelpdeskTokenService) VerifyHelpdeskToken(rawToken string) (*HelpdeskClaims, error) {
	if s.privateKey == nil {
		return nil, ErrHelpdeskKeyMissing
	}

	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return nil, ErrHelpdeskInvalidToken
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrHelpdeskInvalidToken
	}

	var header map[string]string
	if err := json.Unmarshal(headerBytes, &header); err != nil || strings.ToUpper(header["alg"]) != "RS256" {
		return nil, errors.New("unsupported or invalid algorithm in helpdesk token header")
	}

	signedContent := parts[0] + "." + parts[1]
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrHelpdeskInvalidToken
	}

	hashed := sha256.Sum256([]byte(signedContent))
	if err := rsa.VerifyPKCS1v15(&s.privateKey.PublicKey, crypto.SHA256, hashed[:], sigBytes); err != nil {
		return nil, fmt.Errorf("cryptographic verification failed: %w", err)
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrHelpdeskInvalidToken
	}

	var claims HelpdeskClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, ErrHelpdeskInvalidToken
	}

	nowUnix := time.Now().UTC().Unix()
	if nowUnix > claims.ExpiresAt {
		return nil, ErrHelpdeskTokenExpired
	}

	return &claims, nil
}
