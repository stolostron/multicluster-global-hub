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

// ManagedClusterLabelsSpecData struct bundles ManagedClusterLabelsSpec objects.
type ManagedClusterLabelsSpecData struct {
	Objects     []*ManagedClusterLabelsSpec `json:"objects"`
	LeafHubName string                      `json:"leafHubName"`
}
