package bundle

// NewLocalCompleteComplianceStatusBundle creates a new instance of LocalCompleteComplianceStatusBundle.
func NewLocalCompleteComplianceStatusBundle() Bundle {
	return &LocalCompleteComplianceStatusBundle{}
}

// LocalCompleteComplianceStatusBundle abstracts management of local complete compliance status bundle.
type LocalCompleteComplianceStatusBundle struct{ CompleteComplianceStatusBundle }
