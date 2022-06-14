package intervalpolicy

import "time"

// IntervalPolicy defines a policy to return interval based on the received events.
type IntervalPolicy interface {
	// Evaluate evaluates next interval.
	Evaluate()
	// Reset resets the interval of the interval policy.
	Reset()
	// GetInterval returns current interval.
	GetInterval() time.Duration
	// GetMaxInterval returns the max interval that can be used to sync bundles.
	GetMaxInterval() time.Duration
}
