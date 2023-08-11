package statistics

import (
	"time"
)

// conflationUnitMetrics extends timeMeasurement and adds conflation measurements.
type conflationUnitMetrics struct {
	genericMetrics
	startTimestamps map[string]int64
}

func (cum *conflationUnitMetrics) start(conflationUnitName string) {
	cum.mutex.Lock()
	defer cum.mutex.Unlock()

	cum.startTimestamps[conflationUnitName] = time.Now().Unix()
}

func (cum *conflationUnitMetrics) stop(conflationUnitName string, err error) {
	cum.mutex.Lock()
	defer cum.mutex.Unlock()

	startTme := cum.startTimestamps[conflationUnitName]
	cum.addUnsafe(time.Since(time.Unix(startTme, 0)), err)
}
