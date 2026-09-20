package idgen_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/idgen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIdGenStress_PrefixedIDConcurrencyAndParsing executes 10,000 concurrent
// PrefixedID generations across 50 goroutines to prove zero collisions and 100% roundtrip fidelity.
func TestIdGenStress_PrefixedIDConcurrencyAndParsing(t *testing.T) {
	const numGoroutines = 50
	const idsPerGoroutine = 200

	prefixes := []idgen.ResourcePrefix{
		idgen.PrefixWorkspace,
		idgen.PrefixOrganization,
		idgen.PrefixRepository,
		idgen.PrefixReview,
		idgen.PrefixFinding,
		idgen.PrefixUser,
		idgen.PrefixTeam,
		idgen.PrefixJob,
		idgen.PrefixRule,
		idgen.PrefixDevice,
	}

	var mu sync.Mutex
	seenIDs := make(map[string]bool)

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(workerID int) {
			defer wg.Done()
			prefix := prefixes[workerID%len(prefixes)]

			for i := 0; i < idsPerGoroutine; i++ {
				id, err := idgen.PrefixedID(prefix)
				assert.NoError(t, err)
				assert.True(t, strings.HasPrefix(id, string(prefix)))

				// Parse back to UUID
				parsedUUID, err := idgen.ParsePrefixedID(id, prefix)
				assert.NoError(t, err)
				assert.NotEqual(t, uuid.Nil, parsedUUID)
				assert.Equal(t, 7, int(parsedUUID.Version()))

				// Verify timestamp is sane
				ts, err := idgen.ExtractUUIDTimestamp(parsedUUID)
				assert.NoError(t, err)
				assert.WithinDuration(t, time.Now().UTC(), ts, 2*time.Minute)

				mu.Lock()
				assert.False(t, seenIDs[id], "Collision detected for prefixed ID: %s", id)
				seenIDs[id] = true
				mu.Unlock()
			}
		}(g)
	}

	wg.Wait()
	assert.Equal(t, numGoroutines*idsPerGoroutine, len(seenIDs))
}

// TestIdGenStress_GeneratorCollisionResistance exercises IdGenerator methods
// concurrently to confirm uniqueness across execution, correlation, call, and session IDs.
func TestIdGenStress_GeneratorCollisionResistance(t *testing.T) {
	ig := idgen.IdGenerator{}
	const numGoroutines = 40
	const iterations = 100

	var mu sync.Mutex
	execIDs := make(map[string]bool)
	corrIDs := make(map[string]bool)
	callIDs := make(map[string]bool)
	sessIDs := make(map[string]bool)
	msgIDs := make(map[string]bool)

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				eID := ig.ExecutionID()
				cID := ig.CorrelationID()
				clID := ig.CallID()
				sID := ig.SessionID()
				mID := ig.MessageID()

				assert.True(t, ig.ValidateID(eID, "execution"))
				assert.True(t, ig.ValidateID(cID, "correlation"))
				assert.True(t, ig.ValidateID(clID, "call"))
				assert.True(t, ig.ValidateID(sID, "session"))

				mu.Lock()
				assert.False(t, execIDs[eID], "Collision in ExecutionID: %s", eID)
				execIDs[eID] = true

				assert.False(t, corrIDs[cID], "Collision in CorrelationID: %s", cID)
				corrIDs[cID] = true

				assert.False(t, callIDs[clID], "Collision in CallID: %s", clID)
				callIDs[clID] = true

				assert.False(t, sessIDs[sID], "Collision in SessionID: %s", sID)
				sessIDs[sID] = true

				assert.False(t, msgIDs[mID], "Collision in MessageID: %s", mID)
				msgIDs[mID] = true
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
}

