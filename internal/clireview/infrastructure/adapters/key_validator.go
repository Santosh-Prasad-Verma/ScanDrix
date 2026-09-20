package adapters

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/clireview/domain"
)

// TeamKeyStore defines an interface or in-memory map for verifying team API keys.
type TeamKeyStore interface {
	ValidateTeamKey(key string) (*TeamKeyRecord, error)
}

// TeamKeyRecord represents stored team key metadata.
type TeamKeyRecord struct {
	KeyID          string
	TeamID         string
	TeamName       string
	OrganizationID string
	OrgName        string
	Active         bool
}

// DeviceStore handles device tracking and token minting for CLI terminals.
type DeviceStore struct {
	mu      sync.RWMutex
	devices map[string]string // deviceId -> deviceToken
}

// NewDeviceStore creates an initialized device store.
func NewDeviceStore() *DeviceStore {
	return &DeviceStore{
		devices: make(map[string]string),
	}
}

// ValidateOrRegisterDevice checks or creates a device token.
func (ds *DeviceStore) ValidateOrRegisterDevice(deviceID, existingToken, organizationID, userAgent string) (string, error) {
	if strings.TrimSpace(deviceID) == "" {
		return "", errors.New("device_id cannot be empty")
	}

	ds.mu.Lock()
	defer ds.mu.Unlock()

	if existingToken != "" {
		if stored, ok := ds.devices[deviceID]; ok && stored == existingToken {
			return existingToken, nil
		}
	}

	// Generate deterministic new token
	h := sha256.New()
	h.Write([]byte(deviceID + ":" + organizationID + ":" + userAgent + ":" + time.Now().String()))
	newToken := "scandrix_dev_" + hex.EncodeToString(h.Sum(nil))[:32]
	ds.devices[deviceID] = newToken
	return newToken, nil
}

// KeyValidator verifies CLI caller credentials (team API key or user JWT).
type KeyValidator struct {
	jwtSecret   []byte
	teamKeys    map[string]TeamKeyRecord
	deviceStore *DeviceStore
	mu          sync.RWMutex
}

// NewKeyValidator creates a configured KeyValidator.
func NewKeyValidator(jwtSecret string, deviceStore *DeviceStore) *KeyValidator {
	if deviceStore == nil {
		deviceStore = NewDeviceStore()
	}
	return &KeyValidator{
		jwtSecret:   []byte(jwtSecret),
		teamKeys:    make(map[string]TeamKeyRecord),
		deviceStore: deviceStore,
	}
}

// RegisterTeamKey associates a team key with an organization and team.
func (kv *KeyValidator) RegisterTeamKey(key string, record TeamKeyRecord) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	kv.teamKeys[key] = record
}

// ValidateCliKey executes authentication validation against a team key or JWT token.
func (kv *KeyValidator) ValidateCliKey(input domain.ValidateCliKeyInput) domain.ValidateCliKeyResult {
	payload := kv.validateAuth(input)

	shouldTrackDevice := input.DeviceID != "" && payload.Valid && payload.OrganizationID != nil
	if !shouldTrackDevice {
		return payload
	}

	deviceToken, err := kv.deviceStore.ValidateOrRegisterDevice(
		input.DeviceID,
		input.DeviceToken,
		*payload.OrganizationID,
		input.UserAgent,
	)
	if err != nil {
		payload.Valid = false
		payload.Error = err.Error()
		payload.Code = "device_rejected"
		return payload
	}

	payload.DeviceToken = deviceToken
	if payload.Data == nil {
		payload.Data = make(map[string]any)
	}
	payload.Data["deviceToken"] = deviceToken
	return payload
}

func (kv *KeyValidator) validateAuth(input domain.ValidateCliKeyInput) domain.ValidateCliKeyResult {
	teamKey := strings.TrimSpace(input.TeamKey)
	authHeader := strings.TrimSpace(input.AuthHeader)

	var bearerToken string
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		bearerToken = strings.TrimSpace(authHeader[7:])
	}

	buildInvalid := func(errStr string) domain.ValidateCliKeyResult {
		emptyID := ""
		res := domain.ValidateCliKeyResult{
			Valid:        false,
			Error:        errStr,
			Team:         &domain.ValidateCliKeyEntity{ID: &emptyID, Name: ""},
			Organization: &domain.ValidateCliKeyEntity{ID: &emptyID, Name: ""},
			User:         &domain.ValidateCliKeyUser{Email: "", Name: ""},
			Data:         make(map[string]any),
		}
		res.Data["valid"] = false
		res.Data["error"] = errStr
		return res
	}

	// Route 1: Team CLI key (via X-Team-Key or Bearer with scandrix_ prefix)
	if teamKey != "" || strings.HasPrefix(bearerToken, "scandrix_") {
		key := teamKey
		if key == "" {
			key = bearerToken
		}

		kv.mu.RLock()
		record, ok := kv.teamKeys[key]
		kv.mu.RUnlock()

		if !ok || !record.Active {
			return buildInvalid("Invalid or revoked team API key")
		}

		res := domain.ValidateCliKeyResult{
			Valid:            true,
			TeamID:           &record.TeamID,
			OrganizationID:   &record.OrganizationID,
			TeamName:         record.TeamName,
			OrganizationName: record.OrgName,
			Team:             &domain.ValidateCliKeyEntity{ID: &record.TeamID, Name: record.TeamName},
			Organization:     &domain.ValidateCliKeyEntity{ID: &record.OrganizationID, Name: record.OrgName},
			User:             &domain.ValidateCliKeyUser{Email: "", Name: ""},
			Data:             make(map[string]any),
		}
		res.Data["valid"] = true
		res.Data["teamId"] = record.TeamID
		res.Data["organizationId"] = record.OrganizationID
		return res
	}

	// Route 2: JWT Bearer token
	if bearerToken != "" {
		token, err := jwt.Parse(bearerToken, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return kv.jwtSecret, nil
		})

		if err != nil || !token.Valid {
			return buildInvalid("Invalid or expired JWT token")
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return buildInvalid("Malformed token claims")
		}

		email, _ := claims["email"].(string)
		orgID, _ := claims["organizationId"].(string)
		teamID, _ := claims["teamId"].(string)
		teamName, _ := claims["teamName"].(string)
		orgName, _ := claims["organizationName"].(string)

		if teamID == "" {
			if input.QueryTeamID != "" {
				teamID = input.QueryTeamID
			} else {
				teamID = uuid.New().String()
			}
		}

		res := domain.ValidateCliKeyResult{
			Valid:            true,
			TeamID:           &teamID,
			OrganizationID:   &orgID,
			TeamName:         teamName,
			OrganizationName: orgName,
			Team:             &domain.ValidateCliKeyEntity{ID: &teamID, Name: teamName},
			Organization:     &domain.ValidateCliKeyEntity{ID: &orgID, Name: orgName},
			User:             &domain.ValidateCliKeyUser{Email: email, Name: ""},
			Email:            email,
			UserEmail:        email,
			Data:             make(map[string]any),
		}
		res.Data["valid"] = true
		res.Data["email"] = email
		res.Data["teamId"] = teamID
		res.Data["organizationId"] = orgID
		return res
	}

	return buildInvalid("Authentication required. Provide a team API key via X-Team-Key header, or a JWT via Authorization: Bearer header.")
}
