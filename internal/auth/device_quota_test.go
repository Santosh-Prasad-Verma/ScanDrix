package auth_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
)

func TestDeviceQuotaManager(t *testing.T) {
	ctx := context.Background()
	wsID := uuid.New()
	limit := 3
	mgr := auth.NewDeviceManager(nil, limit)

	// 1. Missing Device ID
	_, errMissing := mgr.ValidateOrRegisterDevice(ctx, wsID, "", "ScanDrix-CLI")
	if !errors.Is(errMissing, auth.ErrMissingDeviceID) {
		t.Fatalf("expected ErrMissingDeviceID, got: %v", errMissing)
	}

	// 2. Register devices up to the limit (3 devices)
	for i := 1; i <= limit; i++ {
		devID := fmt.Sprintf("macbook-pro-%d", i)
		dev, err := mgr.ValidateOrRegisterDevice(ctx, wsID, devID, "ScanDrix-CLI/v1.0")
		if err != nil {
			t.Fatalf("failed registering device %d: %v", i, err)
		}
		if dev.DeviceID != devID || dev.WorkspaceID != wsID {
			t.Fatalf("unexpected registered device fields: %+v", dev)
		}
	}

	// 3. Known device should succeed and update lastSeen without counting as a new registration
	knownDev, errKnown := mgr.ValidateOrRegisterDevice(ctx, wsID, "macbook-pro-1", "ScanDrix-CLI/v1.1")
	if errKnown != nil {
		t.Fatalf("unexpected error re-validating known device: %v", errKnown)
	}
	if knownDev.DeviceID != "macbook-pro-1" {
		t.Fatalf("mismatched known device: %+v", knownDev)
	}

	// 4. Registering a 4th new device exceeds the quota limit (DEVICE_LIMIT_REACHED)
	_, errOverLimit := mgr.ValidateOrRegisterDevice(ctx, wsID, "macbook-pro-4", "ScanDrix-CLI/v1.0")
	if !errors.Is(errOverLimit, auth.ErrDeviceLimitReached) {
		t.Fatalf("expected ErrDeviceLimitReached, got: %v", errOverLimit)
	}

	// 5. A different workspace has its own independent quota
	otherWsID := uuid.New()
	devOther, errOther := mgr.ValidateOrRegisterDevice(ctx, otherWsID, "macbook-pro-1", "ScanDrix-CLI/v1.0")
	if errOther != nil {
		t.Fatalf("expected other workspace registration to succeed, got: %v", errOther)
	}
	if devOther.WorkspaceID != otherWsID {
		t.Fatalf("mismatched workspace ID: %+v", devOther)
	}
}
