package statistics

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// genericMetrics contains average and maximum times in milliseconds.
type genericMetrics struct {
	failures      int64      // number of failures
	successes     int64      // number of successes
	totalDuration int64      // in milliseconds
	maxDuration   int64      // in milliseconds
	mutex         sync.Mutex // for updating and getting data in consistent state
}

func (tm *genericMetrics) add(duration time.Duration, err error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	tm.addUnsafe(duration, err)
}

// addUnsafe is unsafe version of add() method which contains a common logic.
// The method shouldn't be accessed directly, but using wrapper methods which lock shared mutex.
func (tm *genericMetrics) addUnsafe(duration time.Duration, err error) {
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
func (tm *genericMetrics) toString() string {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	return tm.toStringUnsafe()
}

// toStringUnsafe is unsafe version of toString() method which contains a common logic.
// The method shouldn't be accessed directly, but using wrapper methods which lock shared mutex.
func (tm *genericMetrics) toStringUnsafe() string {
	average := 0.0

	if tm.successes != 0 {
		average = float64(tm.totalDuration / tm.successes)
	}

	var metrics strings.Builder
	if tm.failures != 0 {
		metrics.WriteString(fmt.Sprintf("failures=%d, ", tm.failures))
	}
	metrics.WriteString(fmt.Sprintf("successes=%d, avg=%.0f ms, max=%d ms", tm.successes, average, tm.maxDuration))
	return metrics.String()
}
