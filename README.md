# cloudnative-patterns

Go implementations of cloud-native patterns 

## Install

```bash
go get github.com/D-Andreev/cloudnative-patterns
```

## [Circuit breaker](https://www.geeksforgeeks.org/system-design/what-is-circuit-breaker-pattern-in-microservices/)

> Temporarily block access to a remote service or resource after failures reach a threshold, instead of repeatedly retrying an operation that's likely to fail. This approach handles faults that take varying amounts of time to recover from, lets the failing service recover, and improves the stability and resiliency of an application.

Wrap a function that may fail. After N failures the breaker opens and returns fast without calling the dependency. After a cooldown it probes in half-open state, a success closes the circuit again.

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
	isFailure := func(err error) bool { return err != nil }
	threshold := 3

	b := breaker.NewBreaker[string](isFailure, threshold)
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

	// Output:
	// call 1: downstream failed (timeout), state=closed
  // call 2: downstream failed (timeout), state=closed
  // call 3: downstream failed (timeout), state=open
  // call 4: circuit open — fast fail (service unavailable), state=open
  // call 5: circuit open — fast fail (service unavailable), state=open
}
```

Output
```sh

```

## Development

```bash
make build      # build e2e demo service
make test       # unit + e2e tests
```
