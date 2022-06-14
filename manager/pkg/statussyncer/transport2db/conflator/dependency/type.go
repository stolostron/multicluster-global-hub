package dependency

type dependencyType string

const (

	// ExactMatch used to specify that dependant bundle requires the exact version of the dependency to be the
	// last processed bundle.
	ExactMatch dependencyType = "ExactMatch"
	// AtLeast used to specify that dependant bundle requires at least some version of the dependency to be processed.
	AtLeast dependencyType = "AtLeast"
)
