package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

var (
	// ErrDeviceLimitReached indicates that the workspace has exhausted its allowed concurrent CLI hardware registrations.
	ErrDeviceLimitReached = errors.New("DEVICE_LIMIT_REACHED: device limit reached for workspace")
	// ErrMissingDeviceID is returned when hardware tracking is enabled but no device ID was provided.
	ErrMissingDeviceID = errors.New("missing hardware device identifier")
)

// CLIDevice tracks an authorized physical machine / developer workstation.
type CLIDevice = models.CLIDevice

// DeviceRepository defines persistence for registered CLI hardware devices.
type DeviceRepository interface {
	GetDevice(ctx context.Context, workspaceID uuid.UUID, deviceID string) (*CLIDevice, error)
	CountDevices(ctx context.Context, workspaceID uuid.UUID) (int, error)
	RegisterDevice(ctx context.Context, device *CLIDevice) error
	UpdateDeviceLastSeen(ctx context.Context, deviceID uuid.UUID, userAgent string) error
}

// InMemoryDeviceStore provides in-memory fallback or standalone storage for tests.
type InMemoryDeviceStore struct {
	mu      sync.RWMutex
	devices map[string]*CLIDevice // key: wsID:deviceID
}

// NewInMemoryDeviceStore initializes the in-memory device registry.
func NewInMemoryDeviceStore() *InMemoryDeviceStore {
	return &InMemoryDeviceStore{
		devices: make(map[string]*CLIDevice),
	}
}

func (s *InMemoryDeviceStore) makeKey(wsID uuid.UUID, devID string) string {
	return fmt.Sprintf("%s:%s", wsID.String(), devID)
}

func (s *InMemoryDeviceStore) GetDevice(ctx context.Context, workspaceID uuid.UUID, deviceID string) (*CLIDevice, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.devices[s.makeKey(workspaceID, deviceID)]
	if !ok {
		return nil, errors.New("device not found")
	}
	cp := *d
	return &cp, nil
}

func (s *InMemoryDeviceStore) CountDevices(ctx context.Context, workspaceID uuid.UUID) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, d := range s.devices {
		if d.WorkspaceID == workspaceID {
			count++
		}
	}
	return count, nil
}

func (s *InMemoryDeviceStore) RegisterDevice(ctx context.Context, device *CLIDevice) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.devices[s.makeKey(device.WorkspaceID, device.DeviceID)] = device
	return nil
}

func (s *InMemoryDeviceStore) UpdateDeviceLastSeen(ctx context.Context, deviceUUID uuid.UUID, userAgent string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.devices {
		if d.ID == deviceUUID {
			d.LastSeen = time.Now().UTC()
			if userAgent != "" {
				d.UserAgent = userAgent
			}
			return nil
		}
	}
	return errors.New("device not found")
}

// DeviceManager enforces enterprise hardware tracking and device limits.
type DeviceManager struct {
	store       DeviceRepository
	deviceLimit int // 0 = unlimited
}

// NewDeviceManager creates a hardware tracking manager.
func NewDeviceManager(store DeviceRepository, deviceLimit int) *DeviceManager {
	if store == nil {
		store = NewInMemoryDeviceStore()
	}
	return &DeviceManager{
		store:       store,
		deviceLimit: deviceLimit,
	}
}

// DeviceLimit returns the configured maximum concurrent hardware devices.
func (m *DeviceManager) DeviceLimit() int {
	return m.deviceLimit
}

// CountDevices returns the number of currently registered hardware devices for a workspace.
func (m *DeviceManager) CountDevices(ctx context.Context, workspaceID uuid.UUID) (int, error) {
	return m.store.CountDevices(ctx, workspaceID)
}

// ValidateOrRegisterDevice checks if a hardware device ID is known or within quota limits.
// Returns a persistent verification token hash or ErrDeviceLimitReached.
func (m *DeviceManager) ValidateOrRegisterDevice(ctx context.Context, workspaceID uuid.UUID, deviceID string, userAgent string) (*CLIDevice, error) {
	if deviceID == "" {
		return nil, ErrMissingDeviceID
	}

	existing, err := m.store.GetDevice(ctx, workspaceID, deviceID)
	if err == nil && existing != nil {
		_ = m.store.UpdateDeviceLastSeen(ctx, existing.ID, userAgent)
		return existing, nil
	}

	// New device -> check limit
	if m.deviceLimit > 0 {
		count, err := m.store.CountDevices(ctx, workspaceID)
		if err == nil && count >= m.deviceLimit {
			return nil, ErrDeviceLimitReached
		}
	}

	// Register new device
	hashBytes := sha256.Sum256([]byte(deviceID))
	tokenHash := hex.EncodeToString(hashBytes[:])

	now := time.Now().UTC()
	dev := &CLIDevice{
		ID:              uuid.New(),
		WorkspaceID:     workspaceID,
		DeviceID:        deviceID,
		DeviceTokenHash: tokenHash,
		UserAgent:       userAgent,
		LastSeen:        now,
		CreatedAt:       now,
	}

	if err := m.store.RegisterDevice(ctx, dev); err != nil {
		return nil, fmt.Errorf("failed registering device: %w", err)
	}

	return dev, nil
}
