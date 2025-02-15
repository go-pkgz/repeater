# Repeater

[![Build Status](https://github.com/go-pkgz/repeater/workflows/build/badge.svg)](https://github.com/go-pkgz/repeater/actions) [![Go Report Card](https://goreportcard.com/badge/github.com/go-pkgz/repeater)](https://goreportcard.com/report/github.com/go-pkgz/repeater) [![Coverage Status](https://coveralls.io/repos/github/go-pkgz/repeater/badge.svg?branch=master)](https://coveralls.io/github/go-pkgz/repeater?branch=master)

Package repeater implements a functional mechanism to repeat operations with different retry strategies.

## Install and update

`go get -u github.com/go-pkgz/repeater`

## Usage

### Basic Example with Exponential Backoff

```go
// create repeater with exponential backoff
r := repeater.NewBackoff(5, time.Second) // 5 attempts starting with 1s delay

err := r.Do(ctx, func() error {
// do something that may fail
return nil
})
```

### Fixed Delay with Critical Error

```go
// create repeater with fixed delay
r := repeater.NewFixed(3, 100*time.Millisecond)

criticalErr := errors.New("critical error")

err := r.Do(ctx, func() error {
// do something that may fail
return fmt.Errorf("temp error")
}, criticalErr) // will stop immediately if criticalErr returned
```

### Custom Backoff Strategy

```go
r := repeater.NewBackoff(5, time.Second,
repeater.WithMaxDelay(10*time.Second),
repeater.WithBackoffType(repeater.BackoffLinear),
repeater.WithJitter(0.1),
)

ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

err := r.Do(ctx, func() error {
// do something that may fail
return nil
})
```

### Stop on Any Error

```go
r := repeater.NewFixed(3, time.Millisecond)

err := r.Do(ctx, func() error {
return errors.New("some error")
}, repeater.ErrAny)  // will stop on any error
```

## Strategies

The package provides several retry strategies:

1. **Fixed Delay** - each retry happens after a fixed time interval
2. **Backoff** - delay between retries increases according to the chosen algorithm:
   - Constant - same delay between attempts
   - Linear - delay increases linearly
   - Exponential - delay doubles with each attempt

Backoff strategy can be customized with:
- Maximum delay cap
- Jitter to prevent thundering herd
- Different backoff types (constant/linear/exponential)

### Custom Strategies

You can implement your own retry strategy by implementing the Strategy interface:

```go
type Strategy interface {
    // NextDelay returns delay for the next attempt
    // attempt starts from 1
    NextDelay(attempt int) time.Duration
}
```

Example of a custom strategy that increases delay by a custom factor:

```go
// CustomStrategy implements Strategy with custom factor-based delays
type CustomStrategy struct {
    Initial time.Duration
    Factor  float64
}

func (s CustomStrategy) NextDelay(attempt int) time.Duration {
    if attempt <= 0 {
        return 0
    }
    delay := time.Duration(float64(s.Initial) * math.Pow(s.Factor, float64(attempt-1)))
    return delay
}

// Usage
strategy := &CustomStrategy{Initial: time.Second, Factor: 1.5}
r := repeater.NewWithStrategy(5, strategy)
err := r.Do(ctx, func() error {
    // attempts will be delayed by: 1s, 1.5s, 2.25s, 3.37s, 5.06s
    return nil
})
```

## Options

For backoff strategy, several options are available:

```go
WithMaxDelay(time.Duration)   // set maximum delay between retries
WithBackoffType(BackoffType)  // set backoff type (constant/linear/exponential)
WithJitter(float64)           // add randomness to delays (0-1.0)
```

## Error Handling

- Stops on context cancellation
- Can stop on specific errors (pass them as additional parameters to Do)
- Special `ErrAny` to stop on any error
- Returns last error if all attempts fail
