package bundle

import "github.com/stolostron/multicluster-global-hub/pkg/bundle/status"

// NewLocalCompleteComplianceStatusBundle creates a new instance of LocalCompleteComplianceStatusBundle.
func NewLocalCompleteComplianceStatusBundle() status.Bundle {
	return &LocalCompleteComplianceStatusBundle{}
}

// LocalCompleteComplianceStatusBundle abstracts management of local complete compliance status bundle.
type LocalCompleteComplianceStatusBundle struct{ CompleteComplianceStatusBundle }
