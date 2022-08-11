package conflator

import (
	"errors"
	"fmt"

	"github.com/stolostron/multicluster-globalhub/manager/pkg/statussyncer/transport2db/bundle"
	"github.com/stolostron/multicluster-globalhub/manager/pkg/statussyncer/transport2db/helpers"
	"github.com/stolostron/multicluster-globalhub/manager/pkg/statussyncer/transport2db/transport"
	"github.com/stolostron/multicluster-globalhub/pkg/bundle/status"
)

var errWrongBundleType = errors.New("received wrong bundle type, expecting DeltaStateBundle")

// newDeltaStateBundleInfo returns a new deltaStateBundleInfo instance.
func newDeltaStateBundleInfo() bundleInfo {
	return &deltaStateBundleInfo{
		bundle:   nil,
		metadata: nil,
		lastDispatchedDeltaBundleData: recoverableDeltaStateBundleData{
			bundle:                         nil,
			lowestPendingTransportMetadata: nil,
		},
		lastReceivedTransportMetadata: nil,
		deltaLineHeadBundleVersion:    nil,
	}
}

// deltaStateBundleInfo wraps delta-state bundles and their metadata.
//
// Definitions:
//
// - Delta Line: refers to all the deltas between two (complete-state) base bundles. That is all the delta-bundles
// that share the same BaseBundleVersion field value.
//
// - Delta Pack: the collection of delta-bundles whose content was merged in the bundle pointed to at the time. When a
// delta-bundle (that represents a delta-pack) is dispatched, the bundle pointer is reset so that we may start a new
// delta pack.
type deltaStateBundleInfo struct {
	bundle   bundle.DeltaStateBundle
	metadata *BundleMetadata

	lastDispatchedDeltaBundleData recoverableDeltaStateBundleData
	lastReceivedTransportMetadata transport.BundleMetadata
	deltaLineHeadBundleVersion    *status.BundleVersion
}

type recoverableDeltaStateBundleData struct {
	bundle                         bundle.DeltaStateBundle
	lowestPendingTransportMetadata transport.BundleMetadata
}

// getBundle returns the wrapped bundle.
func (bi *deltaStateBundleInfo) getBundle() bundle.Bundle {
	return bi.bundle
}

// getMetadata returns the metadata to be forwarded to processors.
func (bi *deltaStateBundleInfo) getMetadata() *BundleMetadata {
	// save the dispatched bundle content before giving metadata, so that we can start a new pack and recover
	// from failure if it happens
	bi.lastDispatchedDeltaBundleData.bundle = bi.bundle
	// lastDispatchedDeltaBundleData's transportMetadata will be used later in case the processing fails, therefore
	// it should point to the delta-pack's earliest pending contributor's (current transport metadata)
	bi.lastDispatchedDeltaBundleData.lowestPendingTransportMetadata = bi.metadata.transportBundleMetadata
	// the bundle's transport metadata will be used to mark as processed, therefore we should give out that of the
	// latest pending contributing delta-bundle's
	bi.metadata.transportBundleMetadata = bi.lastReceivedTransportMetadata
	// reset bundle only when dispatching, to start a new delta-pack
	bi.bundle = nil

	return bi.metadata
}

// update function to update the bundle and its metadata according to delta-state sync-mode.
func (bi *deltaStateBundleInfo) update(newBundle bundle.Bundle, transportMetadata transport.BundleMetadata,
	overwriteMetadataObject bool,
) error {
	newDeltaBundle, ok := newBundle.(bundle.DeltaStateBundle)
	if !ok {
		return fmt.Errorf("%w - received type %s", errWrongBundleType, helpers.GetBundleType(newBundle))
	}

	bundleStartsNewLine := bi.bundle == nil ||
		newDeltaBundle.GetDependencyVersion().NewerThan(bi.bundle.GetDependencyVersion())

	if err := bi.updateBundle(newDeltaBundle); err != nil {
		return fmt.Errorf("failed to update bundle - %w", err)
	}

	bi.updateMetadata(helpers.GetBundleType(newDeltaBundle), newDeltaBundle.GetVersion(), transportMetadata,
		overwriteMetadataObject)

	// update transport metadata only if bundle starts a new line of deltas
	if bundleStartsNewLine {
		// update current line-version info
		bi.deltaLineHeadBundleVersion = bi.metadata.bundleVersion
		bi.metadata.transportBundleMetadata = transportMetadata
	}

	return nil
}

