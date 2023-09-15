package status

import (
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

// ClusterPolicyHistoryEventBundle the base struct for cluster policy history event bundle.
type ClusterPolicyEventBundle struct {
	LeafHubName        string                                         `json:"leafHubName"`
	PolicyStatusEvents map[string]([]*models.LocalClusterPolicyEvent) `json:"policyStatusEvents"`
	BundleVersion      *BundleVersion                                 `json:"bundleVersion"`
}

// for manager
func NewClusterPolicyEventBundle() Bundle {
	return &ClusterPolicyEventBundle{
		PolicyStatusEvents: make(map[string]([]*models.LocalClusterPolicyEvent)),
	}
}

func (bundle *ClusterPolicyEventBundle) GetLeafHubName() string {
	return bundle.LeafHubName
}

func (bundle *ClusterPolicyEventBundle) GetObjects() []interface{} {
	objects := make([]interface{}, 0)
	for _, events := range bundle.PolicyStatusEvents {
		for _, event := range events {
			objects = append(objects, event)
		}
	}
	return objects
}

func (bundle *ClusterPolicyEventBundle) GetVersion() *BundleVersion {
	return bundle.BundleVersion
}

func (bundle *ClusterPolicyEventBundle) SetVersion(version *BundleVersion) {
	bundle.BundleVersion = version
}
