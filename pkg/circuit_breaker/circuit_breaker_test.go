package circuitbreaker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCircuitBreakerInvalidSettings(t *testing.T) {
	isFailure := func(err error) bool { return err != nil }

	testCases := []struct {
		name     string
		settings Settings
		wantErr  bool
	}{
		{
			name:     "empty settings",
			settings: Settings{},
			wantErr:  true,
		},
		{
			name: "nil IsFailure",
			settings: Settings{
				IsFailure: nil,
				Threshold: 3,
			},
			wantErr: true,
		},
		{
			name: "zero threshold",
			settings: Settings{
				IsFailure: isFailure,
				Threshold: 0,
			},
			wantErr: true,
		},
		{
			name: "valid settings",
			settings: Settings{
				IsFailure: isFailure,
				Threshold: 3,
			},
			wantErr: false,
		},
		{
			name: "invalid open backoff strategy",
			settings: Settings{
				IsFailure: isFailure,
				Threshold: 3,
				OpenBackoff: OpenBackoff{
					Strategy: OpenStrategy(99),
					Base:     time.Second,
				},
			},
			wantErr: true,
		},
		{
			name: "open backoff max less than base",
			settings: Settings{
				IsFailure: isFailure,
				Threshold: 3,
				OpenBackoff: OpenBackoff{
					Strategy: OpenExponential,
					Base:     5 * time.Second,
					Max:      time.Second,
				},
			},
			wantErr: true,
		},
		{
			name: "valid open backoff settings",
			settings: Settings{
				IsFailure: isFailure,
				Threshold: 3,
				OpenBackoff: OpenBackoff{
					Strategy: OpenFixed,
					Base:     10 * time.Second,
				},
			},
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := NewBreaker[string](tc.settings)
			if tc.wantErr {
				assert.Error(t, err)
				assert.Nil(t, b)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, b)
			assert.Equal(t, Closed, b.State())
		})
	}
}

func TestCircuitBreaker(t *testing.T) {
	var circuitSuccessResp = "success"
	var circuitErrorResp = errors.New("circuit error")
	var breakerErrResp = errors.New("service unavailable")
	testCases := []struct {
		name                       string
		callBreakerTimes           int
		expectedCircuitCalledTimes int
		expectedResults            []string
		expectedErrs               []error
		circuitReturnErrAfter      int
		circuitReturnSuccessAfter  int
		threshold                  int
		pauseAfterInocationN       int
	}{
		{
			name:                       "Breaker closed state, no failures",
			callBreakerTimes:           5,
			expectedCircuitCalledTimes: 5,
			expectedResults:            []string{"success", "success", "success", "success", "success"},
			expectedErrs:               make([]error, 5),
			threshold:                  3,
			pauseAfterInocationN:       -1,
		},
		{
			name:                       "Breaker opens the circuit after 3 failuires",
			callBreakerTimes:           7,
			expectedCircuitCalledTimes: 6,
			expectedResults:            []string{circuitSuccessResp, circuitSuccessResp, circuitSuccessResp, "", "", "", ""},
			expectedErrs:               []error{nil, nil, nil, circuitErrorResp, circuitErrorResp, circuitErrorResp, breakerErrResp},
			circuitReturnErrAfter:      3,
			threshold:                  3,
			pauseAfterInocationN:       -1,
		},
		{
			name:                       "Breaker opens the circuit and then goes into half open state, request fails and switches to open state again",
			callBreakerTimes:           10,
			expectedCircuitCalledTimes: 7,
			expectedResults:            []string{circuitSuccessResp, circuitSuccessResp, circuitSuccessResp, "", "", "", "", "", "", ""},
			expectedErrs:               []error{nil, nil, nil, circuitErrorResp, circuitErrorResp, circuitErrorResp, breakerErrResp, breakerErrResp, circuitErrorResp, breakerErrResp},
			circuitReturnErrAfter:      3,
			threshold:                  3,
			pauseAfterInocationN:       8,
		},
		{
			name:                       "Breaker half open state then after success response returns to closed",
			callBreakerTimes:           11,
			expectedCircuitCalledTimes: 9,
			expectedResults:            []string{circuitSuccessResp, circuitSuccessResp, circuitSuccessResp, "", "", "", "", "", circuitSuccessResp, circuitSuccessResp, circuitSuccessResp},
			expectedErrs:               []error{nil, nil, nil, circuitErrorResp, circuitErrorResp, circuitErrorResp, breakerErrResp, breakerErrResp, nil, nil, nil},
			circuitReturnErrAfter:      3,
			circuitReturnSuccessAfter:  6,
			threshold:                  3,
			pauseAfterInocationN:       8,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			circuitCalledTimes := 0
			circuitFn := func(ctx context.Context) (string, error) {
				if circuitCalledTimes >= tc.circuitReturnErrAfter && tc.circuitReturnErrAfter > 0 {
					if circuitCalledTimes >= tc.circuitReturnSuccessAfter && tc.circuitReturnSuccessAfter > 0 {
						circuitCalledTimes++
						return "success", nil
					}
					circuitCalledTimes++
					return "", errors.New("circuit error")
				}
				circuitCalledTimes++
				return "success", nil
			}

			settings := Settings{
				IsFailure: func(err error) bool {
					return err != nil
				},
				Threshold: tc.threshold,
			}
			b, err := NewBreaker[string](settings)
			assert.Equal(t, nil, err, "Invalid settings")
			c := b.BreakerFn(circuitFn)
			var results []string
			var errs []error
			for i := range tc.callBreakerTimes {
				if tc.pauseAfterInocationN == i {
					time.Sleep(2 * time.Second)
				}
				res, err := c(context.Background())
				results = append(results, res)
				errs = append(errs, err)
			}

			assert.Equal(t, tc.expectedCircuitCalledTimes, circuitCalledTimes, "Expected circuit called times is incorrect")
			assert.Equal(t, tc.expectedResults, results, "Expected results are incorrect")
			assert.Equal(t, tc.expectedErrs, errs, "Expected errs are incorrect")
		})
	}
}

