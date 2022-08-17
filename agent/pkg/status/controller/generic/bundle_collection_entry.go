package generic

import (
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/bundle"
	statusbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
)

// NewBundleCollectionEntry creates a new instance of BundleCollectionEntry.
func NewBundleCollectionEntry(transportBundleKey string, bundle bundle.Bundle,
	predicate func() bool,
) *BundleCollectionEntry {
	return &BundleCollectionEntry{
		transportBundleKey:    transportBundleKey,
		bundle:                bundle,
		predicate:             predicate,
		lastSentBundleVersion: *bundle.GetBundleVersion(),
	}
}

// BundleCollectionEntry holds information about a specific bundle.
type BundleCollectionEntry struct {
	transportBundleKey    string
	bundle                bundle.Bundle
	predicate             func() bool
	lastSentBundleVersion statusbundle.BundleVersion // not pointer so it does not point to the bundle's internal version
}
