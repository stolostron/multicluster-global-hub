package conflator

import (
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
)

// BundleMetadata abstracts metadata of conflation elements inside the conflation units.
type BundleMetadata struct {
	bundleType    string
	bundleVersion *status.BundleVersion
	// transport metadata is information we need for marking bundle as consumed in transport (e.g. commit offset)
	transportBundleMetadata bundle.BundleMetadata
}
