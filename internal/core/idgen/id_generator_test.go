package idgen_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/idgen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUUIDv7MonotonicityAndTimestamp(t *testing.T) {
	before := time.Now().UTC().Truncate(time.Millisecond)
	u, err := idgen.UUIDv7()
	require.NoError(t, err)
	after := time.Now().UTC().Add(time.Millisecond)

	assert.Equal(t, uuid.Version(7), u.Version())

	ts, err := idgen.ExtractUUIDTimestamp(u)
	require.NoError(t, err)
	assert.False(t, ts.Before(before), "extracted ts should not be before generation time")
	assert.False(t, ts.After(after), "extracted ts should not be after generation time")
}

func TestPrefixedIDGenerationAndParsing(t *testing.T) {
	prefixes := []idgen.ResourcePrefix{
		idgen.PrefixWorkspace,
		idgen.PrefixOrganization,
		idgen.PrefixRepository,
		idgen.PrefixReview,
		idgen.PrefixFinding,
		idgen.PrefixUser,
		idgen.PrefixJob,
		idgen.PrefixEvent,
		idgen.PrefixMessage,
		idgen.PrefixToken,
		idgen.PrefixRule,
		idgen.PrefixDevice,
		idgen.PrefixKey,
	}

	for _, p := range prefixes {
		t.Run(string(p), func(t *testing.T) {
			id, err := idgen.PrefixedID(p)
			require.NoError(t, err)
			assert.True(t, strings.HasPrefix(id, string(p)))

			parsedUUID, err := idgen.ParsePrefixedID(id, p)
			require.NoError(t, err)
			assert.Equal(t, uuid.Version(7), parsedUUID.Version())

			// Negative check: wrong prefix
			_, err = idgen.ParsePrefixedID(id, "wrong_")
			assert.ErrorIs(t, err, idgen.ErrInvalidPrefixedID)
		})
	}
}

func TestTeamCLIKeyFormat(t *testing.T) {
	liveKey, err := idgen.TeamCLIKey()
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(liveKey, "scandrix_live_"))
	assert.True(t, len(liveKey) > 20)
}

func TestSecureToken(t *testing.T) {
	tokenA, err := idgen.SecureToken(32)
	require.NoError(t, err)
	tokenB, err := idgen.SecureToken(32)
	require.NoError(t, err)

	assert.Len(t, tokenA, 64) // 32 bytes = 64 hex characters
	assert.Len(t, tokenB, 64)
	assert.NotEqual(t, tokenA, tokenB)
}

func TestScanDrixIdGeneratorMethods(t *testing.T) {
	var ig idgen.IdGenerator

	// Execution ID
	execID := ig.ExecutionID()
	t.Log("execID:", execID)
	assert.True(t, strings.HasPrefix(execID, "exec_"))
	assert.True(t, ig.ValidateID(execID, "execution"))

	// Correlation ID
	corrID := ig.CorrelationID()
	assert.True(t, strings.HasPrefix(corrID, "corr_"))
	assert.True(t, ig.ValidateID(corrID, "correlation"))

	// Call ID
	callID := ig.CallID()
	assert.True(t, strings.HasPrefix(callID, "call_"))
	assert.True(t, ig.ValidateID(callID, "call"))

	// Session ID
	sessID := ig.SessionID()
	assert.True(t, strings.HasPrefix(sessID, "sess_"))
	assert.True(t, ig.ValidateID(sessID, "session"))

	// Tenant ID
	tenantID := ig.TenantID()
	assert.True(t, strings.HasPrefix(tenantID, "tenant_"))
	assert.True(t, ig.ValidateID(tenantID, "tenant"))

	// Trace, Span, Event, Message IDs
	assert.True(t, strings.HasPrefix(ig.GenerateTraceID(), "trace_"))
	assert.True(t, strings.HasPrefix(ig.GenerateSpanID(), "span_"))
	assert.True(t, strings.HasPrefix(ig.EventID(), "evt_"))
	assert.True(t, strings.HasPrefix(ig.TraceID(), "trace_"))
	assert.True(t, strings.HasPrefix(ig.SpanID(), "span_"))
	assert.True(t, strings.HasPrefix(ig.MessageID(), "msg_"))

	// Timestamp extraction
	ts, err := ig.ExtractTimestamp(corrID)
	require.NoError(t, err)
	assert.True(t, ts > 0)
}

func TestSequentialAndHighThroughputGenerators(t *testing.T) {
	seq := idgen.NewSequentialIdGenerator()
	id1 := seq.GenerateSequential("review", "rev_seq_")
	id2 := seq.GenerateSequential("review", "rev_seq_")
	assert.True(t, strings.HasPrefix(id1, "rev_seq_1_"))
	assert.True(t, strings.HasPrefix(id2, "rev_seq_2_"))

	seq.ResetCounter("review")
	idReset := seq.GenerateSequential("review", "rev_seq_")
	assert.True(t, strings.HasPrefix(idReset, "rev_seq_1_"))

	ht := idgen.NewHighThroughputIdGenerator()
	fastID := ht.GenerateFast()
	assert.NotEmpty(t, fastID)

	batch := ht.GenerateBatch(5)
	assert.Len(t, batch, 5)
}

