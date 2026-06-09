# cloudnative-patterns

Go implementations of cloud-native patterns 

## [Circuit breaker](https://www.geeksforgeeks.org/system-design/what-is-circuit-breaker-pattern-in-microservices/)

Wrap a function that may fail. After N failures the breaker opens and returns fast without calling the dependency. After a cooldown it probes in half-open state, a success closes the circuit again.

```go
package main

import (
	"context"
	"errors"

	breaker "github.com/cloudnative-patterns/pkg/circuit_breaker"
)

func callDownstream(ctx context.Context) (string, error) {
	// call a remote service, database, etc.
	return "", errors.New("timeout")
}

func main() {
	isFailure := func(err error) bool { return err != nil }
	failuresThreshold := 3
	
	b := breaker.NewBreaker[string](isFailure, failuresThreshold)
	callWithBreaker := b.BreakerFn(callDownstream) 

	res, err := callWithBreaker(context.Background())
	if errors.Is(err, breaker.BreakerErrResponse) {
		// circuit is open — return a fallback response
		_ = res
	}
}
```

## Development

```bash
make build      # build e2e demo service
make test       # unit + e2e tests
```
