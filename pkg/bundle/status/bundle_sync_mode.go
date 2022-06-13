package status

// BundleSyncMode used to identify hybrid sync mode - complete/delta bundles.
type BundleSyncMode int8

const (
	// CompleteStateMode used to identify sync mode of complete state bundles.
	CompleteStateMode BundleSyncMode = iota
	// DeltaStateMode used to identify sync mode of delta state bundles.
	DeltaStateMode BundleSyncMode = iota
)
