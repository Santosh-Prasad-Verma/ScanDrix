package clireview

import (
	"github.com/scandrix/backend/internal/clireview/infrastructure/adapters"
)

// TeamKeyStore defines an interface or in-memory map for verifying team API keys.
type TeamKeyStore = adapters.TeamKeyStore

// TeamKeyRecord represents stored team key metadata.
type TeamKeyRecord = adapters.TeamKeyRecord

// DeviceStore handles device tracking and token minting for CLI terminals.
type DeviceStore = adapters.DeviceStore

// NewDeviceStore creates an initialized device store.
func NewDeviceStore() *DeviceStore {
	return adapters.NewDeviceStore()
}

// KeyValidator verifies CLI caller credentials (team API key or user JWT).
type KeyValidator = adapters.KeyValidator

// NewKeyValidator creates a configured KeyValidator.
func NewKeyValidator(jwtSecret string, deviceStore *DeviceStore) *KeyValidator {
	return adapters.NewKeyValidator(jwtSecret, deviceStore)
}
