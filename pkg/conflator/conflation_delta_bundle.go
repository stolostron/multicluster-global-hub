package conflator

import (
	"errors"
	"fmt"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
)

var errWrongBundleType = errors.New("received wrong bundle type, expecting DeltaStateBundle")

// newDeltaConflationBundle returns a new deltaStateBundleInfo instance.
func newDeltaConflationBundle() conflationBundle {
	return &deltaConflationBundle{
		bundle:            nil,
		transportMetadata: nil,
		lastDispatchedDeltaBundleData: recoverableDeltaStateBundleData{
			bundle:                         nil,
			lowestPendingTransportMetadata: nil,
		},
		lastReceivedTransportMetadata: nil,
		deltaLineHeadBundleVersion:    nil,
	}
}

// deltaConflationBundle wraps delta-state bundles and their metadata.
//
// Definitions:
//
// - Delta Line: refers to all the deltas between two (complete-state) base bundles. That is all the delta-bundles
// that share the same BaseBundleVersion field value.
//
// - Delta Pack: the collection of delta-bundles whose content was merged in the bundle pointed to at the time. When a
// delta-bundle (that represents a delta-pack) is dispatched, the bundle pointer is reset so that we may start a new
// delta pack.
type deltaConflationBundle struct {
	bundle            bundle.ManagerDeltaBundle
	transportMetadata *ConflationBundleMetadata

	lastDispatchedDeltaBundleData recoverableDeltaStateBundleData
	lastReceivedTransportMetadata metadata.BundleStatus
	deltaLineHeadBundleVersion    *metadata.BundleVersion
}

type recoverableDeltaStateBundleData struct {
	bundle                         bundle.ManagerDeltaBundle
	lowestPendingTransportMetadata metadata.BundleStatus
}

// getBundle returns the wrapped bundle.
func (bi *deltaConflationBundle) getBundle() bundle.ManagerBundle {
	return bi.bundle
}

// getMetadata returns the metadata to be forwarded to processors.
func (bi *deltaConflationBundle) getMetadata() *ConflationBundleMetadata {
	// save the dispatched bundle content before giving metadata, so that we can start a new pack and recover
	// from failure if it happens
	bi.lastDispatchedDeltaBundleData.bundle = bi.bundle
	// lastDispatchedDeltaBundleData's transportMetadata will be used later in case the processing fails, therefore
	// it should point to the delta-pack's earliest pending contributor's (current transport metadata)
	bi.lastDispatchedDeltaBundleData.lowestPendingTransportMetadata = bi.transportMetadata.bundleStatus
	// the bundle's transport metadata will be used to mark as processed, therefore we should give out that of the
	// latest pending contributing delta-bundle's
	bi.transportMetadata.bundleStatus = bi.lastReceivedTransportMetadata
	// reset bundle only when dispatching, to start a new delta-pack
	bi.bundle = nil

	return bi.transportMetadata
}

// update function to update the bundle and its metadata according to delta-state sync-mode.
func (bi *deltaConflationBundle) update(newBundle bundle.ManagerBundle, transportMetadata metadata.BundleStatus,
	overwriteMetadataObject bool,
) error {
	newDeltaBundle, ok := newBundle.(bundle.ManagerDeltaBundle)
	if !ok {
		return fmt.Errorf("%w - received type %s", errWrongBundleType, bundle.GetBundleType(newBundle))
	}

	bundleStartsNewLine := bi.bundle == nil ||
		newDeltaBundle.GetDependencyVersion().NewerThan(bi.bundle.GetDependencyVersion())

	if err := bi.updateBundle(newDeltaBundle); err != nil {
		return fmt.Errorf("failed to update bundle - %w", err)
	}

	bi.updateMetadata(bundle.GetBundleType(newDeltaBundle), newDeltaBundle.GetVersion(), transportMetadata,
		overwriteMetadataObject)

	// update transport metadata only if bundle starts a new line of deltas
	if bundleStartsNewLine {
		// update current line-version info
		bi.deltaLineHeadBundleVersion = bi.transportMetadata.bundleVersion
		bi.transportMetadata.bundleStatus = transportMetadata
	}

	return nil
}

// updateBundle updates the wrapped bundle and metadata according to the sync mode.
func (bi *deltaConflationBundle) updateBundle(newDeltaBundle bundle.ManagerDeltaBundle) error {
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
func (bi *deltaConflationBundle) updateMetadata(bundleType string, version *metadata.BundleVersion,
	bundleStatus metadata.BundleStatus, overwriteObject bool,
) {
	if bi.transportMetadata == nil { // new metadata
		bi.transportMetadata = &ConflationBundleMetadata{
			bundleType:   bundleType,
			bundleStatus: bundleStatus,
		}
	} else if !overwriteObject {
		// create new metadata with identical info and plug it in
		bi.transportMetadata = &ConflationBundleMetadata{
			bundleType:   bundleType,
			bundleStatus: bi.transportMetadata.bundleStatus, // preserve metadata of the earliest
		}
	}
	// update version
	bi.transportMetadata.bundleVersion = version
	// update latest received transport metadata for committing later if needed
	bi.lastReceivedTransportMetadata = bundleStatus
}

// handleFailure recovers from failure.
// The failed bundle's content is re-merged (not as source of truth) into the current active bundle,
// and the metadata is restored for safe committing (back to the first merged pending delta bundle's).
func (bi *deltaConflationBundle) handleFailure(failedMetadata *ConflationBundleMetadata) {
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
	bi.transportMetadata.bundleStatus = lastDispatchedTransportMetadata
}

// getBundleStatus returns the wrapped bundle's transport metadata.
func (bi *deltaConflationBundle) getBundleStatus() metadata.BundleStatus {
	if bi.transportMetadata == nil {
		return nil
	}

	return bi.transportMetadata.bundleStatus
}

// markAsProcessed releases the bundle content and marks transport metadata as processed.
func (bi *deltaConflationBundle) markAsProcessed(metadata *ConflationBundleMetadata) {
	metadata.bundleStatus.MarkAsProcessed()

	// release fail-recovery data
	bi.lastDispatchedDeltaBundleData.bundle = nil
	bi.lastDispatchedDeltaBundleData.lowestPendingTransportMetadata = nil
}
