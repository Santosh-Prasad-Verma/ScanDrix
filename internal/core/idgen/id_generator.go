package idgen

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// ResourcePrefix defines recognized system entity prefixes.
type ResourcePrefix string

const (
	PrefixWorkspace    ResourcePrefix = "ws_"
	PrefixOrganization ResourcePrefix = "org_"
	PrefixRepository   ResourcePrefix = "repo_"
	PrefixReview       ResourcePrefix = "rev_"
	PrefixFinding      ResourcePrefix = "find_"
	PrefixUser         ResourcePrefix = "usr_"
	PrefixTeam         ResourcePrefix = "team_"
	PrefixJob          ResourcePrefix = "job_"
	PrefixEvent        ResourcePrefix = "evt_"
	PrefixMessage      ResourcePrefix = "msg_"
	PrefixToken        ResourcePrefix = "tok_"
	PrefixRule         ResourcePrefix = "rule_"
	PrefixDevice       ResourcePrefix = "dev_"
	PrefixKey          ResourcePrefix = "key_"
	PrefixSession      ResourcePrefix = "sess_"
)

var (
	ErrInvalidPrefixedID = errors.New("invalid prefixed resource identifier")
	ErrInvalidUUIDv7     = errors.New("failed to generate UUIDv7 identifier")
)

const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
const maxValidByte = 248 // 256 / 62 = 4 with remainder; reject >= 248 to eliminate modulo bias

var (
	execCounter uint64
	startTime   = time.Now()
)

var idValidationPatterns = map[string]*regexp.Regexp{
	"execution":   regexp.MustCompile(`^exec_[a-z0-9]{8,}_[A-Za-z0-9]{8}_[a-z0-9]{1,3}$`),
	"correlation": regexp.MustCompile(`^corr_[A-Za-z0-9]{12}_[a-z0-9]{8,}$`),
	"call":        regexp.MustCompile(`^call_[A-Za-z0-9]{6}_[a-z0-9]{6,}$`),
	"session":     regexp.MustCompile(`^sess_[A-Za-z0-9]{10}_[a-z0-9]{8,}$`),
	"tenant":      regexp.MustCompile(`^tenant_[A-Za-z0-9]{8}$`),
}

// IdGenerator mirrors ScanDrix IdGenerator providing collision-resistant IDs with timing info.
type IdGenerator struct{}

// ExecutionID generates a unique execution ID: exec_[timestamp]_[random8]_[counter].
func (ig IdGenerator) ExecutionID() string {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 36)
	rnd := generateRandomBase62(8)
	cnt := atomic.AddUint64(&execCounter, 1) % 1000
	cntStr := strconv.FormatUint(cnt, 36)
	return fmt.Sprintf("exec_%s_%s_%s", ts, rnd, cntStr)
}

// CorrelationID generates a unique correlation ID: corr_[random12]_[timestamp].
func (ig IdGenerator) CorrelationID() string {
	rnd := generateRandomBase62(12)
	ts := strconv.FormatInt(time.Now().UnixMilli(), 36)
	return fmt.Sprintf("corr_%s_%s", rnd, ts)
}

// CallID generates a unique call ID for tool calls: call_[random6]_[perfNow].
func (ig IdGenerator) CallID() string {
	rnd := generateRandomBase62(6)
	perfNow := time.Since(startTime).Microseconds() + 100000000 // ensure minimum 6 chars base36
	perfStr := strconv.FormatInt(perfNow, 36)
	return fmt.Sprintf("call_%s_%s", rnd, perfStr)
}

// SessionID generates a session ID: sess_[random10]_[timestamp].
func (ig IdGenerator) SessionID() string {
	rnd := generateRandomBase62(10)
	ts := strconv.FormatInt(time.Now().UnixMilli(), 36)
	return fmt.Sprintf("sess_%s_%s", rnd, ts)
}

// TenantID generates a tenant ID: tenant_[random8].
func (ig IdGenerator) TenantID() string {
	rnd := generateRandomBase62(8)
	return fmt.Sprintf("tenant_%s", rnd)
}

// GenerateTraceID generates a unique trace ID: trace_[timestamp]_[random6].
func (ig IdGenerator) GenerateTraceID() string {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 36)
	rnd := generateRandomBase62(6)
	return fmt.Sprintf("trace_%s_%s", ts, rnd)
}

