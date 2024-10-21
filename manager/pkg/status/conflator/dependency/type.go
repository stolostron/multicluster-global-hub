package dependency

type DependencyType string

const (

	// ExactMatch used to specify that dependant bundle requires the exact version of the dependency to be the
	// last processed bundle.
	ExactMatch DependencyType = "ExactMatch"
	// AtLeast used to specify that dependant bundle requires at least some version of the dependency to be processed.
	AtLeast DependencyType = "AtLeast"
)
