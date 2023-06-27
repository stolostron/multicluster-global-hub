package status

// ClusterPolicyHistoryEventBundle the base struct for cluster policy history event bundle.
type ClusterPolicyHistoryEventBundle struct {
	PolicyID      string               `json:"policyId"`
	LeafHubName   string               `json:"leafHubName"`
	Objects       []*PolicyStatusEvent `json:"objects"`
	BundleVersion *BundleVersion       `json:"bundleVersion"`
}

type PolicyStatusEvent struct {
	EventName     string `json:"eventName"`
	ClusterID     string `json:"clusterId"`
	Compliance    string `json:"compliance"`
	LastTimeStamp string `json:"lastTimeStamp"`
	Message       string `json:"message"`
	Count         int    `json:"count"`
}
