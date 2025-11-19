// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package filter

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var (
	cfg           *rest.Config
	kubeClient    kubernetes.Interface
	runtimeClient client.Client
)

func TestMain(m *testing.M) {
	err := os.Setenv("POD_NAMESPACE", "default")
	if err != nil {
		panic(err)
	}
	err = os.Setenv("MANAGER_TESTING", "true")
	if err != nil {
		panic(err)
	}

	// start testenv
	testenv := &envtest.Environment{}

	cfg, err = testenv.Start()
	if err != nil {
		panic(err)
	}

	if cfg == nil {
		panic(fmt.Errorf("empty kubeconfig!"))
	}

	kubeClient, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}

	runtimeClient, err = client.New(cfg, client.Options{})
	if err != nil {
		panic(err)
	}

	// run testings
	code := m.Run()

	// stop testenv
	if err := testenv.Stop(); err != nil {
		panic(err)
	}
	os.Exit(code)
}

func TestTimeFilter(t *testing.T) {
	// init the time cancel to configMap
	ctx, cancel := context.WithCancel(context.Background())
	eventType := "event.managedcluster"
	configs.SetAgentConfig(&configs.AgentConfig{PodNamespace: "default"})

	CacheSyncInterval = 1 * time.Second
	err := LaunchTimeFilter(ctx, runtimeClient, "default", "topic1")
	assert.Nil(t, err)

	fmt.Println(">> verify1: the filter create the configmap if it isn't exist")
	cm, err := configs.GetSyncStateConfigMap(ctx, runtimeClient)
	assert.Nil(t, err)
	utils.PrettyPrint(cm)

	fmt.Println(">> verify2: the configmap cached the key with toipc prefix")
	cacheTime := time.Now()
	CacheTime(eventType, cacheTime)
	time.Sleep(2 * time.Second)
	// check the cache time is synced to the configMap
	err = runtimeClient.Get(ctx, client.ObjectKeyFromObject(cm), cm)
	assert.Nil(t, err)
	utils.PrettyPrint(cm)
	cachedTime, err := time.Parse(configs.AGENT_SYNC_STATE_TIME_FORMAT_VALUE, cm.Data[getConfigMapKey(eventType)])
	assert.Nil(t, err)
	assert.True(t, cachedTime.Equal(cacheTime))
	cancel()

	fmt.Println(">> verify3: update the cache with a expired time, verify the cached time isn't changed")
	// reload the time from configMap
	ctx, cancel = context.WithCancel(context.Background())
	RegisterTimeFilter(eventType)
	err = LaunchTimeFilter(ctx, runtimeClient, "default", "topic1")
	assert.Nil(t, err)

	// update the cache with a expired time, verify the cached time isn't changed
	expiredTime := cacheTime.Add(-10 * time.Second)
	assert.False(t, Newer(eventType, expiredTime))

	CacheTime(eventType, expiredTime)
	time.Sleep(2 * time.Second)

	err = runtimeClient.Get(ctx, client.ObjectKeyFromObject(cm), cm)
	assert.Nil(t, err)
	utils.PrettyPrint(cm)

	cachedTime, err = time.Parse(configs.AGENT_SYNC_STATE_TIME_FORMAT_VALUE, cm.Data[getConfigMapKey(eventType)])
	assert.Nil(t, err)
	assert.True(t, cachedTime.Equal(cacheTime))
	cancel()

	fmt.Println(">> verify4: update the cache with a new topic, the expired time, verify the cached time is change")
	ctx, cancel = context.WithCancel(context.Background())
	RegisterTimeFilter(eventType)
	err = LaunchTimeFilter(ctx, runtimeClient, "default", "topic2")
	assert.Nil(t, err)

	// a new topic with init cache
	assert.True(t, Newer(eventType, expiredTime))

	// update the cache with a expired time
	CacheTime(eventType, expiredTime)
	time.Sleep(2 * time.Second)

	err = runtimeClient.Get(ctx, client.ObjectKeyFromObject(cm), cm)
	assert.Nil(t, err)
	utils.PrettyPrint(cm)

	cachedTime, err = time.Parse(configs.AGENT_SYNC_STATE_TIME_FORMAT_VALUE, cm.Data[getConfigMapKey(eventType)])
	assert.Nil(t, err)
	assert.True(t, cachedTime.Equal(expiredTime))
	cancel()

	fmt.Println(">> verify5: don't lose events with similar time")
	similiarTime := time.Now().Add(10 * time.Second)
	assert.True(t, Newer(eventType, similiarTime))
	CacheTime(eventType, similiarTime)
	assert.True(t, Newer(eventType, similiarTime))
	CacheTime(eventType, similiarTime.Add(2*time.Second))
	assert.True(t, Newer(eventType, similiarTime))
	CacheTime(eventType, similiarTime.Add(5*time.Second))
	assert.False(t, Newer(eventType, similiarTime))
}

