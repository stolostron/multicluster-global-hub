package status

import policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

// MinimalPolicyComplianceStatus holds information for minimal policy compliance status.
type MinimalPolicyComplianceStatus struct {
	PolicyID             string                     `json:"policyId"`
	RemediationAction    policyv1.RemediationAction `json:"remediationAction"`
	NonCompliantClusters int                        `json:"nonCompliantClusters"`
	AppliedClusters      int                        `json:"appliedClusters"`
}

// BaseMinimalComplianceStatusBundle the base struct for minimal compliance status bundle.
type BaseMinimalComplianceStatusBundle struct {
	Objects       []*MinimalPolicyComplianceStatus `json:"objects"`
	LeafHubName   string                           `json:"leafHubName"`
	BundleVersion *BundleVersion                   `json:"bundleVersion"`
}
