package conflator

import (
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/helpers"
	statusbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
)

// newCompleteStateBundleInfo returns a new completeStateBundleInfo instance.
func newCompleteStateBundleInfo() bundleInfo {
	return &completeStateBundleInfo{
		bundle:   nil,
		metadata: nil,
	}
}

// completeStateBundleInfo wraps complete-state bundles and their metadata.
type completeStateBundleInfo struct {
	bundle   statusbundle.Bundle
	metadata *BundleMetadata
}

// getBundle returns the wrapped bundle.
func (bi *completeStateBundleInfo) getBundle() statusbundle.Bundle {
	return bi.bundle
}

// getMetadata returns the metadata to be forwarded to processors.
func (bi *completeStateBundleInfo) getMetadata() *BundleMetadata {
	return bi.metadata
}

// update function to update the bundle and its metadata according to complete-state sync-mode.
func (bi *completeStateBundleInfo) update(bundle statusbundle.Bundle, transportMetadata bundle.BundleMetadata,
	overwriteMetadataObject bool,
) error {
	bi.bundle = bundle

	if bi.metadata == nil || !overwriteMetadataObject {
		bi.metadata = &BundleMetadata{
			bundleType:              helpers.GetBundleType(bundle),
			bundleVersion:           bundle.GetVersion(),
			transportBundleMetadata: transportMetadata,
		}

		return nil
	}

	// update metadata
	bi.metadata.bundleVersion = bundle.GetVersion()
	bi.metadata.transportBundleMetadata = transportMetadata

	return nil
}

// getTransportMetadataToCommit returns the wrapped bundle's transport metadata.
func (bi *completeStateBundleInfo) getTransportMetadataToCommit() bundle.BundleMetadata {
	if bi.metadata == nil {
		return nil
	}

	return bi.metadata.transportBundleMetadata
}

// markAsProcessed releases the bundle content and marks transport metadata as processed.
func (bi *completeStateBundleInfo) markAsProcessed(metadata *BundleMetadata) {
	metadata.transportBundleMetadata.MarkAsProcessed()

	if !metadata.bundleVersion.Equals(bi.metadata.bundleVersion) {
		return
	}
	// if this is the same bundle that was processed then release bundle pointer, otherwise leave
	// the current (newer one) as pending.
	bi.bundle = nil
}
