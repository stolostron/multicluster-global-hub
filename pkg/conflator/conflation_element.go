package conflator

import (
	"fmt"

	statusbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator/dependency"
)

type conflationElement struct {
	bundleInfo                 bundleInfo
	handlerFunction            BundleHandlerFunc
	dependency                 *dependency.Dependency
	isInProcess                bool
	lastProcessedBundleVersion *statusbundle.BundleVersion
}

// update function that updates bundle and metadata and returns whether any error occurred.
func (element *conflationElement) update(bundle statusbundle.Bundle) error {
	// NOTICE - if the bundle is in process, we replace pointers and not override the values inside the pointers for
	// not changing bundles/metadata that were already given to DB workers for processing.
	if err := element.bundleInfo.update(bundle); err != nil {
		return fmt.Errorf("failed to update bundle - %w", err)
	}

	return nil
}

// getBundleForProcessing function to return Bundle and BundleMetadata to forward to processors.
// At the end of this call, the bundle may be released (set to nil).
func (element *conflationElement) getBundleForProcessing() (statusbundle.Bundle, *BundleMetadata) {
	// getBundle must be called before getMetadata since getMetadata assumes that the bundle is being forwarded
	// to processors, therefore it may release the bundle (set to nil) and apply other dispatch-related functionality.
	return element.bundleInfo.getBundle(), element.bundleInfo.getMetadata()
}
