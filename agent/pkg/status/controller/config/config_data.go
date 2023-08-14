package config

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

var (
	leafHubName   = "leafhub"
	syncIntervals = map[IntervalKey]time.Duration{
		ManagedClusterIntervalKey: 5 * time.Second,
		PolicyIntervalKey:         5 * time.Second,
		ControlInfoIntervalKey:    60 * time.Second,
	}
	agentConfigMap = &corev1.ConfigMap{}
)

type IntervalKey string

const (
	PolicyIntervalKey         IntervalKey = "policies"
	ManagedClusterIntervalKey IntervalKey = "managedClusters"
	ControlInfoIntervalKey    IntervalKey = "controlInfo"
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

// GetControlInfoDuration returns control info sync interval.
func GetControlInfoDuration() time.Duration {
	return syncIntervals[ControlInfoIntervalKey]
}

func GetLeafHubName() string {
	return leafHubName
}

func GetAgentConfigMap() *corev1.ConfigMap {
	return agentConfigMap
}
