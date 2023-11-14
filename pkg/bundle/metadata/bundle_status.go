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
}

// NewGenericBundleStatus returns a new instance of BaseBundleStatus
func NewGenericBundleStatus() *GenericBundleStatus {
	return &GenericBundleStatus{
		processed: false,
	}
}

// GenericBundleStatus wraps the shared data/functionality that the different transport BundleMetadata implementations-
// can be based on.
type GenericBundleStatus struct {
	processed bool
}

// MarkAsProcessed function that marks the metadata as processed.
func (genericBundleStatus *GenericBundleStatus) MarkAsProcessed() {
	genericBundleStatus.processed = true
}

// Processed returns whether the bundle was processed or not.
func (genericBundleStatus *GenericBundleStatus) Processed() bool {
	return genericBundleStatus.processed
}
