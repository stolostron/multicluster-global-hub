package grc

type CompleteCompliance struct {
	PolicyID                  string   `json:"policyId"`
	NamespacedName            string   `json:"policy_namespaced_name"` // need it to delete obj from bundle for local resources.
	NonCompliantClusters      []string `json:"nonCompliantClusters"`
	UnknownComplianceClusters []string `json:"unknownComplianceClusters"`
	PendingComplianceClusters []string `json:"pendingComplianceClusters"`
}

type CompleteComplianceBundle []CompleteCompliance
