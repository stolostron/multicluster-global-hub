package configmap

import (
	"fmt"
	"strings"
	"time"

	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

var (
	// Default sync and resync intervals for various event types.
	// The ConfigMap keys use the event type suffix, e.g., "managedcluster", "localpolicyspec", etc.
	defaultSyncInterval = 5 * time.Second
	syncIntervals       = map[string]time.Duration{
		GetSyncKey(enum.ManagedClusterType):      defaultSyncInterval,
		GetSyncKey(enum.LocalPolicySpecType):     defaultSyncInterval,
		GetSyncKey(enum.HubClusterInfoType):      60 * time.Second,
		GetSyncKey(enum.HubClusterHeartbeatType): 60 * time.Second,
		GetSyncKey(enum.ManagedClusterEventType): defaultSyncInterval,
	}

	// Default resync intervals for various event types.
	// Each key in the ConfigMap is formed as "resync.<eventType>", <eventType> is suffix of EventType
	// e.g., "resync.managedcluster", "resync.localpolicyspec", etc.
	defaultResyncInterval = 6 * time.Hour
	reSyncIntervals       = map[string]time.Duration{
		GetResyncKey(enum.ManagedClusterType):  defaultResyncInterval,
		GetResyncKey(enum.LocalPolicySpecType): defaultResyncInterval,
	}

	agentConfigs = map[string]AgentConfigValue{
		AgentAggregationKey:  AggregationFull,
		EnableLocalPolicyKey: EnableLocalPolicyTrue,
	}
)

const (
	AgentAggregationKey  = "aggregationLevel"
	EnableLocalPolicyKey = "enableLocalPolicies"
	AgentLogLevelKey     = "logLevel"
)

type AgentConfigValue string

const (
	AggregationFull        AgentConfigValue = "full"
	AggregationMinimal     AgentConfigValue = "minimal"
	EnableLocalPolicyTrue  AgentConfigValue = "true"
	EnableLocalPolicyFalse AgentConfigValue = "false"
)

// ResolveSyncIntervalFunc is a function for resolving corresponding sync interval from SyncIntervals data structure.
type ResolveSyncIntervalFunc func() time.Duration

// GetManagerClusterDuration returns managed clusters sync interval.
func GetManagerClusterDuration() time.Duration {
	return syncIntervals[enum.ShortenEventType(string(enum.ManagedClusterType))]
}

// GetPolicyDuration returns policies sync interval.
func GetPolicyDuration() time.Duration {
	return syncIntervals[enum.ShortenEventType(string(enum.LocalPolicySpecType))]
}

// GetHubClusterInfoDuration returns control info sync interval.
func GetHubClusterInfoDuration() time.Duration {
	return syncIntervals[enum.ShortenEventType(string(enum.HubClusterInfoType))]
}

func GetHeartbeatDuration() time.Duration {
	return syncIntervals[enum.ShortenEventType(string(enum.HubClusterHeartbeatType))]
}

func GetEventDuration() time.Duration {
	return syncIntervals[enum.ShortenEventType(string(enum.ManagedClusterEventType))]
}

func GetAggregationLevel() AgentConfigValue {
	return agentConfigs[AgentAggregationKey]
}

func GetEnableLocalPolicy() AgentConfigValue {
	return agentConfigs[EnableLocalPolicyKey]
}

func GetResyncInterval(eventType enum.EventType) time.Duration {
	interval, ok := reSyncIntervals[GetResyncKey(eventType)]
	if !ok {
		return defaultResyncInterval
	}
	return interval
}

func GetSyncInterval(eventType enum.EventType) time.Duration {
	interval, ok := syncIntervals[GetSyncKey(eventType)]
	if !ok {
		return defaultSyncInterval
	}
	return interval
}

// SetInterval sets the sync/resync interval for a specific bundle.
// The key is derived from the EventType, e.g., GetSyncKey(enum.ManagedClusterEventType).
func SetInterval(key string, val time.Duration) {
	if strings.HasPrefix(key, "resync.") {
		reSyncIntervals[key] = val
	} else {
		syncIntervals[key] = val
	}
}

func GetResyncKey(eventType enum.EventType) string {
	return fmt.Sprintf("resync.%s", enum.ShortenEventType(string(eventType)))
}

func GetSyncKey(eventType enum.EventType) string {
	return enum.ShortenEventType(string(eventType))
}