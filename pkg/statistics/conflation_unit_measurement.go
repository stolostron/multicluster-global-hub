package statistics

import (
	"fmt"
	"time"
)

// conflationUnitMeasurement extends timeMeasurement and adds conflation measurements.
type conflationUnitMeasurement struct {
	timeMeasurement
	numOfConflations int64
	startTimestamps  map[string]int64
}

func (cum *conflationUnitMeasurement) start(conflationUnitName string) {
	cum.mutex.Lock()
	defer cum.mutex.Unlock()

	cum.startTimestamps[conflationUnitName] = time.Now().Unix()
}

func (cum *conflationUnitMeasurement) stop(conflationUnitName string) {
	cum.mutex.Lock()
	defer cum.mutex.Unlock()

	startTme := cum.startTimestamps[conflationUnitName]

	cum.addUnsafe(time.Since(time.Unix(startTme, 0)), nil)
}

// incrementNumberOfConflations increments number of conflations.
func (cum *conflationUnitMeasurement) incrementNumberOfConflations() {
	cum.mutex.Lock()
	defer cum.mutex.Unlock()

	cum.numOfConflations++
}

// toString is a safe version and must be called in every place where we want to print conflationUnitMeasurement's data.
func (cum *conflationUnitMeasurement) toString() string {
	cum.mutex.Lock()
	defer cum.mutex.Unlock()

	return fmt.Sprintf("%s, num of conflations=%d", cum.timeMeasurement.toStringUnsafe(), cum.numOfConflations)
}
