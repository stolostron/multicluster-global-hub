package status

import (
	"fmt"
)

// NewBundleVersion returns a new instance of BundleVersion.
func NewBundleVersion() *BundleVersion {
	return &BundleVersion{
		Generation: 0,
		Value:      0,
	}
}

// BundleVersion holds the information necessary for the consumers of status bundles to compare versions correctly.
type BundleVersion struct {
	// this Value is incremented every time the bundle is sended to the hub
	Generation uint64 `json:"Generation"`
	// this Value is incremented every time the bundle is updated
	Value uint64 `json:"Value"`
}

// NewerThan returns whether the caller's version is newer than that received as argument.
// If other = nil the result is true.
func (this *BundleVersion) NewerThan(other *BundleVersion) bool {
	if other == nil {
		return true
	}
	if this.Generation > other.Generation {
		return true
	} else if this.Generation < other.Generation {
		return false
	} else {
		return this.Value > other.Value
	}
}

// Equals returns whether the caller's version is equal to that received as argument.
func (this *BundleVersion) Equals(other *BundleVersion) bool {
	return this.Generation == other.Generation && this.Value == other.Value
}

// Equals returns whether the bundles are updated with the same value.
func (this *BundleVersion) EqualValue(other *BundleVersion) bool {
	return this.Value == other.Value
}

// NewerValueThan returns whether the caller's value is newer than that received as argument.
// If other = nil the result is true.
func (this *BundleVersion) NewerValueThan(other *BundleVersion) bool {
	if other == nil {
		return true
	}
	return this.Value > other.Value
}

// String returns string representation of the bundle version.
func (this *BundleVersion) String() string {
	return fmt.Sprintf("%d.%d", this.Generation, this.Value)
}

// Incr increments the Value when bundle is updated.
func (this *BundleVersion) Incr() {
	this.Value++
}

// Next increments the Generation and resets the Value when bundle is sended to the hub.
func (this *BundleVersion) Next() {
	this.Generation++
}

// Reset resets the bundle version with minimal Generation and Value.
func (this *BundleVersion) Reset() {
	this.Generation = 0
	this.Value = 0
}

// InitGen returns whether the bundle version is first Generation.
func (this *BundleVersion) InitGen() bool {
	return this.Generation == 0
}
