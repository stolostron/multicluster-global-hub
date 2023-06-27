package status

// ClusterPolicyHistoryEventBundle the base struct for cluster policy history event bundle.
type BaseClusterPolicyHistoryEventBundle struct {
	LeafHubName        string                        `json:"leafHubName"`
	PolicyStatusEvents map[string]*PolicyStatusEvent `json:"policyStatusEvents"`
	BundleVersion      *BundleVersion                `json:"bundleVersion"`
}

type PolicyStatusEvent struct {
	EventName     string `json:"eventName"`
	ClusterID     string `json:"clusterId"`
	Compliance    string `json:"compliance"`
	LastTimeStamp string `json:"lastTimeStamp"`
	Message       string `json:"message"`
	Count         int    `json:"count"`
}
