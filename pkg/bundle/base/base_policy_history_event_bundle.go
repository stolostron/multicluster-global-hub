package base

import (
	"time"

	"gorm.io/datatypes"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
)

type PolicyHistoryEvent struct {
	ClusterID  string         `json:"clusterId"`
	PolicyID   string         `json:"policyId"`
	Compliance string         `json:"compliance"`
	EventName  string         `json:"eventName"`
	Message    string         `json:"message"`
	Reason     string         `json:"reason"`
	Count      int            `json:"count"`
	Source     datatypes.JSON `json:"source"`
	CreatedAt  time.Time      `json:"createdAt"`
}

// BasePolicyHistoryEventBundle the base struct for cluster policy history event bundle.
type BasePolicyHistoryEventBundle struct {
	LeafHubName          string                             `json:"leafHubName"`
	ReplicasPolicyEvents map[string]([]*PolicyHistoryEvent) `json:"policyStatusEvents"`
	BundleVersion        *metadata.BundleVersion            `json:"bundleVersion"`
}
