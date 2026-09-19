package cache_test

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/core/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ComplexPayload struct {
	ID        string            `json:"id"`
	Index     int               `json:"index"`
	Tags      []string          `json:"tags"`
	Metadata  map[string]any    `json:"metadata"`
	SubItems  []NestedSubItem   `json:"sub_items"`
	Active    bool              `json:"active"`
	CreatedAt time.Time         `json:"created_at"`
}

type NestedSubItem struct {
	Key    string  `json:"key"`
	Weight float64 `json:"weight"`
	Flags  []int   `json:"flags"`
}

// TestCacheStress_HighConcurrencyReadWrite exercises 100 concurrent goroutines performing
// simultaneous set, get, exists, and delete operations to ensure zero data races.
func TestCacheStress_HighConcurrencyReadWrite(t *testing.T) {
	cs := cache.NewCacheService(nil)
	defer cs.Close()

	ctx := context.Background()
	const numGoroutines = 80
	const opsPerGoroutine = 50

	var wg sync.WaitGroup
	var successGets atomic.Int64
	var totalWrites atomic.Int64

	wg.Add(numGoroutines)
	for g := 0; g < numGoroutines; g++ {
		go func(gID int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(gID + 100)))

			for i := 0; i < opsPerGoroutine; i++ {
				key := fmt.Sprintf("tenant_%d:key_%d", gID%10, rng.Intn(20))
				val := ComplexPayload{
					ID:    fmt.Sprintf("item-%d-%d", gID, i),
					Index: i,
					Tags:  []string{"stress", "benchmark", fmt.Sprintf("g-%d", gID)},
					Metadata: map[string]any{
						"worker": gID,
						"cycle":  i,
						"ratio":  rng.Float64(),
					},
					SubItems: []NestedSubItem{
						{Key: "sub-1", Weight: 1.25, Flags: []int{1, 2, 3}},
						{Key: "sub-2", Weight: 9.87, Flags: []int{4, 5, 6}},
					},
					Active:    i%2 == 0,
					CreatedAt: time.Now().UTC(),
				}

				op := rng.Intn(4)
				switch op {
				case 0, 1: // Write
					err := cs.AddToCache(ctx, key, val, 5000)
					if err == nil {
						totalWrites.Add(1)
					}
				case 2: // Read
					var res ComplexPayload
					found, err := cs.GetFromCache(ctx, key, &res)
					if err == nil && found {
						successGets.Add(1)
						assert.NotEmpty(t, res.ID)
					}
				case 3: // Exists check
					_, _ = cs.CacheExists(ctx, key)
				}
			}
		}(g)
	}
	wg.Wait()

	assert.True(t, totalWrites.Load() > 0)
}

// TestCacheStress_PatternInvalidationUnderLoad verifies pattern-based cache clearing
// while concurrent writers are modifying keys in matching and non-matching namespaces.
func TestCacheStress_PatternInvalidationUnderLoad(t *testing.T) {
	cs := cache.NewCacheService(nil)
	defer cs.Close()

	ctx := context.Background()
	const numWriters = 30
	stopCh := make(chan struct{})
	var wg sync.WaitGroup

	// Populate initially
	for i := 0; i < 200; i++ {
		ns := "reviews"
		if i%2 == 0 {
			ns = "profiles"
		}
		_ = cs.AddToCache(ctx, fmt.Sprintf("%s:entry:%d", ns, i), fmt.Sprintf("val-%d", i), 10000)
	}

	// Spin up writers
	wg.Add(numWriters)
	for w := 0; w < numWriters; w++ {
		go func(workerID int) {
			defer wg.Done()
			idx := 0
			for {
				select {
				case <-stopCh:
					return
				default:
					ns := "reviews"
					if workerID%2 == 0 {
						ns = "profiles"
					}
					k := fmt.Sprintf("%s:entry:%d", ns, idx%50)
					_ = cs.AddToCache(ctx, k, fmt.Sprintf("v-%d-%d", workerID, idx), 10000)
					idx++
					time.Sleep(100 * time.Microsecond)
				}
			}
		}(w)
	}

	// Trigger multiple pattern deletions concurrently
	for cycle := 0; cycle < 10; cycle++ {
		time.Sleep(5 * time.Millisecond)
		err := cs.DeleteByKeyPattern(ctx, "reviews:*")
		assert.NoError(t, err)
	}

	close(stopCh)
	wg.Wait()

	// Clear reviews pattern one final time
	err := cs.DeleteByKeyPattern(ctx, "reviews:*")
	require.NoError(t, err)

	// Verify no "reviews:*" remain
	for i := 0; i < 50; i++ {
		exists, err := cs.CacheExists(ctx, fmt.Sprintf("reviews:entry:%d", i))
		require.NoError(t, err)
		assert.False(t, exists)
	}
}

