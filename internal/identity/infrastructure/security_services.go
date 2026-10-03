package infrastructure

import (
	"crypto/rsa"
	"crypto/subtle"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/identity/domain"
	"golang.org/x/crypto/bcrypt"
)

// BcryptPasswordService implements domain.PasswordService using standard bcrypt hashing.
type BcryptPasswordService struct {
	cost int
}

func NewBcryptPasswordService(cost int) *BcryptPasswordService {
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		cost = bcrypt.DefaultCost
	}
	return &BcryptPasswordService{cost: cost}
}

func (s *BcryptPasswordService) HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), s.cost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (s *BcryptPasswordService) MatchPassword(enteredPassword, hashedPassword string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(enteredPassword))
	return err == nil
}

// UserClaims defines custom claims embedded in access tokens.
type UserClaims struct {
	Email          string  `json:"email"`
	Role           string  `json:"role"`
	TeamRole       *string `json:"teamRole,omitempty"`
	Status         string  `json:"status"`
	OrganizationID string  `json:"organizationId"`
	jwt.RegisteredClaims
}

// EmailVerificationClaims defines claims for email verification tokens.
type EmailVerificationClaims struct {
	Email string `json:"email"`
	jwt.RegisteredClaims
}

// Token purposes (AUDIT_REMEDIATION.md F-22).
//
// Email-verification and password-reset tokens used to be byte-identical: same
// claim struct, same 24h TTL, same issuer, and VerifyForgotPassToken simply
// delegated to VerifyEmailToken. A token delivered in a "reset your password"
// email therefore verified an email address, and vice versa -- a reset link was
// usable as an account-takeover confirmation and an address-verification token
// could be redeemed at the reset endpoint.
//
// Each purpose is bound into both the audience and a dedicated claim, and each
// verifier requires its own. Values are compared with a constant-time helper
// because the check is part of deciding whether a token may be redeemed.
const (
	tokenPurposeEmailVerification = "email_verification"
	tokenPurposePasswordReset     = "password_reset"

	audienceEmailVerification = "scandrix-email-verification"
	audiencePasswordReset     = "scandrix-password-reset"
)

// purposeClaims extends EmailVerificationClaims with an explicit purpose so the
// audience alone is not the only thing binding a token to a single use case.
type purposeClaims struct {
	Email   string `json:"email"`
	Purpose string `json:"purpose"`
	jwt.RegisteredClaims
}

// JwtTokenService implements domain.TokenService using HMAC-SHA256 and RSA-256 JWTs.
type JwtTokenService struct {
	config domain.JWTConfig
}

// NewJwtTokenService builds the token service.
//
// SECURITY: this previously fell back to a hardcoded signing secret
// ("scandrix-default-jwt-secret-do-not-use-in-production") when config.Secret
// was empty. That string is in the git history, so anyone who can read the
// repository can mint a token with Role "owner" for any organization
// (AUDIT_REMEDIATION.md F-14). There is no safe default: a missing secret is a
// configuration error and is now returned as one.
func NewJwtTokenService(config domain.JWTConfig) (*JwtTokenService, error) {
	if strings.TrimSpace(config.Secret) == "" {
		return nil, errors.New("JWT secret is required and cannot be empty; set JWT_SECRET (Master Rule 1.1)")
	}
	if strings.TrimSpace(config.RefreshSecret) == "" {
		return nil, errors.New("JWT refresh secret is required and cannot be empty; set JWT_REFRESH_SECRET")
	}
	if config.ExpiresIn == 0 {
		config.ExpiresIn = 15 * time.Minute
	}
	if config.RefreshExpiresIn == 0 {
		config.RefreshExpiresIn = 30 * 24 * time.Hour
	}
	return &JwtTokenService{config: config}, nil
}

func (s *JwtTokenService) CreateTokens(user domain.User, teamRole *domain.TeamMemberRole) (*domain.TokenResponse, error) {
	now := time.Now().UTC()
	orgID := ""
	if user.OrganizationUUID != nil {
		orgID = user.OrganizationUUID.String()
	}

	var teamRoleStr *string
	if teamRole != nil {
		str := string(*teamRole)
		teamRoleStr = &str
	}

	claims := UserClaims{
		Email:          user.Email,
		Role:           string(user.Role),
		TeamRole:       teamRoleStr,
		Status:         string(user.Status),
		OrganizationID: orgID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.UUID.String(),
			Issuer:    "scandrix-orchestrator",
			Audience:  jwt.ClaimStrings{"web"},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.config.ExpiresIn)),
		},
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	accessSigned, err := accessToken.SignedString([]byte(s.config.Secret))
	if err != nil {
		return nil, fmt.Errorf("failed signing access token: %w", err)
	}

	refreshClaims := jwt.RegisteredClaims{
		ID:        uuid.New().String(),
		Subject:   user.UUID.String(),
		Issuer:    "scandrix-orchestrator",
		Audience:  jwt.ClaimStrings{"web"},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(s.config.RefreshExpiresIn)),
	}

	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshSigned, err := refreshToken.SignedString([]byte(s.config.RefreshSecret))
	if err != nil {
		return nil, fmt.Errorf("failed signing refresh token: %w", err)
	}

	return &domain.TokenResponse{
		AccessToken:  accessSigned,
		RefreshToken: refreshSigned,
	}, nil
}

