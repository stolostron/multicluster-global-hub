package filter

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	CacheConfigMapName = "multicluster-global-hub-agent-sync-state"
	CacheInterval      = 5 * time.Second
	CacheTimeFormat    = "2006-01-02 15:04:05.000"
)

var (
	eventTimeCache     = make(map[string]time.Time)
	lastEventTimeCache = make(map[string]time.Time)
)

type timeFilter struct {
	lastSentTime map[string]time.Time
}

// CacheTime cache the latest time
func CacheTime(key string, new time.Time) {
	old, ok := eventTimeCache[key]
	if !ok || old.Before(new) {
		eventTimeCache[key] = new
	}
}

// Newer compares the val time with cached the time, if not exist, then return true
func Newer(key string, val time.Time) bool {
	old, ok := eventTimeCache[key]
	if !ok {
		return true
	}
	return val.After(old)
}

// LaunchTimeFilter start a goroutine periodically sync the time filter cache to configMap
// and also init the event time cache with configmap
func LaunchTimeFilter(ctx context.Context, c client.Client, namespace string) error {
	agentStateConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CacheConfigMapName,
			Namespace: namespace,
		},
	}
	err := c.Get(ctx, client.ObjectKeyFromObject(agentStateConfigMap), agentStateConfigMap)
	if err != nil && errors.IsNotFound(err) {
		if e := c.Create(ctx, agentStateConfigMap); e != nil {
			return e
		}
	} else if err != nil {
		return err
	}

	for key := range lastEventTimeCache {
		err = loadTimeCacheFromConfigMap(agentStateConfigMap, key)
		if err != nil {
			return err
		}
	}

	// TODO: start a goroutine to sync the event sync state periodically, update the lastEventTimeCache

	return nil
}

// RegisterTimeFilter call before the LaunchTimeFilter, it will get the init time from the configMap
func RegisterTimeFilter(key string) {
	eventTimeCache[key] = time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC)
	lastEventTimeCache[key] = time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC)
}

func loadTimeCacheFromConfigMap(cm *corev1.ConfigMap, key string) error {
	val, found := cm.Data[key]
	if !found {
		klog.Info("the time cache isn't found in the ConfigMap", "key", key, "configMap", cm.Name)
		return nil
	}

	timeVal, err := time.Parse(CacheTimeFormat, val)
	if err != nil {
		return err
	}
	eventTimeCache[key] = timeVal
	return nil
}
