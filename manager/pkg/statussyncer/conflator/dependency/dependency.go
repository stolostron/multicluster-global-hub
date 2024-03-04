package dependency

// NewDependency creates a new instance of dependency.
func NewDependency(bundleType string, dependencyType dependencyType) *Dependency {
	return &Dependency{
		BundleType:     bundleType,
		DependencyType: dependencyType,
	}
}

// Dependency represents the dependency between different bundles. a bundle can depend only on one other bundle.
type Dependency struct {
	BundleType     string
	DependencyType dependencyType
}
