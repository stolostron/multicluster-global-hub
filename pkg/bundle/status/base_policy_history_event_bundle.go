package status

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ClusterPolicyHistoryEventBundle the base struct for cluster policy history event bundle.
type BaseClusterPolicyStatusEventBundle struct {
	LeafHubName        string                            `json:"leafHubName"`
	PolicyStatusEvents map[string]([]*PolicyStatusEvent) `json:"policyStatusEvents"`
	BundleVersion      *BundleVersion                    `json:"bundleVersion"`
}

type PolicyStatusEvent struct {
	EventName     string      `json:"eventName"`
	ClusterID     string      `json:"clusterId"`
	PolicyID      string      `json:"policyId"` // this is the root policy id
	Compliance    string      `json:"compliance"`
	LastTimestamp metav1.Time `json:"lastTimestamp,omitempty" protobuf:"bytes,7,opt,name=lastTimestamp"`
	Message       string      `json:"message"`
	Count         int         `json:"count"`
}

// for manager
func NewClusterPolicyStatusEventBundle() Bundle {
	return &BaseClusterPolicyStatusEventBundle{
		PolicyStatusEvents: make(map[string]([]*PolicyStatusEvent)),
	}
}

func (bundle *BaseClusterPolicyStatusEventBundle) GetLeafHubName() string {
	return bundle.LeafHubName
}

func (bundle *BaseClusterPolicyStatusEventBundle) GetObjects() []interface{} {
	objects := make([]interface{}, 0)
	i := 0
	for _, events := range bundle.PolicyStatusEvents {
		for _, event := range events {
			objects[i] = event
			i++
		}
	}
	return objects
}

func (bundle *BaseClusterPolicyStatusEventBundle) GetVersion() *BundleVersion {
	return bundle.BundleVersion
}
