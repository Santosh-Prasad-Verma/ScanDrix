package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/core/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sampleData struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func TestCacheServiceLifecycle(t *testing.T) {
	cs := cache.NewCacheService(nil) // in-memory
	defer cs.Close()

	ctx := context.Background()

	// 1. AddToCache
	data := sampleData{Name: "scandrix", Count: 42}
	err := cs.AddToCache(ctx, "test_key", data, 500) // 500ms TTL
	require.NoError(t, err)

	// 2. CacheExists
	exists, err := cs.CacheExists(ctx, "test_key")
	require.NoError(t, err)
	assert.True(t, exists)

	// 3. GetFromCache
	var retrieved sampleData
	found, err := cs.GetFromCache(ctx, "test_key", &retrieved)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "scandrix", retrieved.Name)
	assert.Equal(t, 42, retrieved.Count)

	// 4. GetTTL
	ttl, err := cs.GetTTL(ctx, "test_key")
	require.NoError(t, err)
	assert.True(t, ttl > 0 && ttl <= 500)

	// 5. RemoveFromCache
	err = cs.RemoveFromCache(ctx, "test_key")
	require.NoError(t, err)

	exists, err = cs.CacheExists(ctx, "test_key")
	require.NoError(t, err)
	assert.False(t, exists)

	// 6. TTL Expiration
	err = cs.AddToCache(ctx, "expire_key", data, 20) // 20ms TTL
	require.NoError(t, err)
	time.Sleep(30 * time.Millisecond)

	found, err = cs.GetFromCache(ctx, "expire_key", &retrieved)
	require.NoError(t, err)
	assert.False(t, found)
}

func TestCacheServiceClearAndReset(t *testing.T) {
	cs := cache.NewCacheService(nil)
	defer cs.Close()

	ctx := context.Background()
	_ = cs.AddToCache(ctx, "k1", "v1", 60000)
	_ = cs.AddToCache(ctx, "k2", "v2", 60000)

	assert.NoError(t, cs.ClearCache(ctx))

	exists, _ := cs.CacheExists(ctx, "k1")
	assert.False(t, exists)
	exists, _ = cs.CacheExists(ctx, "k2")
	assert.False(t, exists)
}

func TestCacheServiceGetMultipleFromCache(t *testing.T) {
	cs := cache.NewCacheService(nil)
	defer cs.Close()

	ctx := context.Background()
	_ = cs.AddToCache(ctx, "user:1", sampleData{Name: "Alice", Count: 1}, 60000)
	_ = cs.AddToCache(ctx, "user:2", sampleData{Name: "Bob", Count: 2}, 60000)

	results, err := cs.GetMultipleFromCache(ctx, []string{"user:1", "user:2", "user:99"})
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Contains(t, results, "user:1")
	assert.Contains(t, results, "user:2")
	assert.NotContains(t, results, "user:99")

	// Empty keys slice
	empty, err := cs.GetMultipleFromCache(ctx, []string{})
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestCacheServiceDeleteByKeyPattern(t *testing.T) {
	cs := cache.NewCacheService(nil)
	defer cs.Close()

	ctx := context.Background()
	_ = cs.AddToCache(ctx, "review:pr:101", "review 101", 60000)
	_ = cs.AddToCache(ctx, "review:pr:102", "review 102", 60000)
	_ = cs.AddToCache(ctx, "review:commit:201", "review 201", 60000)
	_ = cs.AddToCache(ctx, "user:session:abc", "session data", 60000)

	// Delete pattern review:pr:*
	err := cs.DeleteByKeyPattern(ctx, "review:pr:*")
	require.NoError(t, err)

	exists101, _ := cs.CacheExists(ctx, "review:pr:101")
	assert.False(t, exists101)
	exists102, _ := cs.CacheExists(ctx, "review:pr:102")
	assert.False(t, exists102)

	// review:commit:201 and user:session:abc should still exist
	exists201, _ := cs.CacheExists(ctx, "review:commit:201")
	assert.True(t, exists201)
	existsUser, _ := cs.CacheExists(ctx, "user:session:abc")
	assert.True(t, existsUser)

	// Delete with wildcard
	err = cs.DeleteByKeyPattern(ctx, "user:*")
	require.NoError(t, err)
	existsUserAfter, _ := cs.CacheExists(ctx, "user:session:abc")
	assert.False(t, existsUserAfter)
}

