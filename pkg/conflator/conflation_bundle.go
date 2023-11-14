package conflator

import (
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
)

// createBundleInfoFunc function that specifies how to create a bundle-info.
type createBundleInfoFunc func() conflationBundle

// ConflationBundleMetadata abstracts metadata of conflation elements inside the conflation units.
type ConflationBundleMetadata struct {
	bundleType    string
	bundleVersion *metadata.BundleVersion
	// bundleStatus is information we need for marking bundle as consumed in transport (e.g. commit offset)
	bundleStatus metadata.BundleStatus
}

// conflationBundle abstracts the information/functionality of the two types of bundles (complete/delta state bundles).
type conflationBundle interface {
	// getBundle returns the bundle.
	getBundle() bundle.ManagerBundle
	// getMetadata returns the metadata to forward to processors.
	getMetadata() *ConflationBundleMetadata
	// update function to update the bundle and its metadata according to sync-mode.
	update(bundle bundle.ManagerBundle, bundleStatus metadata.BundleStatus, overwriteMetadata bool) error
	// getBundleStatus returns the transport metadata for message committing purposes.
	getBundleStatus() metadata.BundleStatus
	// markAsProcessed marks the given metadata as processed.
	markAsProcessed(elementMetadata *ConflationBundleMetadata)
}

// deltaBundleAdapter extends BundleInfo with delta-bundle related functionalities.
type deltaBundleAdapter interface {
	conflationBundle
	// handleFailure handles bundle processing failure (data recovery if needed).
	handleFailure(failedMetadata *ConflationBundleMetadata)
}
