package status

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

// MarkAsUnprocessed function that marks the metadata as processed.
func (genericBundleStatus *GenericBundleStatus) MarkAsUnprocessed() {
	genericBundleStatus.processed = false
}

// Processed returns whether the bundle was processed or not.
func (genericBundleStatus *GenericBundleStatus) Processed() bool {
	return genericBundleStatus.processed
}
