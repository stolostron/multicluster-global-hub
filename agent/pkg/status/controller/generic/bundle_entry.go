package generic

import (
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
)

// NewBundleEntry creates a new instance of BundleCollectionEntry.
func NewBundleEntry(transportBundleKey string, bundle bundle.AgentBundle, predicate func() bool,
) *BundleEntry {
	return &BundleEntry{
		transportBundleKey:    transportBundleKey,
		bundle:                bundle,
		predicate:             predicate,
		lastSentBundleVersion: *bundle.GetVersion(),
	}
}

// BundleEntry holds information about a specific bundle.
type BundleEntry struct {
	transportBundleKey    string
	bundle                bundle.AgentBundle
	predicate             func() bool
	lastSentBundleVersion metadata.BundleVersion // not pointer so it does not point to the bundle's internal version
}
