// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package mcp_manager_test

import (
	"crypto/rand"
	"encoding/hex"
)

// generateRandomTestKey generates an ephemeral, cryptographically secure 32-byte key for in-memory testing.
func generateRandomTestKey() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
