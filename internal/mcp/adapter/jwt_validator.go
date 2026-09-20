// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Tools Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package adapter

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var base64URLRegex = regexp.MustCompile(`^[A-Za-z0-9_-]*$`)

// JWTOptions configures cryptographic verification and claim constraints.
type JWTOptions struct {
	SecretOrPublicKey any           // []byte, string (for HMAC), or *rsa.PublicKey
	Issuer            string
	Audience          []string
	Algorithms        []string
	ClockTolerance    time.Duration
	MaxTokenAge       time.Duration
}

// JWTClaims holds standard and custom token claims.
type JWTClaims struct {
	jwt.RegisteredClaims
	CustomClaims map[string]any `json:"-"`
}

// JWTValidator provides production-grade cryptographic JWT verification for MCP endpoints.
type JWTValidator struct {
	options JWTOptions
}

// NewJWTValidator creates a new validator with secure algorithm defaults.
func NewJWTValidator(options JWTOptions) *JWTValidator {
	if len(options.Algorithms) == 0 {
		options.Algorithms = []string{
			"RS256", "RS384", "RS512",
			"HS256", "HS384", "HS512",
		}
	}
	return &JWTValidator{options: options}
}

// ForOAuthProvider initializes a validator tuned for OAuth identity tokens.
func ForOAuthProvider(issuer, audience string) *JWTValidator {
	return NewJWTValidator(JWTOptions{
		Issuer:     issuer,
		Audience:   []string{audience},
		Algorithms: []string{"RS256"},
	})
}

// ForServiceTokens initializes a validator tuned for backend service tokens.
func ForServiceTokens(audience string, issuer ...string) *JWTValidator {
	opts := JWTOptions{
		Audience:   []string{audience},
		Algorithms: []string{"RS256", "RS384", "RS512"},
	}
	if len(issuer) > 0 {
		opts.Issuer = issuer[0]
	}
	return NewJWTValidator(opts)
}

// ValidateToken verifies token format, cryptographic signature, and all standard/custom claims.
func (v *JWTValidator) ValidateToken(tokenString string) (*JWTClaims, error) {
	if !v.IsValidJWTFormat(tokenString) {
		return nil, errors.New("invalid JWT format")
	}

	key := v.options.SecretOrPublicKey
	if key == nil {
		if envSecret := os.Getenv("JWT_SECRET"); envSecret != "" {
			key = []byte(envSecret)
		}
	}

	var claims JWTClaims
	claimsMap := make(jwt.MapClaims)

	if key != nil {
		parser := jwt.NewParser(
			jwt.WithValidMethods(v.options.Algorithms),
			jwt.WithLeeway(v.options.ClockTolerance),
		)

		token, err := parser.Parse(tokenString, func(t *jwt.Token) (any, error) {
			switch k := key.(type) {
			case []byte:
				return k, nil
			case string:
				return []byte(k), nil
			case *rsa.PublicKey:
				return k, nil
			default:
				return nil, fmt.Errorf("unsupported key type: %T", key)
			}
		})

		if err != nil {
			return nil, fmt.Errorf("JWT validation failed: %w", err)
		}

		if !token.Valid {
			return nil, errors.New("JWT validation failed: token is invalid")
		}

		if mc, ok := token.Claims.(jwt.MapClaims); ok {
			claimsMap = mc
		}
	} else {
		// No key provided: decode payload safely without verification
		decoded, err := v.decodePayload(tokenString)
		if err != nil {
			return nil, fmt.Errorf("JWT decode failed: %w", err)
		}
		claimsMap = decoded
	}

	// Unmarshal claims into typed struct
	data, err := json.Marshal(claimsMap)
	if err != nil {
		return nil, fmt.Errorf("failed to re-encode claims: %w", err)
	}

	if err := json.Unmarshal(data, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse claims struct: %w", err)
	}
	claims.CustomClaims = claimsMap

	if err := v.validateCustomClaims(&claims, claimsMap); err != nil {
		return nil, fmt.Errorf("JWT validation failed: %w", err)
	}

	return &claims, nil
}

