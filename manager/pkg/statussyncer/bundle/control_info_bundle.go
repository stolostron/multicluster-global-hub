package bundle

import "github.com/stolostron/multicluster-global-hub/pkg/bundle/status"

// NewControlInfoBundle creates a new instance of ControlInfoBundle.
func NewControlInfoBundle() status.Bundle {
	return &ControlInfoBundle{}
}

// ControlInfoBundle abstracts management of control info bundle.
type ControlInfoBundle struct {
	baseBundle
}

// GetObjects returns the objects in the bundle.
func (bundle *ControlInfoBundle) GetObjects() []interface{} {
	return nil
}
