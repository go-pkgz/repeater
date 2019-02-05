package repeater

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/go-pkgz/repeater/strategy"
	"github.com/stretchr/testify/assert"
)

func TestRepeatFixed(t *testing.T) {
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

func TestRepeatFixedFailed(t *testing.T) {
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

func TestRepeatFixedCanceled(t *testing.T) {
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
	assert.True(t, err.Error() == "context deadline exceeded" || err.Error() == "some error")
	assert.Equal(t, 2, called)
	assert.True(t, time.Since(st) >= time.Millisecond*60 && time.Since(st) < time.Millisecond*70)
}

func TestRepeatFixedCriticalError(t *testing.T) {
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
	assert.Equal(t, criticalErr, err)
	assert.Equal(t, 5, called, "called 5 times")
}

func TestRepeatBackoff(t *testing.T) {
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
	assert.Equal(t, 6, called, "called 5 times")

	t.Log(time.Since(st)) // 100 + 100 * 2^1 + 100 * 2^2 + 100 * 2^3 + 100 * 2^4 = 3100
	assert.True(t, time.Since(st) >= 3100*time.Millisecond && time.Since(st) < 4100*time.Millisecond,
		fmt.Sprintf("took %s", time.Since(st)))
}

func TestRepeatBackoffFailed(t *testing.T) {
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
	assert.Equal(t, e, err)
	assert.Equal(t, 1, called, "called 1 times")
}

func TestRepeatBackoffCanceled(t *testing.T) {
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
	assert.True(t, err.Error() == "context deadline exceeded" || err.Error() == "some error")
	assert.Equal(t, 6, called)
}
func TestRepeatOnce(t *testing.T) {
	e := errors.New("some error")
	called := 0
	fun := func() error {
		called++
		return e
	}

	err := New(&strategy.Once{}).Do(context.Background(), fun)
	assert.Equal(t, e, err)
	assert.Equal(t, 1, called, "called 1 time")

	called = 0
	e = nil
	err = New(&strategy.Once{}).Do(context.Background(), fun)
	assert.Nil(t, err)
	assert.Equal(t, 1, called, "called 1 time")
}
