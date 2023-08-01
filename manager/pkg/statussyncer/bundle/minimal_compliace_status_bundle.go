package bundle

import (
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
)

// NewMinimalComplianceStatusBundle creates a new instance of MinimalComplianceStatusBundle.
func NewMinimalComplianceStatusBundle() status.Bundle {
	return &MinimalComplianceStatusBundle{}
}

// MinimalComplianceStatusBundle abstracts management of minimal compliance status bundle.
type MinimalComplianceStatusBundle struct {
	status.BaseMinimalComplianceStatusBundle
}

// GetLeafHubName returns the leaf hub name that sent the bundle.
func (bundle *MinimalComplianceStatusBundle) GetLeafHubName() string {
	return bundle.LeafHubName
}

// GetObjects returns the objects in the bundle.
func (bundle *MinimalComplianceStatusBundle) GetObjects() []interface{} {
	result := make([]interface{}, len(bundle.Objects))
	for i, obj := range bundle.Objects {
		result[i] = obj
	}

	return result
}

// GetVersion returns the bundle version.
func (bundle *MinimalComplianceStatusBundle) GetVersion() *status.BundleVersion {
	return bundle.BundleVersion
}

func (bundle *MinimalComplianceStatusBundle) SetVersion(version *status.BundleVersion) {
	bundle.BundleVersion = version
}
