package event

type RootPolicyEvent struct {
	BaseEvent
	PolicyID   string `json:"policyId"`
	Compliance string `json:"compliance"`
}

type ReplicatedPolicyEvent struct {
	BaseEvent
	PolicyID   string `json:"policyId"`
	ClusterID  string `json:"clusterId"`
	Compliance string `json:"compliance"`
}