// TestConcurrentReads tests concurrent read access to the cache
func TestConcurrentReads(t *testing.T) {
	// Reset the cache for this test
	cacheMutex.Lock()
	eventTimeCache = make(map[string]time.Time)
	lastEventTimeCache = make(map[string]time.Time)
	cacheMutex.Unlock()

	// Setup test data
	testKeys := []string{"key1", "key2", "key3", "key4", "key5"}
	for _, key := range testKeys {
		CacheTime(key, time.Now())
	}

	// Run concurrent reads
	var wg sync.WaitGroup
	numGoroutines := 50
	numReadsPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numReadsPerGoroutine; j++ {
				for _, key := range testKeys {
					_ = Newer(key, time.Now())
				}
			}
		}()
	}

	wg.Wait()
	// If there were race conditions, this test would fail or panic
}

// TestConcurrentWrites tests concurrent write access to the cache
func TestConcurrentWrites(t *testing.T) {
	// Reset the cache for this test
	cacheMutex.Lock()
	eventTimeCache = make(map[string]time.Time)
	lastEventTimeCache = make(map[string]time.Time)
	cacheMutex.Unlock()

	// Run concurrent writes
	var wg sync.WaitGroup
	numGoroutines := 50
	numWritesPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < numWritesPerGoroutine; j++ {
				key := fmt.Sprintf("key%d", goroutineID%5)
				CacheTime(key, time.Now())
			}
		}(i)
	}

	wg.Wait()

	// Verify that all keys were written
	cacheMutex.RLock()
	assert.Greater(t, len(eventTimeCache), 0, "Cache should contain entries after concurrent writes")
	cacheMutex.RUnlock()
}

// TestConcurrentReadsAndWrites tests concurrent read and write access
func TestConcurrentReadsAndWrites(t *testing.T) {
	// Reset the cache for this test
	cacheMutex.Lock()
	eventTimeCache = make(map[string]time.Time)
	lastEventTimeCache = make(map[string]time.Time)
	cacheMutex.Unlock()

	// Setup initial data
	testKeys := []string{"key1", "key2", "key3"}
	for _, key := range testKeys {
		CacheTime(key, time.Now())
	}

	var wg sync.WaitGroup
	numGoroutines := 100
	iterations := 100

	// Start readers
	for i := 0; i < numGoroutines/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				for _, key := range testKeys {
					_ = Newer(key, time.Now())
				}
			}
		}()
	}

	// Start writers
	for i := 0; i < numGoroutines/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				for _, key := range testKeys {
					CacheTime(key, time.Now().Add(time.Duration(j)*time.Millisecond))
				}
			}
		}()
	}

	wg.Wait()
	// If there were race conditions, this test would fail or panic
}

