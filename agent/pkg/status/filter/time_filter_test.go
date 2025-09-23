// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package filter

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

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

	CacheSyncInterval = 1 * time.Second
	err := LaunchTimeFilter(ctx, runtimeClient, "default", "topic1")
	assert.Nil(t, err)

	fmt.Println(">> verify1: the filter create the configmap if it isn't exist")
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: CACHE_CONFIG_NAME, Namespace: "default"}}
	err = runtimeClient.Get(ctx, client.ObjectKeyFromObject(cm), cm)
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
	cachedTime, err := time.Parse(CACHE_TIME_FORMAT, cm.Data[getConfigMapKey(eventType)])
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

	cachedTime, err = time.Parse(CACHE_TIME_FORMAT, cm.Data[getConfigMapKey(eventType)])
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

	cachedTime, err = time.Parse(CACHE_TIME_FORMAT, cm.Data[getConfigMapKey(eventType)])
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
