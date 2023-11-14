package base

import (
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
)

// GenericCompliance holds information for policy compliance status and may be used either for
// complete or delta state bundles.
type GenericCompliance struct {
	PolicyID                  string   `json:"policyId"`
	NamespacedName            string   `json:"-"` // need it to delete obj from bundle for these without finalizer.
	CompliantClusters         []string `json:"compliantClusters"`
	NonCompliantClusters      []string `json:"nonCompliantClusters"`
	UnknownComplianceClusters []string `json:"unknownComplianceClusters"`
}

// BaseComplianceBundle the base struct for clusters per policy bundle and contains the full state.
type BaseComplianceBundle struct {
	Objects       []*GenericCompliance    `json:"objects"`
	LeafHubName   string                  `json:"leafHubName"`
	BundleVersion *metadata.BundleVersion `json:"bundleVersion"`
}

// GenericCompleteCompliance holds information for (optimized) policy compliance status.
type GenericCompleteCompliance struct {
	PolicyID                  string   `json:"policyId"`
	NamespacedName            string   `json:"-"` // need it to delete obj from bundle for local resources.
	NonCompliantClusters      []string `json:"nonCompliantClusters"`
	UnknownComplianceClusters []string `json:"unknownComplianceClusters"`
}

// BaseCompleteComplianceBundle the base struct for complete state compliance status bundle.
type BaseCompleteComplianceBundle struct {
	Objects           []*GenericCompleteCompliance `json:"objects"`
	LeafHubName       string                       `json:"leafHubName"`
	BaseBundleVersion *metadata.BundleVersion      `json:"baseBundleVersion"`
	BundleVersion     *metadata.BundleVersion      `json:"bundleVersion"`
}

// BaseDeltaComplianceBundle the base struct for delta state compliance status bundle.
type BaseDeltaComplianceBundle struct {
	Objects           []*GenericCompliance    `json:"objects"`
	LeafHubName       string                  `json:"leafHubName"`
	BaseBundleVersion *metadata.BundleVersion `json:"baseBundleVersion"`
	BundleVersion     *metadata.BundleVersion `json:"bundleVersion"`
}
