package dependency

// NewDependency creates a new instance of dependency.
func NewDependency(eventType string, dependencyType DependencyType) *Dependency {
	return &Dependency{
		EventType:      eventType,
		DependencyType: dependencyType,
	}
}

// Dependency represents the dependency between different bundles. a bundle can depend only on one other bundle.
type Dependency struct {
	EventType      string
	DependencyType DependencyType
}
