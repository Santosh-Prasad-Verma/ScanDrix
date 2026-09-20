package idgen_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/idgen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Matrix Tests: Prefixed Resource Identifiers
// ============================================================================

func TestIdGenMatrix_AllResourcePrefixes(t *testing.T) {
	prefixes := []struct {
		prefix   idgen.ResourcePrefix
		name     string
		expected string
	}{
		{idgen.PrefixWorkspace, "Workspace", "ws_"},
		{idgen.PrefixOrganization, "Organization", "org_"},
		{idgen.PrefixRepository, "Repository", "repo_"},
		{idgen.PrefixReview, "Review", "rev_"},
		{idgen.PrefixFinding, "Finding", "find_"},
		{idgen.PrefixUser, "User", "usr_"},
		{idgen.PrefixTeam, "Team", "team_"},
		{idgen.PrefixJob, "Job", "job_"},
		{idgen.PrefixEvent, "Event", "evt_"},
		{idgen.PrefixMessage, "Message", "msg_"},
		{idgen.PrefixToken, "Token", "tok_"},
		{idgen.PrefixRule, "Rule", "rule_"},
		{idgen.PrefixDevice, "Device", "dev_"},
		{idgen.PrefixKey, "Key", "key_"},
		{idgen.PrefixSession, "Session", "sess_"},
	}

	for _, p := range prefixes {
		t.Run("Prefix_"+p.name, func(t *testing.T) {
			id, err := idgen.PrefixedID(p.prefix)
			require.NoError(t, err)
			assert.True(t, strings.HasPrefix(id, p.expected), "ID %s must start with %s", id, p.expected)
			assert.Equal(t, len(p.expected)+32, len(id), "ID %s should be prefix + 32-char hex", id)

			// Parsing back into UUID
			parsedUUID, err := idgen.ParsePrefixedID(id, p.prefix)
			require.NoError(t, err)
			assert.NotEqual(t, uuid.Nil, parsedUUID)
			assert.Equal(t, uuid.Version(7), parsedUUID.Version())

			// Extract UUID timestamp
			ts, err := idgen.ExtractUUIDTimestamp(parsedUUID)
			require.NoError(t, err)
			assert.WithinDuration(t, time.Now().UTC(), ts, 5*time.Second)
		})
	}
}

// ============================================================================
// Matrix Tests: UUIDv7 Monotonicity and Timestamp Extraction
// ============================================================================

func TestIdGenMatrix_UUIDv7Monotonicity(t *testing.T) {
	const count = 1000
	uuids := make([]string, count)

	for i := 0; i < count; i++ {
		u, err := idgen.UUIDv7()
		require.NoError(t, err)
		uuids[i] = u.String()
	}

	// UUIDv7 is lexicographically sortable by time
	for i := 1; i < count; i++ {
		assert.True(t, uuids[i] >= uuids[i-1], "UUIDv7[%d]=%s should be >= UUIDv7[%d]=%s", i, uuids[i], i-1, uuids[i-1])
	}
}

// ============================================================================
// Matrix Tests: Format Validation & Pattern Matching
// ============================================================================

func TestIdGenMatrix_FormatValidationPatterns(t *testing.T) {
	ig := idgen.IdGenerator{}

	t.Run("ExecutionID Pattern Compliance", func(t *testing.T) {
		for i := 0; i < 50; i++ {
			id := ig.ExecutionID()
			assert.True(t, ig.ValidateID(id, "execution"), "ExecutionID %s must validate", id)
			assert.False(t, ig.ValidateID(id, "correlation"))
			assert.False(t, ig.ValidateID(id, "session"))
		}
	})

	t.Run("CorrelationID Pattern Compliance", func(t *testing.T) {
		for i := 0; i < 50; i++ {
			id := ig.CorrelationID()
			assert.True(t, ig.ValidateID(id, "correlation"), "CorrelationID %s must validate", id)
			assert.False(t, ig.ValidateID(id, "execution"))
		}
	})

	t.Run("CallID Pattern Compliance", func(t *testing.T) {
		for i := 0; i < 50; i++ {
			id := ig.CallID()
			assert.True(t, ig.ValidateID(id, "call"), "CallID %s must validate", id)
		}
	})

	t.Run("SessionID Pattern Compliance", func(t *testing.T) {
		for i := 0; i < 50; i++ {
			id := ig.SessionID()
			assert.True(t, ig.ValidateID(id, "session"), "SessionID %s must validate", id)
		}
	})

	t.Run("TenantID Pattern Compliance", func(t *testing.T) {
		for i := 0; i < 50; i++ {
			id := ig.TenantID()
			assert.True(t, ig.ValidateID(id, "tenant"), "TenantID %s must validate", id)
		}
	})
}

