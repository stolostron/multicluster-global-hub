package bundle

// CustomBundleRegistration abstract the registration for custom bundles according to bundle ID in transport layer.
// Custom-bundles are bundles that do not implement bundle.GenericBundle.
type CustomBundleRegistration struct {
	InitBundlesResourceFunc func() interface{}
	BundleUpdatesChan       chan interface{}
}
