// Package repeater implements retry functionality with different strategies.
// It provides fixed delays and various backoff strategies (constant, linear, exponential) with jitter support.
// The package allows custom retry strategies and error-specific handling. Context-aware implementation
// supports cancellation and timeouts.
package repeater

import (
	"context"
	"errors"
	"time"
)

// ErrAny is a special sentinel error that, when passed as a critical error to Do,
// makes it fail on any error from the function
var ErrAny = errors.New("any error")

// Stats holds execution statistics for a repeater run
type Stats struct {
	LastError     error         // Last error encountered (nil if succeeded)
	StartedAt     time.Time     // When the repeater started
	FinishedAt    time.Time     // When the repeater finished
	TotalDuration time.Duration // Total elapsed time from start to finish
	WorkDuration  time.Duration // Time spent executing the function (excluding delays)
	DelayDuration time.Duration // Time spent in delays between attempts
	Attempts      int           // Number of attempts made (including the successful one)
	Success       bool          // Whether the operation eventually succeeded
}

// Repeater holds configuration for retry operations.
// Note: Repeater is not thread-safe. Each Repeater instance should not be used
// concurrently for different functions. Create separate Repeater instances for
// concurrent operations.
type Repeater struct {
	strategy Strategy
	stats    Stats
	attempts int
}

// NewWithStrategy creates a repeater with a custom retry strategy
func NewWithStrategy(attempts int, strategy Strategy) *Repeater {
	if attempts <= 0 {
		attempts = 1
	}
	if strategy == nil {
		strategy = NewFixedDelay(time.Second)
	}
	return &Repeater{
		attempts: attempts,
		strategy: strategy,
	}
}

// NewBackoff creates a repeater with backoff strategy
// Default settings (can be overridden with options):
//   - 30s max delay
//   - exponential backoff
//   - 10% jitter
func NewBackoff(attempts int, initial time.Duration, opts ...backoffOption) *Repeater {
	return NewWithStrategy(attempts, newBackoff(initial, opts...))
}

// NewFixed creates a repeater with fixed delay strategy
func NewFixed(attempts int, delay time.Duration) *Repeater {
	return NewWithStrategy(attempts, NewFixedDelay(delay))
}

// Do repeats fun until it succeeds or max attempts reached
// terminates immediately on context cancellation or if err matches any in termErrs.
// if errs contains ErrAny, terminates on any error.
func (r *Repeater) Do(ctx context.Context, fun func() error, termErrs ...error) error {
	var lastErr error

	// reset and initialize stats
	r.stats = Stats{
		StartedAt: time.Now(),
	}

	inErrors := func(err error) bool {
		for _, e := range termErrs {
			if errors.Is(e, ErrAny) {
				return true
			}
			if errors.Is(err, e) {
				return true
			}
		}
		return false
	}

	for attempt := 0; attempt < r.attempts; attempt++ {
		// check context before each attempt
		if err := ctx.Err(); err != nil {
			r.stats.Attempts = attempt
			r.stats.LastError = err
			r.stats.FinishedAt = time.Now()
			r.stats.TotalDuration = r.stats.FinishedAt.Sub(r.stats.StartedAt)
			return err //nolint:wrapcheck // context errors are standard and don't need wrapping
		}

		workStart := time.Now()
		var err error
		if err = fun(); err == nil {
			r.stats.Attempts = attempt + 1
			r.stats.Success = true
			r.stats.WorkDuration += time.Since(workStart)
			r.stats.FinishedAt = time.Now()
			r.stats.TotalDuration = r.stats.FinishedAt.Sub(r.stats.StartedAt)
			return nil
		}

		r.stats.WorkDuration += time.Since(workStart)

		lastErr = err
		if inErrors(err) {
			r.stats.Attempts = attempt + 1
			r.stats.LastError = err
			r.stats.FinishedAt = time.Now()
			r.stats.TotalDuration = r.stats.FinishedAt.Sub(r.stats.StartedAt)
			return err
		}

		// don't sleep after the last attempt
		if attempt < r.attempts-1 {
			delay := r.strategy.NextDelay(attempt + 1)
			if delay > 0 {
				delayStart := time.Now()
				select {
				case <-ctx.Done():
					r.stats.Attempts = attempt + 1
					r.stats.LastError = ctx.Err()
					r.stats.DelayDuration += time.Since(delayStart)
					r.stats.FinishedAt = time.Now()
					r.stats.TotalDuration = r.stats.FinishedAt.Sub(r.stats.StartedAt)
					return ctx.Err() //nolint:wrapcheck // context errors are standard and don't need wrapping
				case <-time.After(delay):
					r.stats.DelayDuration += time.Since(delayStart)
				}
			}
		}
	}

	r.stats.Attempts = r.attempts
	r.stats.LastError = lastErr
	r.stats.FinishedAt = time.Now()
	r.stats.TotalDuration = r.stats.FinishedAt.Sub(r.stats.StartedAt)

	return lastErr
}

// Stats returns the execution statistics from the last Do() call
func (r *Repeater) Stats() Stats {
	return r.stats
}
