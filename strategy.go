package repeater

import (
	"math"
	"math/rand"
	"time"
)

// maxDuration is the longest delay a time.Duration can hold
const maxDuration = time.Duration(math.MaxInt64)

// Strategy defines how to calculate delays between retries
type Strategy interface {
	// NextDelay returns the delay after the given failed attempt, attempt starts from 1.
	// It is not called after the last attempt.
	NextDelay(attempt int) time.Duration
}

// FixedDelay implements fixed time delay between attempts
type FixedDelay struct {
	Delay time.Duration
}

// NewFixedDelay creates a new FixedDelay strategy
func NewFixedDelay(delay time.Duration) FixedDelay {
	return FixedDelay{Delay: delay}
}

// NextDelay returns fixed delay
func (s FixedDelay) NextDelay(_ int) time.Duration {
	return s.Delay
}

// BackoffType represents the backoff strategy type
type BackoffType int

const (
	// BackoffConstant keeps delays the same between attempts
	BackoffConstant BackoffType = iota
	// BackoffLinear increases delays linearly between attempts
	BackoffLinear
	// BackoffExponential increases delays exponentially between attempts
	BackoffExponential
)

// backoff implements various backoff strategies with optional jitter
type backoff struct {
	initial  time.Duration
	maxDelay time.Duration
	btype    BackoffType
	jitter   float64
}

type backoffOption func(*backoff)

// WithMaxDelay sets maximum delay for the backoff strategy, jitter included; zero or a negative value means no maximum
func WithMaxDelay(d time.Duration) backoffOption { //nolint:revive // unexported type is used in the same package
	return func(b *backoff) {
		b.maxDelay = d
	}
}

// WithBackoffType sets backoff type for the strategy
func WithBackoffType(t BackoffType) backoffOption { //nolint:revive // unexported type is used in the same package
	return func(b *backoff) {
		b.btype = t
	}
}

// WithJitter sets jitter factor for the backoff strategy. A factor f spreads each delay
// uniformly over plus or minus f/2 of it. The maximum delay applies both before and after
// the jitter, so a capped delay only gets shorter. Values below 0 are treated as 0 and above 1 as 1.
func WithJitter(factor float64) backoffOption { //nolint:revive // unexported type is used in the same package
	return func(b *backoff) {
		switch {
		case factor > 1:
			b.jitter = 1
		case factor > 0:
			b.jitter = factor
		default: // negative, zero or NaN
			b.jitter = 0
		}
	}
}

func newBackoff(initial time.Duration, opts ...backoffOption) *backoff {
	b := &backoff{
		initial:  initial,
		maxDelay: 30 * time.Second,
		btype:    BackoffExponential,
		jitter:   0.1,
	}

	for _, opt := range opts {
		opt(b)
	}

	return b
}

// NextDelay returns delay for the next attempt
func (s backoff) NextDelay(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}

	// computed in float64, so a large attempt number saturates instead of overflowing time.Duration
	var delay float64
	switch s.btype {
	case BackoffConstant:
		delay = float64(s.initial)
	case BackoffLinear:
		delay = float64(s.initial) * float64(attempt)
	case BackoffExponential:
		// an exponent of 63 or more saturates any positive initial delay, and a much larger one overflows inside math.Ldexp
		delay = math.Ldexp(float64(s.initial), min(attempt-1, 64))
	}

	limit := maxDuration
	if s.maxDelay > 0 {
		limit = s.maxDelay
	}
	delay = min(delay, float64(limit))

	if s.jitter > 0 {
		jitter := delay * s.jitter
		delay += rand.Float64()*jitter - jitter/2 //nolint:gosec // no need for secure random here
	}

	// jitter can push a delay above the cap, so the cap is applied once more
	return toDuration(delay, limit)
}

// toDuration converts d to a time.Duration within [0, limit]
func toDuration(d float64, limit time.Duration) time.Duration {
	if d >= float64(limit) {
		return limit
	}
	if d <= 0 {
		return 0
	}
	return time.Duration(d)
}
