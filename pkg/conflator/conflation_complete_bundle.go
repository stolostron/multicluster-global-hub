package conflator

import (
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
)

// completeConflationBundle wraps complete-state bundles and their metadata.
type completeConflationBundle struct {
	bundle          bundle.ManagerBundle
	elementMetadata *ConflationBundleMetadata
}

// newCompleteConflationBundle returns a new completeStateBundleInfo instance.
func newCompleteConflationBundle() conflationBundle {
	return &completeConflationBundle{
		bundle:          nil,
		elementMetadata: nil,
	}
}

// getBundle returns the wrapped bundle.
func (bi *completeConflationBundle) getBundle() bundle.ManagerBundle {
	return bi.bundle
}

// getMetadata returns the metadata to be forwarded to processors.
func (bi *completeConflationBundle) getMetadata() *ConflationBundleMetadata {
	return bi.elementMetadata
}

// update function to update the bundle and its metadata according to complete-state sync-mode.
func (bi *completeConflationBundle) update(b bundle.ManagerBundle, bundleStatus metadata.BundleStatus,
	overwriteMetadata bool,
) error {
	bi.bundle = b

	if bi.elementMetadata == nil || !overwriteMetadata {
		bi.elementMetadata = &ConflationBundleMetadata{
			bundleType:    bundle.GetBundleType(b),
			bundleVersion: b.GetVersion(),
			bundleStatus:  bundleStatus,
		}

		return nil
	}

	// update metadata
	bi.elementMetadata.bundleVersion = b.GetVersion()
	bi.elementMetadata.bundleStatus = bundleStatus

	return nil
}

// getBundleStatus returns the wrapped bundle's transport metadata.
func (bi *completeConflationBundle) getBundleStatus() metadata.BundleStatus {
	if bi.elementMetadata == nil {
		return nil
	}

	return bi.elementMetadata.bundleStatus
}

// markAsProcessed releases the bundle content and marks transport metadata as processed.
func (bi *completeConflationBundle) markAsProcessed(metadata *ConflationBundleMetadata) {
	metadata.bundleStatus.MarkAsProcessed()

	if !metadata.bundleVersion.Equals(bi.elementMetadata.bundleVersion) {
		return
	}
	// if this is the same bundle that was processed then release bundle pointer, otherwise leave
	// the current (newer one) as pending.
	bi.bundle = nil
}
