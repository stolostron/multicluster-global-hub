package statistics

import (
	"fmt"
	"sync"
	"time"
)

// timeMeasurement contains average and maximum times in milliseconds.
type timeMeasurement struct {
	failures      int64      // number of failures
	successes     int64      // number of successes
	totalDuration int64      // in milliseconds
	maxDuration   int64      // in milliseconds
	mutex         sync.Mutex // for updating and getting data in consistent state
}

func (tm *timeMeasurement) add(duration time.Duration, err error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	tm.addUnsafe(duration, err)
}

// addUnsafe is unsafe version of add() method which contains a common logic.
// The method shouldn't be accessed directly, but using wrapper methods which lock shared mutex.
func (tm *timeMeasurement) addUnsafe(duration time.Duration, err error) {
	if err != nil {
		tm.failures++
		return
	}

	durationMilliseconds := duration.Milliseconds()

	tm.successes++
	tm.totalDuration += durationMilliseconds

	if tm.maxDuration < durationMilliseconds {
		tm.maxDuration = durationMilliseconds
	}
}

// toString is a safe version and must be called in every place where we want to print timeMeasurement's data.
func (tm *timeMeasurement) toString() string {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	return tm.toStringUnsafe()
}

// toStringUnsafe is unsafe version of toString() method which contains a common logic.
// The method shouldn't be accessed directly, but using wrapper methods which lock shared mutex.
func (tm *timeMeasurement) toStringUnsafe() string {
	average := 0.0

	if tm.successes != 0 {
		average = float64(tm.totalDuration / tm.successes)
	}

	return fmt.Sprintf("failures=%d, successes=%d, average time=%.2f ms, max time=%d ms",
		tm.failures, tm.successes, average, tm.maxDuration)
}
