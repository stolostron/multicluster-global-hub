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

// using threshold to indicate the bundle processed status
// the count means the retry times.
// 0 - the default initial value
// 1, 2, 3 ... - the retry times of current bundle has been failed processed
// -1 means it processed successfully
type thresholdBundleStatus struct {
	maxRetry int
	count    int
}

// the retry times(max) when the bundle has been failed processed
func NewThresholdBundleStatus(max int) *thresholdBundleStatus {
	return &thresholdBundleStatus{
		maxRetry: max,
		count:    0,
	}
}

// MarkAsProcessed function that marks the metadata as processed.
func (s *thresholdBundleStatus) MarkAsProcessed() {
	s.count = -1
}

// Processed returns whether the bundle was processed or not.
func (s *thresholdBundleStatus) Processed() bool {
	return s.count == -1 || s.count >= s.maxRetry
}

// MarkAsUnprocessed function that marks the metadata as processed.
func (s *thresholdBundleStatus) MarkAsUnprocessed() {
	s.count++
}
