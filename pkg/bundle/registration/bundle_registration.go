package registration

import "github.com/stolostron/multicluster-global-hub/pkg/bundle/status"

// BundleRegistration abstract the registration for bundles according to bundle key in transport layer.
type BundleRegistration struct {
	MsgID            string
	CreateBundleFunc func() status.Bundle
	Predicate        func() bool
}