// GenerateSpanID generates a span ID: span_[timestamp]_[count].
func (ig IdGenerator) GenerateSpanID() string {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 36)
	rnd := generateRandomBase62(4)
	return fmt.Sprintf("span_%s_%s", ts, rnd)
}

// EventID generates a unique event ID: evt_[random8]_[timestamp].
func (ig IdGenerator) EventID() string {
	rnd := generateRandomBase62(8)
	ts := strconv.FormatInt(time.Now().UnixMilli(), 36)
	return fmt.Sprintf("evt_%s_%s", rnd, ts)
}

// TraceID generates a unique trace ID: trace_[random12]_[timestamp].
func (ig IdGenerator) TraceID() string {
	rnd := generateRandomBase62(12)
	ts := strconv.FormatInt(time.Now().UnixMilli(), 36)
	return fmt.Sprintf("trace_%s_%s", rnd, ts)
}

// SpanID generates a unique span ID: span_[random8].
func (ig IdGenerator) SpanID() string {
	rnd := generateRandomBase62(8)
	return fmt.Sprintf("span_%s", rnd)
}

// MessageID generates a unique message ID: msg_[timestamp]_[random9].
func (ig IdGenerator) MessageID() string {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 36)
	rnd := generateRandomBase62(9)
	return fmt.Sprintf("msg_%s_%s", ts, rnd)
}

// ValidateID validates that the ID conforms to expected regex pattern.
func (ig IdGenerator) ValidateID(id, idType string) bool {
	pattern, ok := idValidationPatterns[idType]
	if !ok {
		return false
	}
	return pattern.MatchString(id)
}

// ExtractTimestamp extracts millisecond Unix epoch from ID if available.
func (ig IdGenerator) ExtractTimestamp(id string) (int64, error) {
	parts := strings.Split(id, "_")
	if len(parts) < 3 {
		return 0, errors.New("id does not contain timestamp segment")
	}
	// Try parsing timestamp from parts[1] (exec_, msg_, trace_) or parts[2] (corr_, sess_, evt_)
	ts, err := strconv.ParseInt(parts[1], 36, 64)
	if err == nil && ts > 0 {
		return ts, nil
	}
	if len(parts) >= 3 {
		ts, err = strconv.ParseInt(parts[2], 36, 64)
		if err == nil && ts > 0 {
			return ts, nil
		}
	}
	return 0, errors.New("unable to parse timestamp from id")
}

// SequentialIdGenerator generates sequential namespace-bound IDs.
type SequentialIdGenerator struct {
	mu       sync.Mutex
	counters map[string]uint64
}

// NewSequentialIdGenerator instantiates a sequential ID generator.
func NewSequentialIdGenerator() *SequentialIdGenerator {
	return &SequentialIdGenerator{
		counters: make(map[string]uint64),
	}
}

// GenerateSequential generates sequential ID: [prefix][next]_[timestamp]_[random4].
func (s *SequentialIdGenerator) GenerateSequential(namespace, prefix string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := s.counters[namespace] + 1
	s.counters[namespace] = next

	ts := strconv.FormatInt(time.Now().UnixMilli(), 36)
	rnd := generateRandomBase62(4)
	return fmt.Sprintf("%s%d_%s_%s", prefix, next, ts, rnd)
}

// ResetCounter resets namespace counter.
func (s *SequentialIdGenerator) ResetCounter(namespace string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.counters, namespace)
}

// HighThroughputIdGenerator generates IDs using pre-allocated buffer for performance.
type HighThroughputIdGenerator struct {
	mu     sync.Mutex
	buffer [16]byte
	idx    int
}

// NewHighThroughputIdGenerator instantiates a high-throughput generator.
func NewHighThroughputIdGenerator() *HighThroughputIdGenerator {
	h := &HighThroughputIdGenerator{}
	_, _ = rand.Read(h.buffer[:])
	return h
}

