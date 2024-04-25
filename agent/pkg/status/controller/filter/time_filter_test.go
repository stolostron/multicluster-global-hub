// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package filter

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stolostron/multicluster-global-hub/pkg/utils"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
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
	// init the time cache to configMap
	initCtx, initCancel := context.WithCancel(context.Background())

	eventTimeCacheInterval = 1 * time.Second
	err := LaunchTimeFilter(initCtx, runtimeClient, "default")
	assert.Nil(t, err)

	// the configMap can be created
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: CACHE_CONFIG_NAME, Namespace: "default"}}
	err = runtimeClient.Get(initCtx, client.ObjectKeyFromObject(cm), cm)
	assert.Nil(t, err)
	utils.PrettyPrint(cm)

	// update the cache time
	testTime := time.Now()
	fmt.Println("test time", testTime)
	CacheTime("test", testTime)

	time.Sleep(2 * time.Second)

	// check the cache time is synced to the configMap
	err = runtimeClient.Get(initCtx, client.ObjectKeyFromObject(cm), cm)
	assert.Nil(t, err)
	cachedTimeStr := cm.Data["test"]
	cachedTime, err := time.Parse(CACHE_TIME_FORMAT, cachedTimeStr)
	assert.Nil(t, err)
	fmt.Println("cached time", cachedTime)
	assert.True(t, cachedTime.Equal(testTime))
	initCancel()

	// reload the time from configMap
	RegisterTimeFilter("test")
	reloadCtx, reloadCancel := context.WithCancel(context.Background())
	defer reloadCancel()

	// update the configmap
	updateTime := testTime.Add(5 * time.Second)
	cm.Data["test"] = updateTime.Format(CACHE_TIME_FORMAT)
	err = runtimeClient.Update(reloadCtx, cm)
	assert.Nil(t, err)

	err = LaunchTimeFilter(reloadCtx, runtimeClient, "default")
	assert.Nil(t, err)

	err = runtimeClient.Get(reloadCtx, client.ObjectKeyFromObject(cm), cm)
	assert.Nil(t, err)
	cachedTimeStr = cm.Data["test"]
	cachedTime, err = time.Parse(CACHE_TIME_FORMAT, cachedTimeStr)
	assert.Nil(t, err)
	fmt.Println("updated time1", cachedTime)
	assert.True(t, cachedTime.Equal(updateTime))

	// update the cache with a expired time, verify the cached time isn't changed
	CacheTime("test", updateTime.Add(-10*time.Second))
	time.Sleep(2 * time.Second)

	err = runtimeClient.Get(reloadCtx, client.ObjectKeyFromObject(cm), cm)
	assert.Nil(t, err)
	cachedTimeStr = cm.Data["test"]
	cachedTime, err = time.Parse(CACHE_TIME_FORMAT, cachedTimeStr)
	assert.Nil(t, err)
	fmt.Println("updated time2", cachedTime)
	assert.True(t, cachedTime.Equal(updateTime))
}
