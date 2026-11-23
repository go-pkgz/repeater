package repeater

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFixedDelay(t *testing.T) {
	s := NewFixedDelay(time.Second)
	assert.Equal(t, time.Second, s.NextDelay(1))
	assert.Equal(t, time.Second, s.NextDelay(5))
	assert.Equal(t, time.Second, s.NextDelay(0))
}

func TestBackoff(t *testing.T) {
	t.Run("base cases", func(t *testing.T) {
		s := newBackoff(time.Second, WithJitter(0))
		assert.Equal(t, 0*time.Second, s.NextDelay(0))
		assert.Equal(t, 1*time.Second, s.NextDelay(1))
	})

	t.Run("backoff types", func(t *testing.T) {
		initial := 100 * time.Millisecond

		t.Run("constant", func(t *testing.T) {
			s := newBackoff(initial, WithJitter(0), WithBackoffType(BackoffConstant))
			assert.Equal(t, initial, s.NextDelay(1))
			assert.Equal(t, initial, s.NextDelay(2))
			assert.Equal(t, initial, s.NextDelay(3))
		})

		t.Run("linear", func(t *testing.T) {
			s := newBackoff(initial, WithJitter(0), WithBackoffType(BackoffLinear))
			assert.Equal(t, 1*initial, s.NextDelay(1))
			assert.Equal(t, 2*initial, s.NextDelay(2))
			assert.Equal(t, 3*initial, s.NextDelay(3))
		})

		t.Run("exponential", func(t *testing.T) {
			s := newBackoff(initial, WithJitter(0), WithBackoffType(BackoffExponential))
			assert.Equal(t, 1*initial, s.NextDelay(1))
			assert.Equal(t, 2*initial, s.NextDelay(2))
			assert.Equal(t, 4*initial, s.NextDelay(3))
			assert.Equal(t, 8*initial, s.NextDelay(4))
		})
	})

	t.Run("max delay", func(t *testing.T) {
		s := newBackoff(time.Second, WithJitter(0), WithMaxDelay(2*time.Second))
		assert.Equal(t, 1*time.Second, s.NextDelay(1))
		assert.Equal(t, 2*time.Second, s.NextDelay(2))
		assert.Equal(t, 2*time.Second, s.NextDelay(3)) // capped at max delay
	})

	t.Run("large attempts stay within max delay", func(t *testing.T) {
		for _, btype := range []BackoffType{BackoffConstant, BackoffLinear, BackoffExponential} {
			s := newBackoff(time.Second, WithJitter(0), WithBackoffType(btype)) // default 30s max delay
			for attempt := 1; attempt <= 200; attempt++ {
				delay := s.NextDelay(attempt)
				assert.Greater(t, delay, time.Duration(0), "type %d, attempt %d", btype, attempt)
				assert.LessOrEqual(t, delay, 30*time.Second, "type %d, attempt %d", btype, attempt)
			}
			assert.Greater(t, s.NextDelay(math.MaxInt), time.Duration(0), "type %d, attempt MaxInt", btype)
		}

		s := newBackoff(time.Second, WithJitter(0))
		assert.Equal(t, 16*time.Second, s.NextDelay(5))
		assert.Equal(t, 30*time.Second, s.NextDelay(6))
		assert.Equal(t, 30*time.Second, s.NextDelay(35))
		assert.Equal(t, 30*time.Second, s.NextDelay(64))
		assert.Equal(t, 30*time.Second, s.NextDelay(math.MaxInt))

		s = newBackoff(time.Hour, WithJitter(0), WithBackoffType(BackoffLinear))
		assert.Equal(t, 30*time.Second, s.NextDelay(math.MaxInt))
	})

	t.Run("no max delay saturates", func(t *testing.T) {
		s := newBackoff(time.Second, WithJitter(0), WithMaxDelay(0))
		assert.Equal(t, 512*time.Second, s.NextDelay(10))
		prev := time.Duration(0)
		for attempt := 1; attempt <= 200; attempt++ {
			delay := s.NextDelay(attempt)
			assert.GreaterOrEqual(t, delay, prev, "attempt %d", attempt)
			prev = delay
		}
		assert.Equal(t, time.Duration(math.MaxInt64), s.NextDelay(100))

		s = newBackoff(time.Hour, WithJitter(0), WithMaxDelay(0), WithBackoffType(BackoffLinear))
		assert.Equal(t, time.Duration(math.MaxInt64), s.NextDelay(math.MaxInt))

		s = newBackoff(time.Second, WithMaxDelay(0)) // default jitter
		for range 100 {
			assert.Greater(t, s.NextDelay(100), time.Duration(0))
		}
	})

	t.Run("jitter", func(t *testing.T) {
		initial := time.Second
		s := newBackoff(initial, WithJitter(0.1)) // 10% jitter

		for i := 0; i < 10; i++ {
			delay := s.NextDelay(1)
			assert.GreaterOrEqual(t, delay, 950*time.Millisecond) // initial - 5% jitter
			assert.Less(t, delay, 1050*time.Millisecond)          // initial + 5% jitter
		}
	})

	t.Run("zero or negative initial delay gives zero delay", func(t *testing.T) {
		for _, btype := range []BackoffType{BackoffConstant, BackoffLinear, BackoffExponential} {
			for _, jitter := range []float64{0, 0.1} {
				for _, initial := range []time.Duration{0, -time.Second} {
					s := newBackoff(initial, WithBackoffType(btype), WithJitter(jitter))
					for _, attempt := range []int{1, 2, 3, 100} {
						assert.Equal(t, time.Duration(0), s.NextDelay(attempt),
							"type %d, jitter %v, initial %v, attempt %d", btype, jitter, initial, attempt)
					}
				}
			}
		}
	})

	t.Run("jitter never exceeds max delay", func(t *testing.T) {
		s := newBackoff(time.Second, WithMaxDelay(10*time.Second)) // default 10% jitter
		for range 1000 {
			delay := s.NextDelay(10)
			assert.LessOrEqual(t, delay, 10*time.Second)
			assert.GreaterOrEqual(t, delay, 9500*time.Millisecond) // cap - 5% jitter
		}
	})

	t.Run("jitter factor clamped to 0-1", func(t *testing.T) {
		s := newBackoff(time.Second, WithJitter(3))
		assert.InDelta(t, 1.0, s.jitter, 0.0001)
		for range 1000 {
			delay := s.NextDelay(1)
			assert.GreaterOrEqual(t, delay, 500*time.Millisecond) // initial - 50% jitter
			assert.LessOrEqual(t, delay, 1500*time.Millisecond)   // initial + 50% jitter
		}

		s = newBackoff(time.Second, WithJitter(-1))
		assert.InDelta(t, 0.0, s.jitter, 0.0001)
		assert.Equal(t, time.Second, s.NextDelay(1))

		s = newBackoff(time.Second, WithJitter(math.NaN()))
		assert.InDelta(t, 0.0, s.jitter, 0.0001)
		assert.Equal(t, time.Second, s.NextDelay(1))
	})

	t.Run("all options", func(t *testing.T) {
		s := newBackoff(time.Second,
			WithBackoffType(BackoffLinear),
			WithMaxDelay(3*time.Second),
			WithJitter(0.2),
		)

		assert.Equal(t, time.Second, s.initial)
		assert.Equal(t, 3*time.Second, s.maxDelay)
		assert.Equal(t, BackoffLinear, s.btype)
		assert.InDelta(t, 0.2, s.jitter, 0.0001, "custom jitter")
	})

	t.Run("defaults", func(t *testing.T) {
		s := newBackoff(time.Second)
		assert.Equal(t, time.Second, s.initial)
		assert.Equal(t, 30*time.Second, s.maxDelay)
		assert.Equal(t, BackoffExponential, s.btype)
		assert.InDelta(t, 0.1, s.jitter, 0.0001, "default jitter")
	})
}
