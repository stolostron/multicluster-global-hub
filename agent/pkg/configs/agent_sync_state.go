package configs

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// The ConfigMap is used to store the agent sync state
	//   1. include the event sync state for events filter: eventType - timestamp
	//   2. include the migration ID for migration status: migrationID - timestamp
	AGENT_SYNC_STATE_CONFIG_MAP_NAME = "multicluster-global-hub-agent-sync-state"
	// use RFC3339 format to store/format the time: https://datatracker.ietf.org/doc/html/rfc3339
	// it algin with the cloudevents time format: https://github.com/cloudevents/sdk-go/blob/main/v2/types/timestamp.go
	AGENT_SYNC_STATE_TIME_FORMAT_VALUE = time.RFC3339Nano
)

func GetSyncStateConfigMap(ctx context.Context, c client.Client) (*corev1.ConfigMap, error) {
	agentStateConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      AGENT_SYNC_STATE_CONFIG_MAP_NAME,
			Namespace: GetAgentConfig().PodNamespace,
		},
	}
	err := c.Get(ctx, client.ObjectKeyFromObject(agentStateConfigMap), agentStateConfigMap)
	if err != nil && errors.IsNotFound(err) {
		if e := c.Create(ctx, agentStateConfigMap); e != nil {
			return nil, e
		}
	} else if err != nil {
		return nil, err
	}
	return agentStateConfigMap, nil
}

// SetSyncTimeState cache the time to the ConfigMap, update the configmap with retry on conflict
func SetSyncTimeState(ctx context.Context, c client.Client, key string) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		agentStateConfigMap, err := GetSyncStateConfigMap(ctx, c)
		if err != nil {
			return err
		}
		if agentStateConfigMap.Data == nil {
			agentStateConfigMap.Data = make(map[string]string)
		}
		agentStateConfigMap.Data[key] = time.Now().Format(AGENT_SYNC_STATE_TIME_FORMAT_VALUE)
		return c.Update(ctx, agentStateConfigMap)
	})
}

// GetSyncTimeState get the time from the ConfigMap:
// if the time is not found, return false, time.Time{}, nil - the time is not cached
// if the time is found, return true, time.Time, nil -  the time is cached
// if the time is found, but the time is invalid, return true, time.Time{}, err - the time is invalid
func GetSyncTimeState(ctx context.Context, c client.Client, key string) (bool, time.Time, error) {
	agentStateConfigMap, err := GetSyncStateConfigMap(ctx, c)
	if err != nil {
		return false, time.Time{}, err
	}
	val, ok := agentStateConfigMap.Data[key]
	if !ok {
		return false, time.Time{}, nil
	}
	timeVal, err := time.Parse(AGENT_SYNC_STATE_TIME_FORMAT_VALUE, val)
	if err != nil {
		return true, time.Time{}, err
	}
	return true, timeVal, nil
}
