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

// NewerThanOrEqual returns whether the caller's version is newer than or equal to the version received as an argument.
// If other is nil, the result is true.
func (v *Version) NewerThanOrEqual(other *Version) bool {
	if other == nil {
		return true
	}
	return v.Generation >= other.Generation && v.Value >= other.Value
}

// Equals returns whether the caller's version is equal to that received as argument.
func (v *Version) Equals(other *Version) bool {
	return v.Generation == other.Generation && v.Value == other.Value
}

// Equals returns whether the bundles are updated with the same value.
func (v *Version) EqualValue(other *Version) bool {
	if other == nil {
		return false
	}
	return v.Value == other.Value
}

// NewerValueThan returns whether the caller's value is newer than that received as argument.
// If other = nil the result is true.
func (v *Version) NewerValueThan(other *Version) bool {
	if other == nil {
		return true
	}
	return v.Value > other.Value
}

func (v *Version) NewerGenerationThan(other *Version) bool {
	if other == nil {
		return true
	}
	return v.Generation >= other.Generation
}

// String returns string representation of the bundle version.
func (v *Version) String() string {
	return fmt.Sprintf("%d.%d", v.Generation, v.Value)
}

// Incr increments the Value when bundle is updated.
func (v *Version) Incr() {
	v.Value++
	if v.Value >= math.MaxUint64-100 {
		v.Reset()
	}
}

// Next increments the Generation and resets the Value when bundle is sended to the hub.
func (v *Version) Next() {
	v.Generation++
	if v.Generation >= math.MaxUint64-100 {
		v.Reset()
	}
}

// Reset resets the bundle version with minimal Generation and Value.
func (v *Version) Reset() {
	v.Generation = 0
	v.Value = 0
}

// InitGen returns whether the bundle version is first Generation.
func (v *Version) InitGen() bool {
	return v.Generation == 0
}