func (s *JwtTokenService) VerifyRefreshToken(token string) (uuid.UUID, error) {
	parsed, err := jwt.ParseWithClaims(token, &jwt.RegisteredClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(s.config.RefreshSecret), nil
	})

	if err != nil || !parsed.Valid {
		return uuid.Nil, errors.New("invalid refresh token")
	}

	claims, ok := parsed.Claims.(*jwt.RegisteredClaims)
	if !ok || claims.Subject == "" {
		return uuid.Nil, errors.New("missing subject claim in refresh token")
	}

	userUUID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, errors.New("invalid subject UUID in token")
	}

	return userUUID, nil
}

func (s *JwtTokenService) CreateEmailToken(userUUID uuid.UUID, email string) (string, error) {
	now := time.Now().UTC()
	claims := purposeClaims{
		Email:   email,
		Purpose: tokenPurposeEmailVerification,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userUUID.String(),
			Issuer:    "scandrix",
			Audience:  jwt.ClaimStrings{audienceEmailVerification},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.Secret))
}

// verifyPurposeToken parses token and requires it to carry exactly wantPurpose and
// the matching audience. Both are checked: the audience stops a token minted for
// another flow from being replayed here, and the explicit purpose stops a token
// that merely shares an issuer from being accepted.
func (s *JwtTokenService) verifyPurposeToken(token, wantPurpose, wantAudience string) (uuid.UUID, string, error) {
	parsed, err := jwt.ParseWithClaims(token, &purposeClaims{}, func(t *jwt.Token) (any, error) {
		// Pin the algorithm. Without this, a token whose header says "none" or
		// names an asymmetric algorithm would be verified against the HMAC key.
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		return []byte(s.config.Secret), nil
	})
	if err != nil || !parsed.Valid {
		return uuid.Nil, "", fmt.Errorf("invalid %s token", wantPurpose)
	}

	claims, ok := parsed.Claims.(*purposeClaims)
	if !ok || claims.Subject == "" {
		return uuid.Nil, "", fmt.Errorf("invalid claims payload for %s token", wantPurpose)
	}

	audienceOK := false
	for _, aud := range claims.Audience {
		if subtle.ConstantTimeCompare([]byte(aud), []byte(wantAudience)) == 1 {
			audienceOK = true
			break
		}
	}
	if !audienceOK {
		return uuid.Nil, "", fmt.Errorf("token audience does not permit %s", wantPurpose)
	}
	if subtle.ConstantTimeCompare([]byte(claims.Purpose), []byte(wantPurpose)) != 1 {
		// A password-reset token arriving here (or the reverse) is the exact
		// replay F-22 describes.
		return uuid.Nil, "", fmt.Errorf("token purpose %q cannot be used as %s", claims.Purpose, wantPurpose)
	}

	userUUID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, "", errors.New("invalid user ID in token")
	}

	return userUUID, claims.Email, nil
}

func (s *JwtTokenService) VerifyEmailToken(token string) (uuid.UUID, string, error) {
	return s.verifyPurposeToken(token, tokenPurposeEmailVerification, audienceEmailVerification)
}

func (s *JwtTokenService) CreateForgotPassToken(userUUID uuid.UUID, email string) (string, error) {
	now := time.Now().UTC()
	claims := purposeClaims{
		Email:   email,
		Purpose: tokenPurposePasswordReset,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:  userUUID.String(),
			Issuer:   "scandrix",
			Audience: jwt.ClaimStrings{audiencePasswordReset},
			IssuedAt: jwt.NewNumericDate(now),
			// Shorter than the verification token's 24h: a reset link is a far
			// more powerful capability than confirming an address.
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.Secret))
}

// VerifyForgotPassToken no longer delegates to VerifyEmailToken: a reset token
// and a verification token are different capabilities and must not be
// interchangeable (AUDIT_REMEDIATION.md F-22).
func (s *JwtTokenService) VerifyForgotPassToken(token string) (uuid.UUID, string, error) {
	return s.verifyPurposeToken(token, tokenPurposePasswordReset, audiencePasswordReset)
}

func (s *JwtTokenService) CreateHelpdeskToken(userUUID uuid.UUID) (string, error) {
	now := time.Now().UTC()
	claims := jwt.RegisteredClaims{
		Subject:   userUUID.String(),
		Issuer:    "scandrix",
		Audience:  jwt.ClaimStrings{"scandrix-helpdesk"},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
	}

	if s.config.HelpdeskPrivateKey == "" {
		// Fallback to HMAC when no RSA private key configured
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		return token.SignedString([]byte(s.config.Secret))
	}

	rsaKey, err := parseRSAPrivateKeyFromPEM(s.config.HelpdeskPrivateKey)
	if err != nil {
		return "", fmt.Errorf("failed parsing helpdesk RSA private key: %w", err)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(rsaKey)
}

func parseRSAPrivateKeyFromPEM(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("failed decoding PEM block")
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err == nil {
		return key, nil
	}

	keyPKCS8, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err == nil {
		if rsaKey, ok := keyPKCS8.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
	}

	return nil, errors.New("unsupported private key format")
}
