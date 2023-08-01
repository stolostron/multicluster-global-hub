package status

import (
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

// ClusterPolicyHistoryEventBundle the base struct for cluster policy history event bundle.
type BaseClusterPolicyStatusEventBundle struct {
	LeafHubName        string                                         `json:"leafHubName"`
	PolicyStatusEvents map[string]([]*models.LocalClusterPolicyEvent) `json:"policyStatusEvents"`
	BundleVersion      *BundleVersion                                 `json:"bundleVersion"`
}

// for manager
func NewClusterPolicyStatusEventBundle() Bundle {
	return &BaseClusterPolicyStatusEventBundle{
		PolicyStatusEvents: make(map[string]([]*models.LocalClusterPolicyEvent)),
	}
}

func (bundle *BaseClusterPolicyStatusEventBundle) GetLeafHubName() string {
	return bundle.LeafHubName
}

func (bundle *BaseClusterPolicyStatusEventBundle) GetObjects() []interface{} {
	objects := make([]interface{}, 0)
	for _, events := range bundle.PolicyStatusEvents {
		for _, event := range events {
			objects = append(objects, event)
		}
	}
	return objects
}

func (bundle *BaseClusterPolicyStatusEventBundle) GetVersion() *BundleVersion {
	return bundle.BundleVersion
}

func (bundle *BaseClusterPolicyStatusEventBundle) SetVersion(version *BundleVersion) {
	bundle.BundleVersion = version
}
