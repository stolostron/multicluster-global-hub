package status

import "fmt"

// NewBundleVersion returns a new instance of BundleVersion.
func NewBundleVersion(incarnation uint64, generation uint64) *BundleVersion {
	return &BundleVersion{
		Incarnation: incarnation,
		Generation:  generation,
	}
}

// BundleVersion holds the information necessary for the consumers of status bundles to compare versions correctly.
// Incarnation is the instance-version of the sender component, a counter that increments on restarts of the latter.
// Generation is the bundle-version relative to the sender's instance, increments on bundle updates.
type BundleVersion struct {
	Incarnation uint64 `json:"incarnation"`
	Generation  uint64 `json:"generation"`
}

// NewerThan returns whether the caller's version is newer than that received as argument.
//
// If other = nil the result is true.
func (version *BundleVersion) NewerThan(other *BundleVersion) bool {
	if other == nil {
		return true
	}

	if version.Incarnation == other.Incarnation {
		return version.Generation > other.Generation
	}

	return version.Incarnation > other.Incarnation
}

// Equals returns whether the caller's version is equal to that received as argument.
func (version *BundleVersion) Equals(other *BundleVersion) bool {
	return version.Incarnation == other.Incarnation && version.Generation == other.Generation
}

// String returns string representation of the bundle version.
func (version *BundleVersion) String() string {
	return fmt.Sprintf("%d.%d", version.Incarnation, version.Generation)
}
