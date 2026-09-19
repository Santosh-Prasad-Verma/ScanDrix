package infrastructure

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
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

// JwtTokenService implements domain.TokenService using HMAC-SHA256 and RSA-256 JWTs.
type JwtTokenService struct {
	config domain.JWTConfig
}

func NewJwtTokenService(config domain.JWTConfig) *JwtTokenService {
	if config.ExpiresIn == 0 {
		config.ExpiresIn = 15 * time.Minute
	}
	if config.RefreshExpiresIn == 0 {
		config.RefreshExpiresIn = 30 * 24 * time.Hour
	}
	if config.Secret == "" {
		config.Secret = "scandrix-default-jwt-secret-do-not-use-in-production"
	}
	if config.RefreshSecret == "" {
		config.RefreshSecret = "scandrix-default-jwt-refresh-secret-do-not-use"
	}
	return &JwtTokenService{config: config}
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
	claims := EmailVerificationClaims{
		Email: email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userUUID.String(),
			Issuer:    "scandrix",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.Secret))
}

func (s *JwtTokenService) VerifyEmailToken(token string) (uuid.UUID, string, error) {
	parsed, err := jwt.ParseWithClaims(token, &EmailVerificationClaims{}, func(t *jwt.Token) (any, error) {
		return []byte(s.config.Secret), nil
	})
	if err != nil || !parsed.Valid {
		return uuid.Nil, "", errors.New("invalid email verification token")
	}

	claims, ok := parsed.Claims.(*EmailVerificationClaims)
	if !ok || claims.Subject == "" {
		return uuid.Nil, "", errors.New("invalid claims payload")
	}

	userUUID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, "", errors.New("invalid user ID in token")
	}

	return userUUID, claims.Email, nil
}

func (s *JwtTokenService) CreateForgotPassToken(userUUID uuid.UUID, email string) (string, error) {
	now := time.Now().UTC()
	claims := EmailVerificationClaims{
		Email: email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userUUID.String(),
			Issuer:    "scandrix",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.Secret))
}

func (s *JwtTokenService) VerifyForgotPassToken(token string) (uuid.UUID, string, error) {
	return s.VerifyEmailToken(token)
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
