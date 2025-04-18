package grc

type Compliance struct {
	PolicyID                  string   `json:"policyId"`
	NamespacedName            string   `json:"policy_namespaced_name"` // need it to delete obj from bundle for these without finalizer.
	CompliantClusters         []string `json:"compliantClusters"`
	NonCompliantClusters      []string `json:"nonCompliantClusters"`
	UnknownComplianceClusters []string `json:"unknownComplianceClusters"`
	PendingComplianceClusters []string `json:"pendingComplianceClusters"`
}

type ComplianceBundle []Compliance

type DeltaComplianceBundle []Compliance
