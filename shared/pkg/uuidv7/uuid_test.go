package uuidv7

import (
	"sync"
	"testing"
	"time"
)

func TestUUIDv7Generation(t *testing.T) {
	u1 := MustNew()
	time.Sleep(2 * time.Millisecond)
	u2 := MustNew()

	// Verify UUID Version is 7
	if u1.Version() != 7 {
		t.Fatalf("expected UUID version 7, got %d", u1.Version())
	}
	if u2.Version() != 7 {
		t.Fatalf("expected UUID version 7, got %d", u2.Version())
	}

	// Verify time ordering (u1 string < u2 string lexicographically)
	if u1.String() >= u2.String() {
		t.Fatalf("expected u1 (%s) < u2 (%s)", u1.String(), u2.String())
	}

	// Verify extracted timestamp is close to now
	now := time.Now()
	t1 := ExtractTime(u1)
	diff := now.Sub(t1)
	if diff < 0 || diff > 2*time.Second {
		t.Fatalf("extracted timestamp %v too far from now %v (diff: %v)", t1, now, diff)
	}
}

func TestUUIDv7ConcurrencyUniqueness(t *testing.T) {
	const count = 5000
	var mu sync.Mutex
	seen := make(map[string]struct{}, count)
	var wg sync.WaitGroup

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			u := MustNew()
			str := u.String()

			mu.Lock()
			if _, exists := seen[str]; exists {
				t.Errorf("collision detected for UUID: %s", str)
			}
			seen[str] = struct{}{}
			mu.Unlock()
		}()
	}

	wg.Wait()
	if len(seen) != count {
		t.Fatalf("expected %d unique UUIDs, got %d", count, len(seen))
	}
}