// TestIdGenStress_SequentialMonotonicity tests strictly increasing IDs per namespace
// and namespace-level isolation.
func TestIdGenStress_SequentialMonotonicity(t *testing.T) {
	seq := idgen.NewSequentialIdGenerator()

	// 1. Single namespace monotonicity
	for i := 1; i <= 50; i++ {
		id := seq.GenerateSequential("reviews", "rev_")
		expectedPrefix := fmt.Sprintf("rev_%d_", i)
		assert.True(t, strings.HasPrefix(id, expectedPrefix), "Expected prefix %s got %s", expectedPrefix, id)
	}

	// 2. Multi-namespace isolation
	for i := 1; i <= 20; i++ {
		idA := seq.GenerateSequential("ns_a", "a_")
		idB := seq.GenerateSequential("ns_b", "b_")

		assert.True(t, strings.HasPrefix(idA, fmt.Sprintf("a_%d_", i)))
		assert.True(t, strings.HasPrefix(idB, fmt.Sprintf("b_%d_", i)))
	}

	// 3. Reset counter
	seq.ResetCounter("reviews")
	afterReset := seq.GenerateSequential("reviews", "rev_")
	assert.True(t, strings.HasPrefix(afterReset, "rev_1_"))
}

// TestIdGenStress_HighThroughputGenerator tests buffered fast generation under load.
func TestIdGenStress_HighThroughputGenerator(t *testing.T) {
	gen := idgen.NewHighThroughputIdGenerator()
	const count = 5000

	batch := gen.GenerateBatch(count)
	require.Len(t, batch, count)

	seen := make(map[string]bool, count)
	for _, id := range batch {
		assert.False(t, seen[id], "Collision in HighThroughput Batch: %s", id)
		seen[id] = true
	}
}

// TestIdGenStress_TeamCLIKeyValidation validates team CLI key entropy, prefix,
// and length invariants.
func TestIdGenStress_TeamCLIKeyValidation(t *testing.T) {
	seenKeys := make(map[string]bool)

	for i := 0; i < 1000; i++ {
		key, err := idgen.TeamCLIKey()
		require.NoError(t, err)

		assert.True(t, strings.HasPrefix(key, "scandrix_live_"))
		assert.Equal(t, 14+48, len(key)) // "scandrix_live_" (14) + 24 bytes hex (48)

		assert.False(t, seenKeys[key], "Collision in TeamCLIKey: %s", key)
		seenKeys[key] = true
	}
}

// TestIdGenStress_ParsePrefixedIDValidationErrors verifies rejection of mismatched prefixes,
// invalid hex lengths, and corrupted characters.
func TestIdGenStress_ParsePrefixedIDValidationErrors(t *testing.T) {
	// 1. Wrong prefix
	_, err := idgen.ParsePrefixedID("ws_019203948576abcdef0123456789abcd", idgen.PrefixRepository)
	assert.ErrorIs(t, err, idgen.ErrInvalidPrefixedID)

	// 2. Too short hex
	_, err = idgen.ParsePrefixedID("repo_1234", idgen.PrefixRepository)
	assert.ErrorIs(t, err, idgen.ErrInvalidPrefixedID)

	// 3. Too long hex
	_, err = idgen.ParsePrefixedID("repo_019203948576abcdef0123456789abcdef9999", idgen.PrefixRepository)
	assert.ErrorIs(t, err, idgen.ErrInvalidPrefixedID)

	// 4. Non-hex characters
	_, err = idgen.ParsePrefixedID("repo_019203948576abcdef0123456789zzzz", idgen.PrefixRepository)
	assert.Error(t, err)

	// 5. Empty string
	_, err = idgen.ParsePrefixedID("", idgen.PrefixRepository)
	assert.ErrorIs(t, err, idgen.ErrInvalidPrefixedID)
}

// TestIdGenStress_ExtractUUIDv7TimestampRejectsV4 verifies that attempting to extract
// timestamps from a non-v7 UUID (e.g. UUIDv4) strictly fails with error.
func TestIdGenStress_ExtractUUIDv7TimestampRejectsV4(t *testing.T) {
	v4 := uuid.New() // Version 4 random UUID
	_, err := idgen.ExtractUUIDTimestamp(v4)
	assert.Error(t, err)
	assert.Equal(t, "uuid is not version 7", err.Error())
}