func TestHalfOpenAllowsOnlyOneProbe(t *testing.T) {
	const threshold = 1
	circuitCalled := 0
	var circuitMu sync.Mutex
	circuitFn := func(ctx context.Context) (string, error) {
		circuitMu.Lock()
		circuitCalled++
		circuitMu.Unlock()
		time.Sleep(200 * time.Millisecond)
		return "", errors.New("circuit error")
	}

	settings := Settings{
		IsFailure: func(err error) bool { return err != nil },
		Threshold: threshold,
	}
	b, err := NewBreaker[string](settings)
	assert.Equal(t, nil, err, "Invalid settings")
	c := b.BreakerFn(circuitFn)

	_, _ = c(context.Background())
	assert.Equal(t, Open, b.State())

	time.Sleep(2 * time.Second)

	const concurrentCalls = 10
	var wg sync.WaitGroup
	errs := make([]error, concurrentCalls)
	for i := range concurrentCalls {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = c(context.Background())
		}(i)
	}
	wg.Wait()

	assert.Equal(t, 2, circuitCalled, "only the initial failure and one half-open probe should reach the circuit")
	for _, err := range errs {
		assert.Error(t, err)
	}
	assert.Equal(t, Open, b.State())
}

func TestOpenBackoffDuration(t *testing.T) {
	testCases := []struct {
		name     string
		backoff  OpenBackoff
		d        int
		expected time.Duration
	}{
		{
			name:     "default exponential d=0",
			backoff:  OpenBackoff{Strategy: OpenExponential, Base: 2 * time.Second},
			d:        0,
			expected: 2 * time.Second,
		},
		{
			name:     "default exponential d=2",
			backoff:  OpenBackoff{Strategy: OpenExponential, Base: 2 * time.Second},
			d:        2,
			expected: 8 * time.Second,
		},
		{
			name:     "exponential capped by max",
			backoff:  OpenBackoff{Strategy: OpenExponential, Base: 2 * time.Second, Max: 5 * time.Second},
			d:        3,
			expected: 5 * time.Second,
		},
		{
			name:     "fixed",
			backoff:  OpenBackoff{Strategy: OpenFixed, Base: 10 * time.Second},
			d:        5,
			expected: 10 * time.Second,
		},
		{
			name:     "linear d=0",
			backoff:  OpenBackoff{Strategy: OpenLinear, Base: 2 * time.Second},
			d:        0,
			expected: 2 * time.Second,
		},
		{
			name:     "linear d=2",
			backoff:  OpenBackoff{Strategy: OpenLinear, Base: 2 * time.Second},
			d:        2,
			expected: 6 * time.Second,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.backoff.Duration(tc.d))
		})
	}
}

func TestNewBreakerDefaultOpenBackoff(t *testing.T) {
	b, err := NewBreaker[string](Settings{
		IsFailure: func(err error) bool { return err != nil },
		Threshold: 1,
	})
	assert.NoError(t, err)
	assert.Equal(t, OpenExponential, b.openBackoff.Strategy)
	assert.Equal(t, DefaultOpenBase, b.openBackoff.Base)
	assert.Equal(t, time.Duration(0), b.openBackoff.Max)
	assert.Equal(t, 2*time.Second, b.openBackoff.Duration(0))
	assert.Equal(t, 4*time.Second, b.openBackoff.Duration(1))
}

func TestOpenFixedBackoffBlocksUntilBaseElapsed(t *testing.T) {
	settings := Settings{
		IsFailure: func(err error) bool { return err != nil },
		Threshold: 1,
		OpenBackoff: OpenBackoff{
			Strategy: OpenFixed,
			Base:     4 * time.Second,
		},
	}
	b, err := NewBreaker[string](settings)
	assert.NoError(t, err)

	call := b.BreakerFn(func(ctx context.Context) (string, error) {
		return "", errors.New("fail")
	})

	_, err = call(context.Background())
	assert.Error(t, err)
	assert.Equal(t, Open, b.State())

	_, err = call(context.Background())
	assert.True(t, errors.Is(err, BreakerErrResponse))

	time.Sleep(2 * time.Second)
	_, err = call(context.Background())
	assert.True(t, errors.Is(err, BreakerErrResponse))

	time.Sleep(3 * time.Second)
	_, err = call(context.Background())
	assert.Error(t, err)
	assert.False(t, errors.Is(err, BreakerErrResponse))

	time.Sleep(4 * time.Second)
	_, err = call(context.Background())
	assert.NotNil(t, err)
	assert.Equal(t, "fail", err.Error())
}
