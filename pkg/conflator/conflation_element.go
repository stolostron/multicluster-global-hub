package conflator

import (
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator/dependency"
)

type conflationElement struct {
	conflationBundle     conflationBundle
	handlerFunction      BundleHandlerFunc
	dependency           *dependency.Dependency
	isInProcess          bool
	lastProcessedVersion *metadata.BundleVersion
}

// update function that updates bundle and metadata and returns whether any error occurred.
func (element *conflationElement) update(b bundle.ManagerBundle, m metadata.BundleStatus) error {
	// NOTICE - if the bundle is in process, we replace pointers and not override the values inside the pointers for
	// not changing bundles/metadata that were already given to DB workers for processing.
	return element.conflationBundle.update(b, m, !element.isInProcess)
}

// getBundleForProcessing function to return Bundle and BundleMetadata to forward to processors.
// At the end of this call, the bundle may be released (set to nil).
func (element *conflationElement) getBundleForProcessing() (bundle.ManagerBundle, *ConflationBundleMetadata) {
	// getBundle must be called before getMetadata since getMetadata assumes that the bundle is being forwarded
	// to processors, therefore it may release the bundle (set to nil) and apply other dispatch-related functionality.
	return element.conflationBundle.getBundle(), element.conflationBundle.getMetadata()
}