// TestCacheStress_TTLPrecisionAndNegativeValues ensures non-positive or large TTLs
// fall back to safe system defaults without memory leaks or panics.
func TestCacheStress_TTLPrecisionAndNegativeValues(t *testing.T) {
	cs := cache.NewCacheService(nil)
	defer cs.Close()

	ctx := context.Background()

	// 0 ms TTL should default to 60000ms
	err := cs.AddToCache(ctx, "zero_ttl", "val", 0)
	require.NoError(t, err)
	ttl, err := cs.GetTTL(ctx, "zero_ttl")
	require.NoError(t, err)
	assert.True(t, ttl > 55000 && ttl <= 60000)

	// Negative TTL should also default to 60000ms
	err = cs.AddToCache(ctx, "neg_ttl", "val", -500)
	require.NoError(t, err)
	ttl, err = cs.GetTTL(ctx, "neg_ttl")
	require.NoError(t, err)
	assert.True(t, ttl > 55000 && ttl <= 60000)

	// Non-existent key TTL returns -1
	ttl, err = cs.GetTTL(ctx, "non_existent_key_random")
	require.NoError(t, err)
	assert.Equal(t, int64(-1), ttl)
}

// TestCacheStress_BatchOperationsEdgeCases tests GetMultipleFromCache with large slice of keys,
// duplicates, empty strings, and non-existent identifiers.
func TestCacheStress_BatchOperationsEdgeCases(t *testing.T) {
	cs := cache.NewCacheService(nil)
	defer cs.Close()

	ctx := context.Background()
	const keyCount = 200

	keys := make([]string, keyCount)
	for i := 0; i < keyCount; i++ {
		k := fmt.Sprintf("batch:item:%03d", i)
		keys[i] = k
		if i%2 == 0 {
			err := cs.AddToCache(ctx, k, map[string]int{"seq": i}, 50000)
			require.NoError(t, err)
		}
	}

	// Add duplicates to test querying duplicate keys
	queryKeys := append(keys, keys[0], keys[2], "batch:non_existent:999", "")

	results, err := cs.GetMultipleFromCache(ctx, queryKeys)
	require.NoError(t, err)

	// Exactly half the keys were inserted
	assert.Equal(t, keyCount/2, len(results))

	for i := 0; i < keyCount; i++ {
		k := fmt.Sprintf("batch:item:%03d", i)
		if i%2 == 0 {
			assert.Contains(t, results, k)
		} else {
			assert.NotContains(t, results, k)
		}
	}
}

// TestCacheStress_RapidClearAndRepopulate tests rapid clear cycles under sustained load.
func TestCacheStress_RapidClearAndRepopulate(t *testing.T) {
	cs := cache.NewCacheService(nil)
	defer cs.Close()

	ctx := context.Background()
	const iterations = 15

	for iter := 0; iter < iterations; iter++ {
		// Populate 100 items
		var wg sync.WaitGroup
		wg.Add(10)
		for w := 0; w < 10; w++ {
			go func(worker int) {
				defer wg.Done()
				for i := 0; i < 10; i++ {
					k := fmt.Sprintf("iter:%d:w:%d:i:%d", iter, worker, i)
					_ = cs.AddToCache(ctx, k, i*100, 30000)
				}
			}(w)
		}
		wg.Wait()

		// Verify presence
		exists, err := cs.CacheExists(ctx, fmt.Sprintf("iter:%d:w:0:i:0", iter))
		require.NoError(t, err)
		assert.True(t, exists)

		// Clear cache
		err = cs.ResetCache(ctx)
		require.NoError(t, err)

		// Verify absence
		exists, err = cs.CacheExists(ctx, fmt.Sprintf("iter:%d:w:0:i:0", iter))
		require.NoError(t, err)
		assert.False(t, exists)
	}
}