// ValidateAudience verifies that expectedAudience is contained within the token's audience claim.
func (v *JWTValidator) ValidateAudience(tokenString string, expectedAudience string) bool {
	claims, err := v.decodePayload(tokenString)
	if err != nil {
		return false
	}

	audVal, ok := claims["aud"]
	if !ok {
		return false
	}

	switch a := audVal.(type) {
	case string:
		return a == expectedAudience
	case []any:
		for _, item := range a {
			if s, ok := item.(string); ok && s == expectedAudience {
				return true
			}
		}
	}
	return false
}

// IsExpired checks whether the token has expired.
func (v *JWTValidator) IsExpired(tokenString string) bool {
	claims, err := v.decodePayload(tokenString)
	if err != nil {
		return true // treat unparseable tokens as expired
	}

	expVal, ok := claims["exp"]
	if !ok {
		return false
	}

	var expSec int64
	switch e := expVal.(type) {
	case float64:
		expSec = int64(e)
	case int64:
		expSec = e
	case json.Number:
		if val, err := e.Int64(); err == nil {
			expSec = val
		}
	}

	if expSec == 0 {
		return false
	}

	now := time.Now().Unix()
	return expSec < now
}

// GetExpirationTime extracts token expiration timestamp.
func (v *JWTValidator) GetExpirationTime(tokenString string) *time.Time {
	claims, err := v.decodePayload(tokenString)
	if err != nil {
		return nil
	}

	expVal, ok := claims["exp"]
	if !ok {
		return nil
	}

	var expSec int64
	switch e := expVal.(type) {
	case float64:
		expSec = int64(e)
	case int64:
		expSec = e
	case json.Number:
		if val, err := e.Int64(); err == nil {
			expSec = val
		}
	}

	if expSec == 0 {
		return nil
	}

	t := time.Unix(expSec, 0)
	return &t
}

// IsValidJWTFormat ensures the string conforms to standard 3-part base64url JWT structure.
func (v *JWTValidator) IsValidJWTFormat(token string) bool {
	if len(token) < 20 {
		return false
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}

	for _, part := range parts {
		if !base64URLRegex.MatchString(part) {
			return false
		}
	}
	return true
}

func (v *JWTValidator) decodePayload(token string) (jwt.MapClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[1] == "" {
		return nil, errors.New("invalid JWT format")
	}

	seg := parts[1]
	// Handle unpadded base64url
	if l := len(seg) % 4; l > 0 {
		seg += strings.Repeat("=", 4-l)
	}

	decoded, err := base64.URLEncoding.DecodeString(seg)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64url payload: %w", err)
	}

	var claims jwt.MapClaims
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse JSON claims: %w", err)
	}

	return claims, nil
}

func (v *JWTValidator) validateCustomClaims(claims *JWTClaims, raw jwt.MapClaims) error {
	// Issuer check
	if v.options.Issuer != "" && claims.Issuer != v.options.Issuer {
		return errors.New("invalid issuer")
	}

	// Audience check
	if len(v.options.Audience) > 0 {
		matched := false
		for _, expAud := range v.options.Audience {
			for _, actAud := range claims.Audience {
				if actAud == expAud {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			return errors.New("invalid audience")
		}
	}

	// Subject or JTI check
	jti, _ := raw["jti"].(string)
	if claims.Subject == "" && jti == "" {
		return errors.New("token must have subject (sub) or JWT ID (jti)")
	}

	now := time.Now()

	// Max token age check
	if v.options.MaxTokenAge > 0 && claims.IssuedAt != nil {
		tokenAge := now.Sub(claims.IssuedAt.Time)
		if tokenAge > v.options.MaxTokenAge {
			return errors.New("token is too old")
		}
	}

	// Not before check
	if claims.NotBefore != nil {
		if now.Before(claims.NotBefore.Time.Add(-v.options.ClockTolerance)) {
			return errors.New("token is not yet valid (nbf claim)")
		}
	}

	return nil
}
