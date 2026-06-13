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

## Implemented patterns

- [x] [Circuit breaker](#circuit-breaker)
- [x] [Debounce](#debounce)
- [x] [Retry](#retry)
- [x] [Throttle](#throttle)
- [ ] [Timeout](#timeout)

## Install

```bash
go get github.com/D-Andreev/cloudnative-patterns
```

## Circuit breaker

> Temporarily block access to a remote service or resource after failures reach a threshold, instead of repeatedly retrying an operation that's likely to fail. This approach handles faults that take varying amounts of time to recover from, lets the failing service recover, and improves the stability and resiliency of an application.

### States

| State | Behavior |
|-------|----------|
| **Closed** | Normal operation. Every call reaches the downstream function. Failures counted by `IsFailure` accumulate; a success resets the count. When failures reach `Threshold`, the breaker opens. |
| **Open** | The dependency is treated as unavailable. Calls fail immediately with `BreakerErrResponse` without invoking the downstream function. After an open duration (see `OpenBackoff`), the breaker moves to half-open. |
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

type CallRequest struct{}

func callDownstream(ctx context.Context, _ CallRequest) (string, error) {
	return "", errors.New("timeout")
}

func main() {
	settings := breaker.Settings{
		IsFailure: func(err error) bool { return err != nil },
		Threshold: 3,
	}
	b, err := breaker.NewBreaker[CallRequest, string](settings)
	if err != nil {
		fmt.Println("Invalid settings", err)
		return
	}
	call := b.Wrap(callDownstream)

	for i := 1; i <= 5; i++ {
		_, err := call(context.Background(), CallRequest{})

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
| `OpenBackoff` | `OpenBackoff` | How long the circuit stays open before probing half-open. Optional; defaults to exponential backoff with a `2s` base and no cap. |

#### Open backoff strategies

`OpenBackoff` controls how long the circuit remains open. `d` is the number of failures past `Threshold` (0 on first open, 1 after a failed half-open probe, and so on).

| Strategy | Constant | Duration | Example (Base = 2s) |
|----------|----------|----------|---------------------|
| Exponential (default) | `OpenExponential` | `Base × 2^d` | 2s, 4s, 8s, 16s… |
| Fixed | `OpenFixed` | `Base` | 2s, 2s, 2s… |
| Linear | `OpenLinear` | `Base × (d + 1)` | 2s, 4s, 6s, 8s… |

Set `OpenBackoff.Max` to cap the computed duration. Omit `OpenBackoff` entirely to keep the default exponential strategy with a `2s` base.

```go
import (
    "time"

    breaker "github.com/D-Andreev/cloudnative-patterns/pkg/circuit_breaker"
)

settings := breaker.Settings{
    IsFailure: func(err error) bool { return err != nil },
    Threshold: 3,
    OpenBackoff: breaker.OpenBackoff{
        Strategy: breaker.OpenFixed,
        Base:     30 * time.Second,
    },
}
```

## Debounce

> Coalesce a burst of calls into a single execution. Within a configurable window, either the **first** call runs and later callers share its result, or each new call resets the timer and only the **last** call in a quiet period runs.

### Modes

| Mode | Constant | Behavior |
|------|----------|----------|
| **Function first** | `FunctionFirst` | The first call in a window runs the inner function; subsequent calls within `Duration` return the cached result (or wait if the first call is still in flight). |
| **Function last** | `FunctionLast` | Each call resets a timer. The inner function runs once after `Duration` passes with no new calls. Superseded callers receive `context.Canceled`. |

### Usage

#### Function first

Use when the first event in a burst should win (e.g. submit on first click, share one in-flight result with concurrent waiters).

```go
package main

import (
	"context"
	"fmt"
	"time"

	debounce "github.com/D-Andreev/cloudnative-patterns/pkg/debounce"
)

type FetchRequest struct{}

func fetch(ctx context.Context, _ FetchRequest) (string, error) {
	return "ok", nil
}

func main() {
	d, err := debounce.NewDebounce[FetchRequest, string](debounce.Settings{
		DebounceType: debounce.FunctionFirst,
		Duration:     1 * time.Second,
	})
	if err != nil {
		fmt.Println("invalid settings:", err)
		return
	}

	call := d.First(fetch)

	res1, _ := call(context.Background(), FetchRequest{}) // runs fetch
	res2, _ := call(context.Background(), FetchRequest{}) // cached
	res3, _ := call(context.Background(), FetchRequest{}) // cached

	fmt.Println(res1, res2, res3) // ok ok ok
}
```

#### Function last

Use when only the **latest input** matters (e.g. search-as-you-type). Each `call()` resets a timer; the inner function runs **once** after `Duration` passes with no new calls. Earlier waiters get `context.Canceled`.

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	debounce "github.com/D-Andreev/cloudnative-patterns/pkg/debounce"
)

type SearchRequest struct {
	Query string
}

func search(ctx context.Context, req SearchRequest) (string, error) {
	select {
	case <-time.After(50 * time.Millisecond):
		return "results for " + req.Query, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func main() {
	d, err := debounce.NewDebounce[SearchRequest, string](debounce.Settings{
		DebounceType: debounce.FunctionLast,
		Duration:     200 * time.Millisecond,
	})
	if err != nil {
		fmt.Println("invalid settings:", err)
		return
	}

	call := d.Last(search)

	// Rapid keystrokes — each call resets the 200ms quiet-period timer.
	go func() {
		_, err := call(context.Background(), SearchRequest{Query: "a"})
		if errors.Is(err, context.Canceled) {
			fmt.Println("a: superseded")
		}
	}()
	time.Sleep(20 * time.Millisecond)

	go func() {
		_, err := call(context.Background(), SearchRequest{Query: "ab"})
		if errors.Is(err, context.Canceled) {
			fmt.Println("ab: superseded")
		}
	}()
	time.Sleep(20 * time.Millisecond)

	res, err := call(context.Background(), SearchRequest{Query: "abc"}) // waits 200ms quiet, then runs search once
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(res)
}
```

Each `call()` **blocks** until it wins (timer fired + inner fn done) or is superseded. The winning caller waits roughly **`Duration` + inner work time** (~250ms here), not the 20ms gaps between keystrokes.

**Output**

```sh
a: superseded
ab: superseded
results for abc
```

Only one `search` runs, with the **last** query (`"abc"`). Superseded callers should respect `ctx` in long-running work.

### Settings

| Field | Type | Description |
|-------|------|-------------|
| `Duration` | `time.Duration` | Debounce window. Defaults to `300ms` when unset or zero. Must be at least `1ms`. |
| `DebounceType` | `DebounceType` | `FunctionFirst` (0) or `FunctionLast` (1). Required. |

## Retry

> Re-invoke a failing operation after a delay. Useful for transient errors (network blips, rate limits) without hammering a dependency. Stops on success, when `MaxFailures` retries are exhausted, or when the context is canceled during a wait.

### Usage

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	retry "github.com/D-Andreev/cloudnative-patterns/pkg/retry"
)

type RetryRequest struct{}

func main() {
	r, err := retry.NewRetry[RetryRequest, string](retry.Settings{
		Delay:       100 * time.Millisecond,
		MaxFailures: 3,
	})
	if err != nil {
		fmt.Println("invalid settings:", err)
		return
	}

	attempts := 0
	call := r.Wrap(func(context.Context, RetryRequest) (string, error) {
		attempts++
		if attempts < 3 {
			return "", errors.New("temporary")
		}
		return "ok", nil
	})

	res, err := call(context.Background(), RetryRequest{})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("%s after %d attempts\n", res, attempts)
}
```

**Output**

```sh
ok after 3 attempts
```

The first two failures wait `Delay` and retry. On success the loop returns immediately — it does not use all `MaxFailures` slots. With persistent failure, `MaxFailures: 3` allows up to **four** invocations (initial attempt plus three retries).

### Settings

| Field | Type | Description |
|-------|------|-------------|
| `Delay` | `time.Duration` | Wait between retry attempts after a failed invocation. |
| `MaxFailures` | `int` | Maximum retries after the first failed attempt. `0` means no retries. |

## Throttle

> Limit how often an effector runs using a token bucket. You get up to `Maximum` calls per window; tokens refill by `Refill` every `Duration`. When no token is available, behavior depends on which wrapper you choose.

### Modes

| Mode | Method | When throttled |
|------|--------|----------------|
| **Error** | `WithError` | Returns `too many calls` and does not run the effector. |
| **Replay** | `WithReplay` | Returns the last successful result without running the effector. |
| **Queue** | `WithQueue` | Enqueues the request and returns `too many calls`; a later caller with a token processes queued requests (FIFO). |

### Usage

#### With error

Use when callers should handle rate limiting explicitly (for example return HTTP 429).

```go
package main

import (
	"context"
	"fmt"
	"time"

	throttle "github.com/D-Andreev/cloudnative-patterns/pkg/throttle"
)

func main() {
	th, err := throttle.NewThrottle[struct{}, string](throttle.Settings{
		Maximum:  2,
		Duration: time.Second,
		Refill:   2,
	})
	if err != nil {
		fmt.Println("invalid settings:", err)
		return
	}

	call := th.WithError(func(context.Context, struct{}) (string, error) {
		return "ok", nil
	})

	for i := 1; i <= 3; i++ {
		res, err := call(context.Background(), struct{}{})
		if err != nil {
			fmt.Printf("call %d: throttled (%v)\n", i, err)
			continue
		}
		fmt.Printf("call %d: %s\n", i, res)
	}
}
```

**Output**

```sh
call 1: ok
call 2: ok
call 3: throttled (too many calls)
```

#### With replay

Use when stale data is acceptable.

```go
package main

import (
	"context"
	"fmt"
	"time"

	throttle "github.com/D-Andreev/cloudnative-patterns/pkg/throttle"
)

func main() {
	th, err := throttle.NewThrottle[struct{}, string](throttle.Settings{
		Maximum:  1,
		Duration: time.Second,
		Refill:   1,
	})
	if err != nil {
		fmt.Println("invalid settings:", err)
		return
	}

	attempts := 0
	call := th.WithReplay(func(context.Context, struct{}) (string, error) {
		attempts++
		return fmt.Sprintf("value-%d", attempts), nil
	})

	res1, _ := call(context.Background(), struct{}{})
	res2, _ := call(context.Background(), struct{}{})

	fmt.Println(res1)
	fmt.Println(res2)
	fmt.Println("effector calls:", attempts)
}
```

**Output**

```sh
value-1
value-1
effector calls: 1
```

#### With queue

Use when throttled requests should eventually run, but callers must tolerate an immediate `too many calls` response.

```go
package main

import (
	"context"
	"fmt"
	"time"

	throttle "github.com/D-Andreev/cloudnative-patterns/pkg/throttle"
)

type QueueRequest struct {
	Label string
}

func main() {
	th, err := throttle.NewThrottle[QueueRequest, string](throttle.Settings{
		Maximum:  1,
		Duration: 100 * time.Millisecond,
		Refill:   1,
	})
	if err != nil {
		fmt.Println("invalid settings:", err)
		return
	}

	call := th.WithQueue(func(_ context.Context, req QueueRequest) (string, error) {
		return req.Label, nil
	})

	res, err := call(context.Background(), QueueRequest{Label: "first"})
	fmt.Println("first:", res, "err:", err)

	_, err = call(context.Background(), QueueRequest{Label: "queued"})
	fmt.Println("queued:", err)

	time.Sleep(150 * time.Millisecond)

	res, err = call(context.Background(), QueueRequest{Label: "caller"})
	fmt.Println("after refill:", res, "err:", err)
}
```

**Output**

```sh
first: first err: <nil>
queued: too many calls
after refill: queued err: <nil>
```

The `"queued"` request was stored when throttled and executed when a later caller had a token.

### Settings

| Field | Type | Description |
|-------|------|-------------|
| `Maximum` | `uint` | Bucket capacity and initial token count. Must be at least `1`. |
| `Duration` | `time.Duration` | Refill interval. Must be at least `1ms`. |
| `Refill` | `uint` | Tokens added every `Duration`, capped at `Maximum`. Must be at least `1`. |

## Timeout

Coming soon.
