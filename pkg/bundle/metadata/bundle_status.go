package metadata

// GetBundleStatusesFunc is the function to be called by committer to fetch metadata to commit.
type GetBundleStatusesFunc func() []BundleStatus

// BundleStatus indicate whether the bundle is processed,
// may include metadata that relates to transport - e.g. commit offset.
type BundleStatus interface {
	// MarkAsProcessed function that marks the metadata as processed.
	MarkAsProcessed()
	// Processed returns whether the bundle was processed or not.
	Processed() bool
	// MarkAsUnprocessed function that marks the metadata as unprocessed.
	MarkAsUnprocessed()
}
