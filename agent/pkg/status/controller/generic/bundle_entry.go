package generic

import (
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
)

// NewBundleEntry creates a new instance of BundleCollectionEntry.
func NewBundleEntry(transportBundleKey string, bundle bundle.AgentBundle,
	bundlePredicate func() bool,
) *BundleEntry {
	return &BundleEntry{
		transportBundleKey:    transportBundleKey,
		Bundle:                bundle,
		bundlePredicate:       bundlePredicate,
		lastSentBundleVersion: *bundle.GetVersion(),
	}
}

// BundleEntry holds information about a specific bundle.
type BundleEntry struct {
	transportBundleKey    string
	Bundle                bundle.AgentBundle
	bundlePredicate       func() bool
	lastSentBundleVersion metadata.BundleVersion // not pointer so it does not point to the bundle's internal version
}
