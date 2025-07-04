package version

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	ExtVersion           = "extversion"
	ExtDependencyVersion = "extdependencyversion"
	MaxUint64            = math.MaxUint64 - 10
)

// NewVersion returns a new instance of BundleVersion.
func NewVersion() *Version {
	return &Version{
		Generation: 0,
		Value:      0,
	}
}

func VersionFrom(strVersion string) (*Version, error) {
	vals := strings.Split(strVersion, ".")
	if len(vals) != 2 {
		return nil, fmt.Errorf("malformed version string: %s", strVersion)
	}
	generation, err := strconv.ParseUint(vals[0], 10, 64)
	if err != nil {
		return nil, err
	}
	val, err := strconv.ParseUint(vals[1], 10, 64)
	if err != nil {
		return nil, err
	}
	return &Version{
		Generation: generation,
		Value:      val,
	}, nil
}

// Version holds the information necessary for the consumers of status bundles to compare versions correctly.
type Version struct {
	// this Value is incremented every time the bundle is sended to the hub
	Generation uint64 `json:"Generation"`
	// this Value is incremented every time the bundle is updated
	Value uint64 `json:"Value"`
}

// NewerThan returns whether the caller's version is newer than that received as argument.
// If other = nil the result is true.
func (this *Version) NewerThan(other *Version) bool {
	if other == nil {
		return true
	}
	if this.Equals(other) {
		return false
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
func (this *Version) Equals(other *Version) bool {
	return this.Generation == other.Generation && this.Value == other.Value
}

// Equals returns whether the bundles are updated with the same value.
func (this *Version) EqualValue(other *Version) bool {
	if other == nil {
		return false
	}
	return this.Value == other.Value
}

// NewerValueThan returns whether the caller's value is newer than that received as argument.
// If other = nil the result is true.
func (this *Version) NewerValueThan(other *Version) bool {
	if other == nil {
		return true
	}
	return this.Value > other.Value
}

func (this *Version) NewerGenerationThan(other *Version) bool {
	if other == nil {
		return true
	}
	return this.Generation >= other.Generation
}

// String returns string representation of the bundle version.
func (this *Version) String() string {
	return fmt.Sprintf("%d.%d", this.Generation, this.Value)
}

// Incr increments the Value when bundle is updated.
func (this *Version) Incr() {
	this.Value++
	if this.Value >= math.MaxUint64-100 {
		this.Reset()
	}
}

// Next increments the Generation and resets the Value when bundle is sended to the hub.
func (this *Version) Next() {
	this.Generation++
	if this.Generation >= math.MaxUint64-100 {
		this.Reset()
	}
}

// Reset resets the bundle version with minimal Generation and Value.
func (this *Version) Reset() {
	this.Generation = 0
	this.Value = 0
}

// InitGen returns whether the bundle version is first Generation.
func (this *Version) InitGen() bool {
	return this.Generation == 0
}