// TestConcurrentRegisterTimeFilter tests concurrent calls to RegisterTimeFilter
func TestConcurrentRegisterTimeFilter(t *testing.T) {
	// Reset the cache for this test
	cacheMutex.Lock()
	eventTimeCache = make(map[string]time.Time)
	lastEventTimeCache = make(map[string]time.Time)
	cacheMutex.Unlock()

	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("filter%d", id%5)
			RegisterTimeFilter(key)
		}(i)
	}

	wg.Wait()

	// Verify that all filters were registered
	cacheMutex.RLock()
	assert.Greater(t, len(eventTimeCache), 0, "Cache should contain registered filters")
	cacheMutex.RUnlock()
}

// TestConcurrentMixedOperations tests all operations happening concurrently
func TestConcurrentMixedOperations(t *testing.T) {
	// Reset the cache for this test
	cacheMutex.Lock()
	eventTimeCache = make(map[string]time.Time)
	lastEventTimeCache = make(map[string]time.Time)
	cacheMutex.Unlock()

	var wg sync.WaitGroup
	numGoroutines := 20
	iterations := 50

	// RegisterTimeFilter operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				key := fmt.Sprintf("key%d", id%10)
				RegisterTimeFilter(key)
			}
		}(i)
	}

	// CacheTime operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				key := fmt.Sprintf("key%d", id%10)
				CacheTime(key, time.Now().Add(time.Duration(j)*time.Millisecond))
			}
		}(i)
	}

	// Newer operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				key := fmt.Sprintf("key%d", id%10)
				_ = Newer(key, time.Now())
			}
		}(i)
	}

	wg.Wait()

	// Verify the cache is in a consistent state
	cacheMutex.RLock()
	eventCacheLen := len(eventTimeCache)
	lastEventCacheLen := len(lastEventTimeCache)
	cacheMutex.RUnlock()

	assert.Greater(t, eventCacheLen, 0, "Event cache should have entries")
	assert.Greater(t, lastEventCacheLen, 0, "Last event cache should have entries")
}

// TestCacheTimeWithSameKey tests that CacheTime correctly handles updates for the same key
func TestCacheTimeWithSameKey(t *testing.T) {
	// Reset the cache
	cacheMutex.Lock()
	eventTimeCache = make(map[string]time.Time)
	lastEventTimeCache = make(map[string]time.Time)
	cacheMutex.Unlock()

	key := "test-key"
	initialTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	newerTime := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	olderTime := time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC)

	// Set initial time
	CacheTime(key, initialTime)

	cacheMutex.RLock()
	cached := eventTimeCache[key]
	cacheMutex.RUnlock()
	assert.Equal(t, initialTime, cached)

	// Update with newer time - should update
	CacheTime(key, newerTime)

	cacheMutex.RLock()
	cached = eventTimeCache[key]
	cacheMutex.RUnlock()
	assert.Equal(t, newerTime, cached)

	// Update with older time - should not update
	CacheTime(key, olderTime)

	cacheMutex.RLock()
	cached = eventTimeCache[key]
	cacheMutex.RUnlock()
	assert.Equal(t, newerTime, cached, "Cache should not be updated with older time")
}

// TestNewerWithDeltaDuration tests the DeltaDuration tolerance in Newer function
func TestNewerWithDeltaDuration(t *testing.T) {
	// Reset the cache
	cacheMutex.Lock()
	eventTimeCache = make(map[string]time.Time)
	lastEventTimeCache = make(map[string]time.Time)
	cacheMutex.Unlock()

	key := "test-delta"
	baseTime := time.Now()

	CacheTime(key, baseTime)

	// Time within DeltaDuration should be considered newer
	withinDelta := baseTime.Add(1 * time.Second)
	assert.True(t, Newer(key, withinDelta), "Time within DeltaDuration should be newer")

	// Time before the (cached - DeltaDuration) should not be newer
	beforeDelta := baseTime.Add(-4 * time.Second)
	assert.False(t, Newer(key, beforeDelta), "Time before (cached - DeltaDuration) should not be newer")

	// Time exactly at DeltaDuration boundary
	atDelta := baseTime.Add(-DeltaDuration)
	assert.False(t, Newer(key, atDelta), "Time exactly at -DeltaDuration should not be newer")
}
