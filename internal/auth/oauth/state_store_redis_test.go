package oauth_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/scandrix/backend/internal/auth/oauth"
)

func TestRedisStateStoreFallbackWhenOffline(t *testing.T) {
	// Point to offline Redis port to test fallback resilience
	dummyClient := redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:63799",
		DialTimeout: 20 * time.Millisecond,
		ReadTimeout: 20 * time.Millisecond,
		MaxRetries:  -1,
	})
	defer dummyClient.Close()

	store := oauth.NewRedisStateStore(dummyClient, 5*time.Minute)

	state, err := store.Generate(oauth.ProviderGitHub)
	if err != nil {
		t.Fatalf("expected state generation to succeed with fallback, got: %v", err)
	}

	if state == "" {
		t.Fatal("expected non-empty state token")
	}

	// Validate should succeed via memory fallback
	if !store.Validate(state, oauth.ProviderGitHub) {
		t.Error("expected state to validate via memory fallback")
	}

	// Replay attack: second use must fail
	if store.Validate(state, oauth.ProviderGitHub) {
		t.Error("expected state to be consumed and rejected on second use")
	}
}

func TestRedisStateStoreWithLiveRedisIfAvailable(t *testing.T) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://127.0.0.1:6379/0"
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Skip("Valid REDIS_URL not configured; skipping live Redis test")
	}

	client := redis.NewClient(opt)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		t.Skip("Live Redis unavailable; skipping live Redis test")
	}

	store := oauth.NewRedisStateStore(client, 2*time.Second)

	state, err := store.Generate(oauth.ProviderGitLab)
	if err != nil {
		t.Fatalf("failed to generate state on live Redis: %v", err)
	}

	// First validation: match expected
	if !store.Validate(state, oauth.ProviderGitLab) {
		t.Error("expected valid state from Redis")
	}

	// Second validation: replay must be rejected
	if store.Validate(state, oauth.ProviderGitLab) {
		t.Error("expected state consumed and rejected from Redis on second use")
	}
}

func TestRedisStateStoreConcurrentOperations(t *testing.T) {
	store := oauth.NewStateStore(5 * time.Minute)

	var wg sync.WaitGroup
	const iterations = 100

	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			state, err := store.Generate(oauth.ProviderGitHub)
			if err != nil || state == "" {
				t.Errorf("concurrent generate failed: %v", err)
				return
			}
			if !store.Validate(state, oauth.ProviderGitHub) {
				t.Errorf("concurrent validate failed for state %s", state)
			}
		}()
	}

	wg.Wait()
}
