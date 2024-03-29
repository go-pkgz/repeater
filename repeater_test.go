package repeater

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/go-pkgz/repeater/strategy"
)

func TestRepeaterFixed(t *testing.T) {
	e := errors.New("some error")
	called := 0
	fun := func() error {
		called++
		if called == 5 { // only 5th call returns ok
			return nil
		}
		return e
	}

	st := time.Now()
	err := NewDefault(10, time.Millisecond*10).Do(context.Background(), fun)
	assert.Nil(t, err, "should be ok")
	assert.Equal(t, 5, called, "called 5 times")
	assert.True(t, time.Since(st) >= 40*time.Millisecond)
	called = 0
	err = NewDefault(4, time.Millisecond).Do(context.Background(), fun)
	assert.NotNil(t, err, "should be err")
	assert.Equal(t, 4, called, "called 4 times")

	called = 0
	err = NewDefault(0, time.Millisecond).Do(context.Background(), fun)
	assert.NotNil(t, err, "should be err")
	assert.Equal(t, 1, called, "called 1 time")

	called = 0
	err = NewDefault(5, time.Millisecond).Do(context.Background(), fun, e)
	assert.NotNil(t, err, "should be err, fail on e right away")
	assert.Equal(t, 1, called, "called 1 time")

	called = 0
	err = NewDefault(5, time.Millisecond).Do(context.Background(), fun, errors.New("unknown error"))
	assert.Nil(t, err, "err not matched to fail-on")
	assert.Equal(t, 5, called, "called 5 time")
}

func TestRepeaterFixedFailed(t *testing.T) {
	e := errors.New("some error")
	called := 0
	fun := func() error {
		called++
		return e
	}

	err := NewDefault(10, time.Millisecond).Do(context.Background(), fun)
	assert.Equal(t, e, err)
	assert.Equal(t, 10, called, "called 10 times")

	called = 0
	err = NewDefault(1, time.Millisecond).Do(context.Background(), fun)
	assert.Equal(t, e, err)
	assert.Equal(t, 1, called, "called 1 times")
}

func TestRepeaterFixedCanceled(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Millisecond*60)
	defer cancel()

	called := 0
	fun := func() error {
		called++
		return errors.New("some error")
	}

	st := time.Now()
	err := NewDefault(10, time.Millisecond*50).Do(ctx, fun)
	require.NotNil(t, err)
	assert.True(t, err.Error() == "context deadline exceeded" || err.Error() == "some error")
	assert.Equal(t, 2, called)
	assert.True(t, time.Since(st) >= time.Millisecond*60 && time.Since(st) < time.Millisecond*70)
}

func TestRepeaterFixedCriticalError(t *testing.T) {
	criticalErr := errors.New("critical error")

	called := 0
	fun := func() error {
		called++
		if called == 5 {
			return criticalErr
		}
		return errors.New("some error")
	}

	err := NewDefault(10, time.Millisecond).Do(context.Background(), fun, criticalErr)
	assert.ErrorIs(t, err, criticalErr)
	assert.Equal(t, 5, called, "called 5 times")
}

func TestRepeaterFixedCriticalErrorWrap(t *testing.T) {
	criticalErr := errors.New("critical error")

	called := 0
	fun := func() error {
		called++
		if called == 5 {
			return fmt.Errorf("wrap err: %w", criticalErr)
		}
		return errors.New("some error")
	}

	err := NewDefault(10, time.Millisecond).Do(context.Background(), fun, criticalErr)
	assert.ErrorIs(t, err, criticalErr)
	assert.Equal(t, 5, called, "called 5 times")
}

func TestRepeaterBackoff(t *testing.T) {
	e := errors.New("some error")
	called := 0
	fun := func() error {
		called++
		if called == 6 { // only 6th call returns ok, 5 delays
			return nil
		}
		return e
	}

	st := time.Now()
	strtg := strategy.Backoff{
		Duration: 100 * time.Millisecond,
		Repeats:  10,
		Factor:   2,
		Jitter:   false,
	}

	err := New(&strtg).Do(context.Background(), fun)
	assert.Nil(t, err, "should be ok")
	assert.Equal(t, 6, called, "called 6 times")

	//nolint:gocritic
	t.Log(time.Since(st)) // 100 + 100 * 2^1 + 100 * 2^2 + 100 * 2^3 + 100 * 2^4 = 3100
	assert.True(t, time.Since(st) >= 3100*time.Millisecond && time.Since(st) < 4100*time.Millisecond,
		fmt.Sprintf("took %s", time.Since(st)))
}