// ============================================================================
// Matrix Tests: High-Concurrency Multithreaded Collision Stress
// ============================================================================

func TestIdGenMatrix_HighConcurrencyCollisionStress(t *testing.T) {
	ig := idgen.IdGenerator{}

	const numGoroutines = 50
	const idsPerRoutine = 500
	const totalExpectedIDs = numGoroutines * idsPerRoutine

	var mu sync.Mutex
	seenIDs := make(map[string]struct{}, totalExpectedIDs)

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	startTime := time.Now()

	for g := 0; g < numGoroutines; g++ {
		go func(routineID int) {
			defer wg.Done()
			localIDs := make([]string, idsPerRoutine)
			for i := 0; i < idsPerRoutine; i++ {
				switch (routineID*idsPerRoutine + i) % 4 {
				case 0:
					localIDs[i] = ig.ExecutionID()
				case 1:
					localIDs[i] = ig.CorrelationID()
				case 2:
					localIDs[i] = idgen.MustPrefixedID(idgen.PrefixReview)
				case 3:
					u, _ := idgen.UUIDv7()
					localIDs[i] = u.String()
				}
			}

			mu.Lock()
			for _, id := range localIDs {
				seenIDs[id] = struct{}{}
			}
			mu.Unlock()
		}(g)
	}

	wg.Wait()

	assert.Equal(t, totalExpectedIDs, len(seenIDs), "Zero collision tolerance across %d IDs generated in %v", totalExpectedIDs, time.Since(startTime))
}

// ============================================================================
// Matrix Tests: Timestamp Extraction Validity
// ============================================================================

func TestIdGenMatrix_ExtractTimestampAccuracy(t *testing.T) {
	ig := idgen.IdGenerator{}

	t.Run("ExecutionID Timestamp", func(t *testing.T) {
		beforeMs := time.Now().Add(-100 * time.Millisecond).UnixMilli()
		id := ig.ExecutionID()
		afterMs := time.Now().Add(100 * time.Millisecond).UnixMilli()

		extractedMs, err := ig.ExtractTimestamp(id)
		require.NoError(t, err)
		assert.True(t, extractedMs >= beforeMs && extractedMs <= afterMs, "Extracted ms %d should be between %d and %d", extractedMs, beforeMs, afterMs)
	})

	t.Run("Invalid Format Extraction Returns Error", func(t *testing.T) {
		_, err := ig.ExtractTimestamp("no-underscores")
		assert.Error(t, err)

		_, err2 := ig.ExtractTimestamp("one_underscore")
		assert.Error(t, err2)

		_, err3 := ig.ExtractTimestamp("bad_$$$$$_$$$$$")
		assert.Error(t, err3)
	})
}

// ============================================================================
// Matrix Tests: ScanDrix Team CLI Key & Secure Token Security
// ============================================================================

func TestIdGenMatrix_TeamCLIKeyAndSecureToken(t *testing.T) {
	t.Run("TeamCLIKey Has Proper Prefix and Entropy", func(t *testing.T) {
		key, err := idgen.TeamCLIKey()
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(key, "scandrix_live_"), "Team CLI key %s must start with 'scandrix_live_'", key)
		assert.Equal(t, len("scandrix_live_")+48, len(key)) // 24 bytes hex = 48 chars
	})

	t.Run("SecureToken Length Customization", func(t *testing.T) {
		lengths := []int{16, 32, 64, 128}
		for _, l := range lengths {
			tok, err := idgen.SecureToken(l)
			require.NoError(t, err)
			assert.Equal(t, l*2, len(tok)) // hex is 2x byte length
		}
	})
}