// GenerateFast generates an ID: [timestamp36]_[random36].
func (h *HighThroughputIdGenerator) GenerateFast() string {
	h.mu.Lock()
	if h.idx >= 12 {
		_, _ = rand.Read(h.buffer[:])
		h.idx = 0
	}
	val := binary.BigEndian.Uint32(h.buffer[h.idx : h.idx+4])
	h.idx += 4
	h.mu.Unlock()

	ts := strconv.FormatInt(time.Now().UnixMilli(), 36)
	rnd := strconv.FormatUint(uint64(val), 36)
	return fmt.Sprintf("%s_%s", ts, rnd)
}

// GenerateBatch produces count fast IDs.
func (h *HighThroughputIdGenerator) GenerateBatch(count int) []string {
	results := make([]string, count)
	ts := strconv.FormatInt(time.Now().UnixMilli(), 36)
	for i := 0; i < count; i++ {
		fast := h.GenerateFast()
		results[i] = fmt.Sprintf("%s_%s_%s", ts, fast, strconv.FormatInt(int64(i), 36))
	}
	return results
}

func generateRandomBase62(length int) string {
	var sb strings.Builder
	sb.Grow(length)
	buf := make([]byte, length*2)

	for sb.Len() < length {
		_, err := rand.Read(buf)
		if err != nil {
			break
		}
		for _, b := range buf {
			if b < maxValidByte {
				sb.WriteByte(base62Chars[b%62])
				if sb.Len() >= length {
					break
				}
			}
		}
	}
	return sb.String()
}

// ExtractUUIDTimestamp extracts the UTC timestamp from an RFC 9562 UUIDv7.
func ExtractUUIDTimestamp(u uuid.UUID) (time.Time, error) {
	if u.Version() != 7 {
		return time.Time{}, errors.New("uuid is not version 7")
	}
	bytes := u[:]
	milli := int64(binary.BigEndian.Uint64([]byte{
		0, 0,
		bytes[0], bytes[1], bytes[2], bytes[3], bytes[4], bytes[5],
	}))
	return time.UnixMilli(milli).UTC(), nil
}

// UUIDv7 generates a standard RFC 9562 time-ordered UUIDv7.
func UUIDv7() (uuid.UUID, error) {
	return uuid.NewV7()
}

// MustUUIDv7 generates a standard UUIDv7 or panics on entropy exhaustion.
func MustUUIDv7() uuid.UUID {
	u, err := uuid.NewV7()
	if err != nil {
		panic(fmt.Sprintf("entropy failure: %v", err))
	}
	return u
}

// PrefixedID returns a typed resource string: prefix + UUIDv7 without hyphens.
func PrefixedID(prefix ResourcePrefix) (string, error) {
	u, err := uuid.NewV7()
	if err != nil {
		return "", ErrInvalidUUIDv7
	}
	rawHex := strings.ReplaceAll(u.String(), "-", "")
	return string(prefix) + rawHex, nil
}

// MustPrefixedID generates a prefixed ID or panics on error.
func MustPrefixedID(prefix ResourcePrefix) string {
	id, err := PrefixedID(prefix)
	if err != nil {
		panic(err)
	}
	return id
}

// ParsePrefixedID validates prefix and decodes embedded UUID.
func ParsePrefixedID(input string, expectedPrefix ResourcePrefix) (uuid.UUID, error) {
	if !strings.HasPrefix(input, string(expectedPrefix)) {
		return uuid.Nil, fmt.Errorf("%w: expected prefix '%s'", ErrInvalidPrefixedID, expectedPrefix)
	}
	rawHex := strings.TrimPrefix(input, string(expectedPrefix))
	if len(rawHex) != 32 {
		return uuid.Nil, fmt.Errorf("%w: invalid hex payload length %d", ErrInvalidPrefixedID, len(rawHex))
	}
	formatted := fmt.Sprintf("%s-%s-%s-%s-%s",
		rawHex[0:8], rawHex[8:12], rawHex[12:16], rawHex[16:20], rawHex[20:32],
	)
	return uuid.Parse(formatted)
}

// TeamCLIKey generates a cryptographically secure team CLI access token prefixed with "scandrix_live_".
func TeamCLIKey() (string, error) {
	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed generating entropy: %w", err)
	}
	return "scandrix_live_" + hex.EncodeToString(bytes), nil
}

// SecureToken produces a high-entropy hex string.
func SecureToken(byteLength int) (string, error) {
	if byteLength <= 0 {
		byteLength = 32
	}
	bytes := make([]byte, byteLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed generating entropy: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}
