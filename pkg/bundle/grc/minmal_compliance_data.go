package grc

import (
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

// MinimalCompliance holds information for minimal policy compliance status.
type MinimalCompliance struct {
	PolicyID             string                     `json:"policyId"`
	NamespacedName       string                     `json:"-"` // need it to delete obj from bundle for local resources.
	RemediationAction    policyv1.RemediationAction `json:"remediationAction"`
	NonCompliantClusters int                        `json:"nonCompliantClusters"`
	AppliedClusters      int                        `json:"appliedClusters"`
}

type MinimalComplianceData []MinimalCompliance
