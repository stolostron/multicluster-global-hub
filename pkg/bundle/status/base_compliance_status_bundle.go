package status

// PolicyGenericComplianceStatus holds information for policy compliance status and may be used either for
// complete or delta state bundles.
type PolicyGenericComplianceStatus struct {
	PolicyID                  string   `json:"policyId"`
	CompliantClusters         []string `json:"compliantClusters"`
	NonCompliantClusters      []string `json:"nonCompliantClusters"`
	UnknownComplianceClusters []string `json:"unknownComplianceClusters"`
}

// PolicyCompleteComplianceStatus holds information for (optimized) policy compliance status.
type PolicyCompleteComplianceStatus struct {
	PolicyID                  string   `json:"policyId"`
	NonCompliantClusters      []string `json:"nonCompliantClusters"`
	UnknownComplianceClusters []string `json:"unknownComplianceClusters"`
}

// BaseClustersPerPolicyBundle the base struct for clusters per policy bundle and contains the full state.
type BaseClustersPerPolicyBundle struct {
	Objects       []*PolicyGenericComplianceStatus `json:"objects"`
	LeafHubName   string                           `json:"leafHubName"`
	BundleVersion *BundleVersion                   `json:"bundleVersion"`
}

// BaseCompleteComplianceStatusBundle the base struct for complete state compliance status bundle.
type BaseCompleteComplianceStatusBundle struct {
	Objects           []*PolicyCompleteComplianceStatus `json:"objects"`
	LeafHubName       string                            `json:"leafHubName"`
	BaseBundleVersion *BundleVersion                    `json:"baseBundleVersion"`
	BundleVersion     *BundleVersion                    `json:"bundleVersion"`
}

// BaseDeltaComplianceStatusBundle the base struct for delta state compliance status bundle.
type BaseDeltaComplianceStatusBundle struct {
	Objects           []*PolicyGenericComplianceStatus `json:"objects"`
	LeafHubName       string                           `json:"leafHubName"`
	BaseBundleVersion *BundleVersion                   `json:"baseBundleVersion"`
	BundleVersion     *BundleVersion                   `json:"bundleVersion"`
}
