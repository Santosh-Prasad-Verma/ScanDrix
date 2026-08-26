package uuidv7

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// New generates a new time-ordered UUIDv7 according to RFC 9562.
func New() (uuid.UUID, error) {
	var val [16]byte

	// 1. Current Unix timestamp in milliseconds (48 bits)
	ms := uint64(time.Now().UnixMilli())
	val[0] = byte(ms >> 40)
	val[1] = byte(ms >> 32)
	val[2] = byte(ms >> 24)
	val[3] = byte(ms >> 16)
	val[4] = byte(ms >> 8)
	val[5] = byte(ms)

	// 2. Fill the remaining 10 bytes (80 bits) with cryptographic random data
	if _, err := rand.Read(val[6:]); err != nil {
		return uuid.Nil, fmt.Errorf("failed to generate random bytes for uuidv7: %w", err)
	}

	// 3. Set Version = 7 (bits 4-7 of byte 6: 0111)
	val[6] = (val[6] & 0x0F) | 0x70

	// 4. Set Variant = RFC 4122 (bits 6-7 of byte 8: 10)
	val[8] = (val[8] & 0x3F) | 0x80

	return uuid.UUID(val), nil
}

// MustNew generates a UUIDv7 and panics if random byte generation fails.
func MustNew() uuid.UUID {
	u, err := New()
	if err != nil {
		panic(err)
	}
	return u
}

// ExtractTime returns the timestamp embedded within a UUIDv7.
func ExtractTime(u uuid.UUID) time.Time {
	var msBytes [8]byte
	copy(msBytes[2:], u[:6])
	ms := binary.BigEndian.Uint64(msBytes[:])
	return time.UnixMilli(int64(ms))
}
