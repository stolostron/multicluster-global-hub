package configmap

import (
	"time"
)

var (
	syncIntervals = map[AgentConfigKey]time.Duration{
		ManagedClusterIntervalKey:      5 * time.Second,
		PolicyIntervalKey:              5 * time.Second,
		HubClusterInfoIntervalKey:      60 * time.Second,
		HubClusterHeartBeatIntervalKey: 60 * time.Second,
		EventIntervalKey:               5 * time.Second,
	}

	resyncInterval = 6 * time.Hour
	syncInterval   = 5 * time.Second
	agentConfigs   = map[AgentConfigKey]AgentConfigValue{
		AgentAggregationKey:  AggregationFull,
		EnableLocalPolicyKey: EnableLocalPolicyTrue,
	}
)

type AgentConfigKey string

const (
	PolicyIntervalKey              AgentConfigKey = "policies"
	ManagedClusterIntervalKey      AgentConfigKey = "managedClusters"
	HubClusterInfoIntervalKey      AgentConfigKey = "hubClusterInfo"
	HubClusterHeartBeatIntervalKey AgentConfigKey = "hubClusterHeartbeat"
	EventIntervalKey               AgentConfigKey = "events"

	AgentAggregationKey  AgentConfigKey = "aggregationLevel"
	EnableLocalPolicyKey AgentConfigKey = "enableLocalPolicies"
	AgentLogLevelKey     AgentConfigKey = "logLevel"
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
	return syncIntervals[ManagedClusterIntervalKey]
}

// GetPolicyDuration returns policies sync interval.
func GetPolicyDuration() time.Duration {
	return syncIntervals[PolicyIntervalKey]
}

// GetHubClusterInfoDuration returns control info sync interval.
func GetHubClusterInfoDuration() time.Duration {
	return syncIntervals[HubClusterInfoIntervalKey]
}

func GetHeartbeatDuration() time.Duration {
	return syncIntervals[HubClusterHeartBeatIntervalKey]
}

func GetEventDuration() time.Duration {
	return syncIntervals[EventIntervalKey]
}

func GetAggregationLevel() AgentConfigValue {
	return agentConfigs[AgentAggregationKey]
}

func GetEnableLocalPolicy() AgentConfigValue {
	return agentConfigs[EnableLocalPolicyKey]
}

func GetResyncInterval() time.Duration {
	return resyncInterval
}

func GetSyncInterval() time.Duration {
	return syncInterval
}

func SetInterval(key AgentConfigKey, val time.Duration) {
	syncIntervals[key] = val
}

func SetResyncInterval(val time.Duration) {
	resyncInterval = val
}
