package conflator

import (
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	statusbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
)

// createBundleInfoFunc function that specifies how to create a bundle-info.
type createBundleInfoFunc func() bundleInfo

// bundleInfo abstracts the information/functionality of the two types of bundles (complete/delta state bundles).
type bundleInfo interface {
	// getBundle returns the bundle.
	getBundle() statusbundle.Bundle
	// getMetadata returns the metadata to forward to processors.
	getMetadata() *BundleMetadata
	// update function to update the bundle and its metadata according to sync-mode.
	update(bundle statusbundle.Bundle, metadata bundle.BundleMetadata, overwriteMetadataObject bool) error
	// getTransportMetadataToCommit returns the transport metadata for message committing purposes.
	getTransportMetadataToCommit() bundle.BundleMetadata
	// markAsProcessed marks the given metadata as processed.
	markAsProcessed(metadata *BundleMetadata)
}

// deltaBundleInfo extends BundleInfo with delta-bundle related functionalities.
type deltaBundleInfo interface {
	bundleInfo
	// handleFailure handles bundle processing failure (data recovery if needed).
	handleFailure(failedMetadata *BundleMetadata)
}