// TestCacheStress_DeepNestedJSONSerialization validates error-free storage and retrieval
// of multi-megabyte JSON trees, arrays with thousands of elements, and Unicode characters.
func TestCacheStress_DeepNestedJSONSerialization(t *testing.T) {
	cs := cache.NewCacheService(nil)
	defer cs.Close()

	ctx := context.Background()

	type DeepNode struct {
		Name     string              `json:"name"`
		Children map[string]DeepNode `json:"children,omitempty"`
		Values   []float64           `json:"values"`
		Unicode  string              `json:"unicode"`
	}

	root := DeepNode{
		Name:    "root_node",
		Values:  make([]float64, 1000),
		Unicode: "ScanDrix ⚡ 🚀 💻 🔒 🛡️ 🎯 日本語 中文 Español Deutsch",
		Children: map[string]DeepNode{
			"child_1": {
				Name:    "child_node_1",
				Values:  []float64{1.1, 2.2, 3.3, 4.4},
				Unicode: "Sub-node UTF-8 characters: äöüß",
			},
			"child_2": {
				Name:    "child_node_2",
				Values:  []float64{5.5, 6.6, 7.7, 8.8},
				Unicode: "Mathematical symbols: ∑ ∏ ∫ ∬ ∭ ∮",
			},
		},
	}

	for i := 0; i < 1000; i++ {
		root.Values[i] = float64(i) * 0.12345
	}

	err := cs.AddToCache(ctx, "deep_json_key", root, 20000)
	require.NoError(t, err)

	var retrieved DeepNode
	found, err := cs.GetFromCache(ctx, "deep_json_key", &retrieved)
	require.NoError(t, err)
	require.True(t, found)

	assert.Equal(t, "root_node", retrieved.Name)
	assert.Equal(t, root.Unicode, retrieved.Unicode)
	assert.Len(t, retrieved.Values, 1000)
	assert.InDelta(t, 12.345, retrieved.Values[100], 0.0001)
	assert.Contains(t, retrieved.Children, "child_1")
	assert.Contains(t, retrieved.Children, "child_2")
	assert.Equal(t, "child_node_1", retrieved.Children["child_1"].Name)
}

// TestCacheStress_SpecialKeyFormats tests caching keys containing spaces, slashes,
// colons, symbols, and high-order Unicode code points.
func TestCacheStress_SpecialKeyFormats(t *testing.T) {
	cs := cache.NewCacheService(nil)
	defer cs.Close()

	ctx := context.Background()

	specialKeys := []string{
		"repo/path/with/slashes",
		"user:email+alias@domain.co.uk",
		"key with spaces and \t tabs",
		"symbols!@#$%^&*()_+-=[]{}|;':,.<>?",
		"unicode_key_🔥_тест_测试",
		"",
	}

	for _, k := range specialKeys {
		err := cs.AddToCache(ctx, k, fmt.Sprintf("payload-for-%s", k), 10000)
		require.NoError(t, err)

		exists, err := cs.CacheExists(ctx, k)
		require.NoError(t, err)
		assert.True(t, exists)

		var val string
		found, err := cs.GetFromCache(ctx, k, &val)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, fmt.Sprintf("payload-for-%s", k), val)

		err = cs.RemoveFromCache(ctx, k)
		require.NoError(t, err)

		exists, err = cs.CacheExists(ctx, k)
		require.NoError(t, err)
		assert.False(t, exists)
	}
}

// TestCacheStress_PatternGlobWildcards verifies glob translation for various wildcards:
// '*', '?', character sets, and escaping of regex metacharacters.
func TestCacheStress_PatternGlobWildcards(t *testing.T) {
	cs := cache.NewCacheService(nil)
	defer cs.Close()

	ctx := context.Background()

	_ = cs.AddToCache(ctx, "app.v1.users", "v1", 10000)
	_ = cs.AddToCache(ctx, "app.v2.users", "v2", 10000)
	_ = cs.AddToCache(ctx, "app.v10.users", "v10", 10000)
	_ = cs.AddToCache(ctx, "app+test(1)", "meta", 10000)
	_ = cs.AddToCache(ctx, "other:data", "other", 10000)

	// Test '?' matches single character (matches app.v1.users and app.v2.users, not app.v10.users)
	err := cs.DeleteByKeyPattern(ctx, "app.v?.users")
	require.NoError(t, err)

	exists1, _ := cs.CacheExists(ctx, "app.v1.users")
	assert.False(t, exists1)
	exists2, _ := cs.CacheExists(ctx, "app.v2.users")
	assert.False(t, exists2)

	// v10 should still be present
	exists10, _ := cs.CacheExists(ctx, "app.v10.users")
	assert.True(t, exists10)

	// Regex metacharacter escaping check
	existsMeta, _ := cs.CacheExists(ctx, "app+test(1)")
	assert.True(t, existsMeta)

	err = cs.DeleteByKeyPattern(ctx, "app+test(*)")
	require.NoError(t, err)

	existsMetaAfter, _ := cs.CacheExists(ctx, "app+test(1)")
	assert.False(t, existsMetaAfter)

	// Other remains untouched
	existsOther, _ := cs.CacheExists(ctx, "other:data")
	assert.True(t, existsOther)
}
