package bundle

// NewControlInfoBundle creates a new instance of ControlInfoBundle.
func NewControlInfoBundle() Bundle {
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