// updateBundle updates the wrapped bundle and metadata according to the sync mode.
func (bi *deltaStateBundleInfo) updateBundle(newDeltaBundle bundle.DeltaStateBundle) error {
	// update content of newBundle with the currently held info, since a delta bundle contains events as opposed to
	// the full-state in CompleteState bundles.
	// note: if the currently held info is irrelevant to the newDeltaBundle, InheritEvents handles it.
	if err := newDeltaBundle.InheritEvents(bi.bundle); err != nil {
		return fmt.Errorf("failed to merge bundles - %w", err)
	}

	// update bundle
	bi.bundle = newDeltaBundle

	return nil
}

// updateMetadata updates the wrapped metadata according to the delta-state sync mode.
// createNewObjects boolean sets whether new (bundle/metadata) objects must be pointed to.
func (bi *deltaStateBundleInfo) updateMetadata(bundleType string, version *status.BundleVersion,
	transportMetadata transport.BundleMetadata, overwriteObject bool,
) {
	if bi.metadata == nil { // new metadata
		bi.metadata = &BundleMetadata{
			bundleType:              bundleType,
			transportBundleMetadata: transportMetadata,
		}
	} else if !overwriteObject {
		// create new metadata with identical info and plug it in
		bi.metadata = &BundleMetadata{
			bundleType:              bundleType,
			transportBundleMetadata: bi.metadata.transportBundleMetadata, // preserve metadata of the earliest
		}
	}
	// update version
	bi.metadata.bundleVersion = version
	// update latest received transport metadata for committing later if needed
	bi.lastReceivedTransportMetadata = transportMetadata
}

// handleFailure recovers from failure.
// The failed bundle's content is re-merged (not as source of truth) into the current active bundle,
// and the metadata is restored for safe committing (back to the first merged pending delta bundle's).
func (bi *deltaStateBundleInfo) handleFailure(failedMetadata *BundleMetadata) {
	lastDispatchedDeltaBundle := bi.lastDispatchedDeltaBundleData.bundle
	lastDispatchedTransportMetadata := bi.lastDispatchedDeltaBundleData.lowestPendingTransportMetadata
	// release currently saved data
	bi.lastDispatchedDeltaBundleData.bundle = nil
	bi.lastDispatchedDeltaBundleData.lowestPendingTransportMetadata = nil

	if bi.deltaLineHeadBundleVersion.NewerThan(failedMetadata.bundleVersion) {
		return // failed bundle's content is irrelevant since a covering baseline was received
	}

	if bi.bundle == nil {
		// did not receive updates, restore content
		bi.bundle = lastDispatchedDeltaBundle
	} else if err := bi.bundle.InheritEvents(lastDispatchedDeltaBundle); err != nil {
		// otherwise, the failed metadata is NOT from an older delta-line and its version is smaller than current-
		// version (since bi.bundle is not nil, therefore a bundle got in after the last dispatch for sure).
		// inherit content of the dispatched (failed) bundle, since content is flushed upon dispatching.
		//  -- the error should never happen but just for safety
		return
	}

	// restore transport metadata to that of the earliest contributor in the saved delta-pack
	bi.metadata.transportBundleMetadata = lastDispatchedTransportMetadata
}

// getTransportMetadataToCommit returns the wrapped bundle's transport metadata.
func (bi *deltaStateBundleInfo) getTransportMetadataToCommit() transport.BundleMetadata {
	if bi.metadata == nil {
		return nil
	}

	return bi.metadata.transportBundleMetadata
}

// markAsProcessed releases the bundle content and marks transport metadata as processed.
func (bi *deltaStateBundleInfo) markAsProcessed(metadata *BundleMetadata) {
	metadata.transportBundleMetadata.MarkAsProcessed()

	// release fail-recovery data
	bi.lastDispatchedDeltaBundleData.bundle = nil
	bi.lastDispatchedDeltaBundleData.lowestPendingTransportMetadata = nil
}
