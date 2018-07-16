package repeater

import (
	"fmt"
	"testing"
	"time"

	"errors"

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

	err := NewDefault(10, time.Millisecond).Do(fun)
	assert.Nil(t, err, "should be ok")
	assert.Equal(t, 5, called, "called 5 times")

	called = 0
	err = NewDefault(4, time.Millisecond).Do(fun)
	assert.NotNil(t, err, "should be err")
	assert.Equal(t, 4, called, "called 4 times")

	called = 0
	err = NewDefault(0, time.Millisecond).Do(fun)
	assert.NotNil(t, err, "should be err")
	assert.Equal(t, 1, called, "called 1 time")

	called = 0
	err = NewDefault(5, time.Millisecond).Do(fun, e)
	assert.NotNil(t, err, "should be err, fail on e right away")
	assert.Equal(t, 1, called, "called 1 time")

	called = 0
	err = NewDefault(5, time.Millisecond).Do(fun, errors.New("unknown error"))
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

	err := NewDefault(10, time.Millisecond).Do(fun)
	assert.Equal(t, e, err)
	assert.Equal(t, 10, called, "called 10 times")

	called = 0
	err = NewDefault(1, time.Millisecond).Do(fun)
	assert.Equal(t, e, err)
	assert.Equal(t, 1, called, "called 1 times")
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

	err := NewDefault(10, time.Millisecond).Do(fun, criticalErr)
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
	err := New(strategy.NewBackoff(10, 2, false)).Do(fun)
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

	err := New(strategy.NewBackoff(5, 2, true)).Do(fun)
	assert.Equal(t, e, err)
	assert.Equal(t, 5, called, "called 5 times")

	called = 0
	err = New(strategy.NewBackoff(1, 2, true)).Do(fun)
	assert.Equal(t, e, err)
	assert.Equal(t, 1, called, "called 1 times")
}

func TestRepeatOnce(t *testing.T) {
	e := errors.New("some error")
	called := 0
	fun := func() error {
		called++
		return e
	}

	err := New(strategy.NewOnce()).Do(fun)
	assert.Equal(t, e, err)
	assert.Equal(t, 1, called, "called 1 time")

	called = 0
	e = nil
	err = New(strategy.NewOnce()).Do(fun)
	assert.Nil(t, err)
	assert.Equal(t, 1, called, "called 1 time")
}
