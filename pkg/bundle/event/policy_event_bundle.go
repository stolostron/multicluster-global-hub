package event

type RootPolicyEvent struct {
	BaseEvent
	PolicyID   string `json:"policyId"`
	Compliance string `json:"compliance"`
}

type ReplicatedPolicyEvent struct {
	BaseEvent
	PolicyID    string `json:"policyId"`
	ClusterID   string `json:"clusterId"`
	ClusterName string `json:"clusterName"`
	Compliance  string `json:"compliance"`
}

type ReplicatedPolicyEventBundle []*ReplicatedPolicyEvent

type RootPolicyEventBundle []*RootPolicyEvent
