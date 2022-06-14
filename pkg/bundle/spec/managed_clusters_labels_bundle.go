package spec

import "time"

// ManagedClusterLabelsSpec struct holds information for managed cluster labels.
type ManagedClusterLabelsSpec struct {
	ClusterName      string            `json:"clusterName"`
	Labels           map[string]string `json:"labels"`
	DeletedLabelKeys []string          `json:"deletedLabelKeys"`
	UpdateTimestamp  time.Time         `json:"updateTimestamp"`
	Version          int64             `json:"version"`
}

// ManagedClusterLabelsSpecBundle struct bundles ManagedClusterLabelsSpec objects.
type ManagedClusterLabelsSpecBundle struct {
	Objects     []*ManagedClusterLabelsSpec `json:"objects"`
	LeafHubName string                      `json:"leafHubName"`
}
