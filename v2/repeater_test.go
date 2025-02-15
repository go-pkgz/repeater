package repeater

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepeater(t *testing.T) {
	t.Run("zero or negative attempts converted to 1", func(t *testing.T) {
		r := NewFixed(0, time.Millisecond)
		assert.Equal(t, 1, r.attempts)
		r = NewFixed(-1, time.Millisecond)
		assert.Equal(t, 1, r.attempts)
	})

	t.Run("nil strategy defaults to fixed 1s", func(t *testing.T) {
		r := NewWithStrategy(1, nil)
		s, ok := r.strategy.(FixedDelay)
		require.True(t, ok)
		assert.Equal(t, time.Second, s.Delay)
	})
}

func TestDo(t *testing.T) {
	t.Run("success first try", func(t *testing.T) {
		calls := 0
		r := NewFixed(3, time.Millisecond)
		err := r.Do(context.Background(), func() error {
			calls++
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, 1, calls)
	})

	t.Run("success after retries", func(t *testing.T) {
		calls := 0
		r := NewFixed(3, time.Millisecond)
		err := r.Do(context.Background(), func() error {
			calls++
			if calls < 3 {
				return errors.New("not yet")
			}
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, 3, calls)
	})

	t.Run("failure after all attempts", func(t *testing.T) {
		calls := 0
		r := NewFixed(3, time.Millisecond)
		err := r.Do(context.Background(), func() error {
			calls++
			return errors.New("always fails")
		})
		require.Error(t, err)
		assert.Equal(t, "always fails", err.Error())
		assert.Equal(t, 3, calls)
	})

	t.Run("stops on critical error", func(t *testing.T) {
		calls := 0
		criticalErr := errors.New("critical")
		r := NewFixed(5, time.Millisecond)
		err := r.Do(context.Background(), func() error {
			calls++
			return criticalErr
		}, criticalErr)
		require.ErrorIs(t, err, criticalErr)
		assert.Equal(t, 1, calls)
	})

	t.Run("stops on wrapped critical error", func(t *testing.T) {
		calls := 0
		criticalErr := errors.New("critical")
		r := NewFixed(5, time.Millisecond)
		err := r.Do(context.Background(), func() error {
			calls++
			return errors.Join(errors.New("wrapped"), criticalErr)
		}, criticalErr)
		require.ErrorIs(t, err, criticalErr)
		assert.Equal(t, 1, calls)
	})
}

func TestDoContext(t *testing.T) {
	t.Run("respects cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		calls := 0
		r := NewFixed(5, 100*time.Millisecond)

		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		err := r.Do(ctx, func() error {
			calls++
			return errors.New("failed")
		})

		require.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, 1, calls)
	})

	t.Run("timeout before first attempt", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()

		time.Sleep(5 * time.Millisecond) // ensure timeout
		calls := 0
		r := NewFixed(5, time.Millisecond)
		err := r.Do(ctx, func() error {
			calls++
			return errors.New("failed")
		})
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Equal(t, 0, calls)
	})
}

func TestDoWithErrAny(t *testing.T) {
	t.Run("stops on any error", func(t *testing.T) {
		calls := 0
		r := NewFixed(5, time.Millisecond)
		err := r.Do(context.Background(), func() error {
			calls++
			return errors.New("some error")
		}, ErrAny)
		require.Error(t, err)
		assert.Equal(t, 1, calls, "should stop on first error with ErrAny")
	})

	t.Run("combines with other critical errors", func(t *testing.T) {
		counts := make(map[string]int)
		r := NewFixed(5, time.Millisecond)
		criticalErr := errors.New("critical")

		err := r.Do(context.Background(), func() error {
			// return different errors
			counts["total"]++
			if counts["total"] == 2 {
				return criticalErr
			}
			return errors.New("non-critical")
		}, criticalErr, ErrAny)

		require.Error(t, err)
		assert.Equal(t, 1, counts["total"], "should stop on first error when ErrAny is used")
	})
}

func TestNewBackoff(t *testing.T) {
	r := NewBackoff(5, time.Second)
	assert.Equal(t, 5, r.attempts)

	st, ok := r.strategy.(*backoff)
	require.True(t, ok)

	// check defaults
	assert.Equal(t, time.Second, st.initial)
	assert.Equal(t, 30*time.Second, st.maxDelay)
	assert.Equal(t, BackoffExponential, st.btype)
	assert.Equal(t, 0.1, st.jitter)

	// check with options
	r = NewBackoff(5, time.Second,
		WithMaxDelay(5*time.Second),
		WithBackoffType(BackoffLinear),
		WithJitter(0.2),
	)
	st, ok = r.strategy.(*backoff)
	require.True(t, ok)
	assert.Equal(t, time.Second, st.initial)
	assert.Equal(t, 5*time.Second, st.maxDelay)
	assert.Equal(t, BackoffLinear, st.btype)
	assert.Equal(t, 0.2, st.jitter)
}

func TestBackoffReal(t *testing.T) {
	startTime := time.Now()
	var attempts []time.Time

	expectedAttempts := 4
	r := NewBackoff(expectedAttempts, 10*time.Millisecond, WithJitter(0))

	// record all attempt times
	err := r.Do(context.Background(), func() error {
		attempts = append(attempts, time.Now())
		return errors.New("test error")
	})
	require.Error(t, err)

	assert.Equal(t, expectedAttempts, len(attempts), "should make exactly %d attempts", expectedAttempts)

	// first attempt should be immediate
	assert.Less(t, attempts[0].Sub(startTime), 5*time.Millisecond)

	// check intervals between attempts
	var intervals []time.Duration
	for i := 1; i < len(attempts); i++ {
		intervals = append(intervals, attempts[i].Sub(attempts[i-1]))
		t.Logf("attempt %d interval: %v", i, intervals[i-1])
	}

	// check total time for all attempts
	// with exponential backoff and 10ms initial delay we expect:
	// - attempt 1 - immediate (0ms)
	// - attempt 2 - after 10ms delay  (total ~10ms)
	// - attempt 3 - after 20ms delay  (total ~30ms)
	// - attempt 4 - after 40ms delay  (total ~70ms)
	totalTime := attempts[len(attempts)-1].Sub(startTime)
	assert.Greater(t, totalTime, 65*time.Millisecond)
	assert.Less(t, totalTime, 75*time.Millisecond)
}

func ExampleRepeater_Do() {
	// create repeater with exponential backoff
	r := NewBackoff(5, time.Second)

	err := r.Do(context.Background(), func() error {
		// simulating successful operation
		return nil
	})

	fmt.Printf("completed with error: %v", err)
	// Output: completed with error: <nil>
}

func ExampleNewFixed() {
	// create repeater with fixed 100ms delay between attempts
	r := NewFixed(3, 100*time.Millisecond)

	// retry on "temp error" but give up immediately on "critical error"
	criticalErr := errors.New("critical error")

	// run Do and check the returned error
	err := r.Do(context.Background(), func() error {
		// simulating critical error
		return criticalErr
	}, criticalErr)

	if err != nil {
		fmt.Printf("got expected error: %v", err)
	}
	// Output: got expected error: critical error
}

func ExampleNewBackoff() {
	// create backoff repeater with custom settings
	r := NewBackoff(3, time.Millisecond,
		WithMaxDelay(10*time.Millisecond),
		WithBackoffType(BackoffLinear),
		WithJitter(0),
	)

	var attempts int
	err := r.Do(context.Background(), func() error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary error")
		}
		return nil
	})

	fmt.Printf("completed with error: %v after %d attempts", err, attempts)
	// Output: completed with error: <nil> after 3 attempts
}