func TestRepeaterBackoffFailed(t *testing.T) {
	e := errors.New("some error")
	called := 0
	fun := func() error {
		called++
		return e
	}

	strtg := strategy.Backoff{
		Duration: 0,
		Repeats:  5,
		Factor:   2,
		Jitter:   true,
	}
	err := New(&strtg).Do(context.Background(), fun)
	assert.Equal(t, e, err)
	assert.Equal(t, 5, called, "called 5 times")

	strtg = strategy.Backoff{
		Duration: 100 * time.Millisecond,
		Repeats:  1,
		Factor:   2,
		Jitter:   true,
	}
	called = 0
	err = New(&strtg).Do(context.Background(), fun)
	assert.ErrorIs(t, err, e)
	assert.Equal(t, 1, called, "called 1 times")
}

func TestRepeaterBackoffCanceled(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Millisecond*450)
	defer cancel()

	called := 0
	fun := func() error {
		called++
		return errors.New("some error")
	}

	strtg := strategy.Backoff{
		Duration: 10 * time.Millisecond,
		Repeats:  100,
		Factor:   2,
		Jitter:   false,
	}

	err := New(&strtg).Do(ctx, fun)
	require.Error(t, err)
	assert.True(t, err.Error() == "context deadline exceeded" || err.Error() == "some error")
	assert.Equal(t, 6, called)
}

func TestRepeaterOnce(t *testing.T) {
	e := errors.New("some error")
	called := 0
	fun := func() error {
		called++
		return e
	}

	err := New(&strategy.Once{}).Do(context.Background(), fun)
	assert.ErrorIs(t, err, e)
	assert.Equal(t, 1, called, "called 1 time")

	called = 0
	e = nil
	err = New(&strategy.Once{}).Do(context.Background(), fun)
	assert.NoError(t, err)
	assert.Equal(t, 1, called, "called 1 time")
}

func TestRepeaterNil(t *testing.T) {
	e := errors.New("some error")
	called := 0
	fun := func() error {
		called++
		if called == 5 { // only 5th call returns ok
			return nil
		}
		return e
	}

	r := New(nil)
	r.Strategy.(*strategy.FixedDelay).Delay = 10 * time.Millisecond
	err := r.Do(context.Background(), fun)
	assert.Nil(t, err, "should be ok")
	assert.Equal(t, 5, called, "called 5 times")
}

func TestRepeaterMemoryLeakFixed(t *testing.T) {
	rep := func() {
		called := 0
		fun := func() error {
			time.Sleep(5 * time.Millisecond) // simulate slow call
			called++
			if called == 5 { // only 5th call returns ok
				return nil
			}
			return errors.New("some error")
		}
		w := New(&strategy.FixedDelay{Delay: 1 * time.Millisecond, Repeats: 10})
		err := w.Do(context.Background(), fun)
		require.NoError(t, err)
		assert.Equal(t, 5, called)
	}

	before := runtime.NumGoroutine()
	var num []int
	for i := 0; i < 25; i++ {
		rep()
		num = append(num, runtime.NumGoroutine())
	}
	time.Sleep(10 * time.Millisecond) // allow GC some time to deal with garbage
	after := runtime.NumGoroutine()
	require.False(t, after > before, "goroutines leak: %+v, before:%d, after:%d", num, before, after)
}

func TestRepeaterMemoryLeakBackOff(t *testing.T) {
	rep := func() {
		called := 0
		fun := func() error {
			time.Sleep(5 * time.Millisecond) // simulate slow call
			called++
			if called == 5 { // only 5th call returns ok
				return nil
			}
			return errors.New("some error")
		}
		w := New(&strategy.Backoff{Repeats: 10, Duration: 1 * time.Millisecond, Factor: 1.5})
		err := w.Do(context.Background(), fun)
		require.NoError(t, err)
		assert.Equal(t, 5, called)
	}

	before := runtime.NumGoroutine()
	var num []int
	for i := 0; i < 25; i++ {
		rep()
		num = append(num, runtime.NumGoroutine())
	}
	time.Sleep(10 * time.Millisecond) // allow GC some time to deal with garbage
	after := runtime.NumGoroutine()
	require.False(t, after > before, "goroutines leak: %+v, before:%d, after:%d", num, before, after)
}
