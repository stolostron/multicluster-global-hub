package transport

// GetBundlesMetadataFunc is the function to be called by committer to fetch metadata to commit.
type GetBundlesMetadataFunc func() []BundleMetadata

// BundleMetadata may include metadata that relates to transport - e.g. commit offset.
type BundleMetadata interface {
	// MarkAsProcessed function that marks the metadata as processed.
	MarkAsProcessed()
	// Processed returns whether the bundle was processed or not.
	Processed() bool
}

// NewBaseBundleMetadata returns a new instance of BaseBundleMetadata.
func NewBaseBundleMetadata() *BaseBundleMetadata {
	return &BaseBundleMetadata{
		processed: false,
	}
}

// BaseBundleMetadata wraps the shared data/functionality that the different transport BundleMetadata implementations-
// can be based on.
type BaseBundleMetadata struct {
	processed bool
}

// MarkAsProcessed function that marks the metadata as processed.
func (metadata *BaseBundleMetadata) MarkAsProcessed() {
	metadata.processed = true
}

// Processed returns whether the bundle was processed or not.
func (metadata *BaseBundleMetadata) Processed() bool {
	return metadata.processed
}
