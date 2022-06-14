package intervalpolicy

import (
	"time"

	"github.com/cenkalti/backoff/v4"
)

const (
	baseFactor                              = 2
	maxInterval                             = 60 * time.Second
	consecutiveEvaluationsBeforeNextBackoff = 3
)

// NewExponentialBackoffPolicy creates new exponential backoff interval policy.
func NewExponentialBackoffPolicy(interval time.Duration) IntervalPolicy {
	exponentialBackoff := &backoff.ExponentialBackOff{
		InitialInterval:     interval,
		RandomizationFactor: 0,
		Multiplier:          baseFactor,
		MaxInterval:         maxInterval,
		MaxElapsedTime:      0,
		Stop:                0,
		Clock:               backoff.SystemClock,
	}

	exponentialBackoff.Reset()

	return &exponentialBackoffPolicy{
		exponentialBackoff:          exponentialBackoff,
		interval:                    exponentialBackoff.NextBackOff(), // after reset, next returns initial interval
		consecutiveEvaluationsCount: 0,
	}
}

// exponentialBackoffIntervalPolicy interval policy that implements exponential backoff.
type exponentialBackoffPolicy struct {
	exponentialBackoff          *backoff.ExponentialBackOff
	interval                    time.Duration
	consecutiveEvaluationsCount int
}

// Evaluate reevaluates the interval.
func (policy *exponentialBackoffPolicy) Evaluate() {
	policy.consecutiveEvaluationsCount++

	if policy.consecutiveEvaluationsCount == consecutiveEvaluationsBeforeNextBackoff {
		policy.interval = policy.exponentialBackoff.NextBackOff()
		policy.consecutiveEvaluationsCount = 0
	}
}

// Reset resets the entire state of the policy.
func (policy *exponentialBackoffPolicy) Reset() {
	policy.exponentialBackoff.Reset()
	policy.interval = policy.exponentialBackoff.NextBackOff() // after reset, next returns initial interval
	policy.consecutiveEvaluationsCount = 0
}

// GetInterval returns reevaluated interval.
func (policy *exponentialBackoffPolicy) GetInterval() time.Duration {
	return policy.interval
}

// GetMaxInterval returns the max interval that can be used to sync bundles.
func (policy *exponentialBackoffPolicy) GetMaxInterval() time.Duration {
	return maxInterval
}
