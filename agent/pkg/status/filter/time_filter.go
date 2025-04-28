package filter

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

const (
	CACHE_CONFIG_NAME = "multicluster-global-hub-agent-sync-state"
	CACHE_TIME_FORMAT = "2006-01-02 15:04:05.000000 -0700 MST m=+0.000000000"
)

var (
	topicName = ""
	// cache time in the runtime
	eventTimeCache = make(map[string]time.Time)
	// the cache to be persist into configMap to deduplicate messages
	lastEventTimeCache = make(map[string]time.Time)
	// the interval to update the the runtime cache in the the configMap(by lastEventTimeCache)
	CacheSyncInterval = 5 * time.Second
	DeltaDuration     = 3 * time.Second
	log               = logger.DefaultZapLogger()
)

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

	// add noise to the time filter to ensure that events occurring very close together in time are not discarded
	older := old.Add(-DeltaDuration)
	return val.After(older)
}

// LaunchTimeFilter start a goroutine periodically sync the time filter cache to configMap
// and also init the event time cache with configmap
func LaunchTimeFilter(ctx context.Context, c client.Client, namespace string, topic string) error {
	topicName = topic
	agentStateConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CACHE_CONFIG_NAME,
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
		err = loadEventTimeCacheFromConfigMap(agentStateConfigMap, key)
		if err != nil {
			return err
		}
	}

	go func() {
		ticker := time.NewTicker(CacheSyncInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Info("cancel context")
				return
			case <-ticker.C:
				err := periodicSync(ctx, c, namespace)
				if err != nil {
					log.Errorf("failed to sync the configmap %v", err)
				}
			}
		}
	}()

	return nil
}

func periodicSync(ctx context.Context, c client.Client, namespace string) error {
	// update the lastSentCache
	update := false
	for key, currentTime := range eventTimeCache {
		lastTime, found := lastEventTimeCache[key]
		if !found {
			update = true
			lastEventTimeCache[key] = currentTime
		}

		if lastTime.Before(currentTime) {
			update = true
			lastEventTimeCache[key] = currentTime
		}
	}

	// sync the lastSentCache to ConfigMap
	if update {
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: CACHE_CONFIG_NAME, Namespace: namespace}}
		err := c.Get(ctx, client.ObjectKeyFromObject(cm), cm)
		if err != nil {
			return err
		}
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		for key, val := range lastEventTimeCache {
			cm.Data[cacheKey(key)] = val.Format(CACHE_TIME_FORMAT)
		}
		err = c.Update(ctx, cm, &client.UpdateOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

// RegisterTimeFilter call before the LaunchTimeFilter, it will get the init time from the configMap
func RegisterTimeFilter(key string) {
	eventTimeCache[key] = time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC)
	lastEventTimeCache[key] = time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC)
}

func loadEventTimeCacheFromConfigMap(cm *corev1.ConfigMap, key string) error {
	val, found := cm.Data[cacheKey(key)]
	if !found {
		log.Infow("the time cache isn't found in the ConfigMap", "key", key, "configMap", cm.Name)
		return nil
	}

	timeVal, err := time.Parse(CACHE_TIME_FORMAT, val)
	if err != nil {
		return err
	}
	eventTimeCache[key] = timeVal
	return nil
}

// cacheKey is to add the topic prefix for the origin key, so if the topic is changed, it won't filter the the event
func cacheKey(key string) string {
	return fmt.Sprintf("%s--%s", topicName, key)
}
