# cloudnative-patterns
<p align="center">
  <a href="https://github.com/D-Andreev/cloudnative-patterns/actions/workflows/ci.yml">
    <img src="https://github.com/D-Andreev/cloudnative-patterns/actions/workflows/ci.yml/badge.svg" alt="CI">
  </a>
  <a href="https://godoc.org/github.com/D-Andreev/cloudnative-patterns">
    <img src="https://godoc.org/github.com/D-Andreev/cloudnative-patterns?status.svg" alt="GoDoc">
  </a>
</p>

Implementations of various cloud native patterns with Go.

## Install

```bash
go get github.com/D-Andreev/cloudnative-patterns
```

## [Circuit breaker](https://www.geeksforgeeks.org/system-design/what-is-circuit-breaker-pattern-in-microservices/)

> Temporarily block access to a remote service or resource after failures reach a threshold, instead of repeatedly retrying an operation that's likely to fail. This approach handles faults that take varying amounts of time to recover from, lets the failing service recover, and improves the stability and resiliency of an application.

### States

| State | Behavior |
|-------|----------|
| **Closed** | Normal operation. Every call reaches the downstream function. Failures counted by `IsFailure` accumulate; a success resets the count. When failures reach `Threshold`, the breaker opens. |
| **Open** | The dependency is treated as unavailable. Calls fail immediately with `BreakerErrResponse` without invoking the downstream function. After a cooldown, the breaker moves to half-open. |
| **Half-open** | A single probe call is allowed to test recovery. If it succeeds, the breaker closes and the failure count resets. If it fails, the breaker opens again with a longer cooldown. Concurrent callers while a probe is in flight also receive `BreakerErrResponse`. |


### Usage
```go
package main

import (
	"context"
	"errors"
	"fmt"

	breaker "github.com/D-Andreev/cloudnative-patterns/pkg/circuit_breaker"
)

func callDownstream(ctx context.Context) (string, error) {
	return "", errors.New("timeout")
}

func main() {
	settings := breaker.Settings{
		IsFailure: func(err error) bool { return err != nil },
		Threshold: 3,
	}
	b, err := breaker.NewBreaker[string](settings)
	if err != nil {
		fmt.Println("Invalid settings", err)
		return
	}
	call := b.BreakerFn(callDownstream)

	for i := 1; i <= 5; i++ {
		_, err := call(context.Background())

		switch {
		case errors.Is(err, breaker.BreakerErrResponse):
			fmt.Printf("call %d: circuit open — fast fail (%v), state=%s\n", i, err, b.State())
		case err != nil:
			fmt.Printf("call %d: downstream failed (%v), state=%s\n", i, err, b.State())
		default:
			fmt.Printf("call %d: success, state=%s\n", i, b.State())
		}
	}
}
```

Output
```sh
call 1: downstream failed (timeout), state=closed
call 2: downstream failed (timeout), state=closed
call 3: downstream failed (timeout), state=open
call 4: circuit open — fast fail (service unavailable), state=open
call 5: circuit open — fast fail (service unavailable), state=open
```

### Settings

| Field | Type | Description |
|-------|------|-------------|
| `IsFailure` | `func(error) bool` | Called after each downstream invocation. Return `true` if the error should count toward opening the circuit (e.g. timeouts, 5xx). Return `false` for errors that should not trip the breaker (e.g. client validation errors). Required, must not be nil. |
| `Threshold` | `int` | Number of consecutive failures (as determined by `IsFailure`) before the circuit opens. Must be at least `1`. |
