package enum

// EventSyncMode used to identify hybrid sync mode - complete/delta bundles.
type EventSyncMode int8

const (
	// CompleteStateMode used to identify sync mode of complete state bundles.
	CompleteStateMode EventSyncMode = iota
	// DeltaStateMode used to identify sync mode of delta state bundles.
	DeltaStateMode EventSyncMode = iota
	// HybridStateMode enables synchronization using both full state (via list) and delta updates (via watch).
	HybridStateMode EventSyncMode = iota
)
