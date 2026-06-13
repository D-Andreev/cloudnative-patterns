package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRetry(t *testing.T) {
	r, err := NewRetry[struct{}, string](Settings{
		Delay:       10 * time.Millisecond,
		MaxFailures: 3,
	})
	require.NoError(t, err)
	require.NotNil(t, r)
}

func TestRetryFnSuccessOnFirstAttempt(t *testing.T) {
	r, err := NewRetry[struct{}, string](Settings{
		Delay:       10 * time.Millisecond,
		MaxFailures: 3,
	})
	require.NoError(t, err)

	var calls int
	call := r.RetryFn(func(context.Context, struct{}) (string, error) {
		calls++
		return "ok", nil
	})

	res, err := call(context.Background(), struct{}{})
	require.NoError(t, err)
	assert.Equal(t, "ok", res)
	assert.Equal(t, 1, calls)
}

func TestRetryFnSuccessAfterRetries(t *testing.T) {
	r, err := NewRetry[struct{}, string](Settings{
		Delay:       10 * time.Millisecond,
		MaxFailures: 3,
	})
	require.NoError(t, err)

	var calls int
	call := r.RetryFn(func(context.Context, struct{}) (string, error) {
		calls++
		if calls < 3 {
			return "", errors.New("temporary")
		}
		return "ok", nil
	})

	start := time.Now()
	res, err := call(context.Background(), struct{}{})
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, "ok", res)
	assert.Equal(t, 3, calls)
	assert.GreaterOrEqual(t, elapsed, 20*time.Millisecond)
}

func TestRetryFnExhaustsRetries(t *testing.T) {
	r, err := NewRetry[struct{}, string](Settings{
		Delay:       10 * time.Millisecond,
		MaxFailures: 2,
	})
	require.NoError(t, err)

	wantErr := errors.New("permanent")
	var calls int
	call := r.RetryFn(func(context.Context, struct{}) (string, error) {
		calls++
		return "", wantErr
	})

	res, err := call(context.Background(), struct{}{})
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
	assert.Empty(t, res)
	assert.Equal(t, 3, calls)
}

func TestRetryFnNoRetriesWhenMaxFailuresZero(t *testing.T) {
	r, err := NewRetry[struct{}, string](Settings{
		Delay:       10 * time.Millisecond,
		MaxFailures: 0,
	})
	require.NoError(t, err)

	wantErr := errors.New("fail")
	var calls int
	call := r.RetryFn(func(context.Context, struct{}) (string, error) {
		calls++
		return "", wantErr
	})

	_, err = call(context.Background(), struct{}{})
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
	assert.Equal(t, 1, calls)
}

func TestRetryFnContextCanceledDuringDelay(t *testing.T) {
	r, err := NewRetry[struct{}, string](Settings{
		Delay:       200 * time.Millisecond,
		MaxFailures: 3,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var calls int
	call := r.RetryFn(func(context.Context, struct{}) (string, error) {
		calls++
		return "", errors.New("temporary")
	})

	done := make(chan struct{})
	var res string
	var callErr error
	go func() {
		defer close(done)
		res, callErr = call(ctx, struct{}{})
	}()

	require.Eventually(t, func() bool {
		return calls == 1
	}, time.Second, 5*time.Millisecond)

	cancel()

	<-done
	require.Error(t, callErr)
	assert.ErrorIs(t, callErr, context.Canceled)
	assert.Empty(t, res)
	assert.Equal(t, 1, calls)
}
