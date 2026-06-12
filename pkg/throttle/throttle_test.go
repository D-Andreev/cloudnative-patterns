package throttle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewThrottleInvalidSettings(t *testing.T) {
	testCases := []struct {
		name     string
		settings Settings
	}{
		{
			name: "maximum below min",
			settings: Settings{
				Maximum:  0,
				Duration: time.Second,
				Refill:   1,
			},
		},
		{
			name: "duration below min",
			settings: Settings{
				Maximum:  1,
				Duration: 0,
				Refill:   1,
			},
		},
		{
			name: "refill below min",
			settings: Settings{
				Maximum:  1,
				Duration: time.Second,
				Refill:   0,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			th, err := NewThrottle[string](tc.settings)
			assert.Error(t, err)
			assert.Nil(t, th)
		})
	}
}

func TestNewThrottleValidSettings(t *testing.T) {
	th, err := NewThrottle[string](Settings{
		Maximum:  3,
		Duration: 100 * time.Millisecond,
		Refill:   1,
	})
	require.NoError(t, err)
	require.NotNil(t, th)
}

func TestThrottleFnAllowsCallsUpToMaximum(t *testing.T) {
	th, err := NewThrottle[string](Settings{
		Maximum:  3,
		Duration: time.Second,
		Refill:   1,
	})
	require.NoError(t, err)

	var calls int
	call := th.ThrottleFnWithError(func(context.Context) (string, error) {
		calls++
		return "ok", nil
	})

	for range 3 {
		res, err := call(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "ok", res)
	}

	res, err := call(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, tooManyCalls)
	assert.Empty(t, res)
	assert.Equal(t, 3, calls)
}

func TestThrottleFnConsumesTokenOnEffectorError(t *testing.T) {
	th, err := NewThrottle[string](Settings{
		Maximum:  1,
		Duration: time.Second,
		Refill:   1,
	})
	require.NoError(t, err)

	wantErr := errors.New("downstream failed")
	call := th.ThrottleFnWithError(func(context.Context) (string, error) {
		return "", wantErr
	})

	_, err = call(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)

	_, err = call(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, tooManyCalls)
}

func TestThrottleFnRefillsTokens(t *testing.T) {
	th, err := NewThrottle[string](Settings{
		Maximum:  2,
		Duration: 50 * time.Millisecond,
		Refill:   2,
	})
	require.NoError(t, err)

	var calls int
	call := th.ThrottleFnWithError(func(context.Context) (string, error) {
		calls++
		return "ok", nil
	})

	for range 2 {
		_, err := call(context.Background())
		require.NoError(t, err)
	}

	_, err = call(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, tooManyCalls)

	require.Eventually(t, func() bool {
		res, err := call(context.Background())
		return err == nil && res == "ok"
	}, time.Second, 10*time.Millisecond)

	assert.Equal(t, 3, calls)
}

func TestThrottleFnPartialRefill(t *testing.T) {
	th, err := NewThrottle[string](Settings{
		Maximum:  3,
		Duration: 50 * time.Millisecond,
		Refill:   1,
	})
	require.NoError(t, err)

	call := th.ThrottleFnWithError(func(context.Context) (string, error) {
		return "ok", nil
	})

	for range 3 {
		_, err := call(context.Background())
		require.NoError(t, err)
	}

	_, err = call(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, tooManyCalls)

	time.Sleep(60 * time.Millisecond)

	_, err = call(context.Background())
	require.NoError(t, err)

	_, err = call(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, tooManyCalls)
}

func TestThrottleFnContextCanceledBeforeCall(t *testing.T) {
	th, err := NewThrottle[string](Settings{
		Maximum:  3,
		Duration: time.Second,
		Refill:   1,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var calls int
	call := th.ThrottleFnWithError(func(context.Context) (string, error) {
		calls++
		return "ok", nil
	})

	res, err := call(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, res)
	assert.Equal(t, 0, calls)
}

func TestThrottleFnDoesNotInvokeEffectorWhenThrottled(t *testing.T) {
	th, err := NewThrottle[string](Settings{
		Maximum:  1,
		Duration: time.Second,
		Refill:   1,
	})
	require.NoError(t, err)

	var calls int
	call := th.ThrottleFnWithError(func(context.Context) (string, error) {
		calls++
		return "ok", nil
	})

	_, err = call(context.Background())
	require.NoError(t, err)

	_, err = call(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, tooManyCalls)
	assert.Equal(t, 1, calls)
}

func TestThrottleFnWithReplayReturnsLastSuccessWhenThrottled(t *testing.T) {
	th, err := NewThrottle[string](Settings{
		Maximum:  1,
		Duration: time.Second,
		Refill:   1,
	})
	require.NoError(t, err)

	var calls int
	call := th.ThrottleFnWithReplay(func(context.Context) (string, error) {
		calls++
		return "cached", nil
	})

	res, err := call(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "cached", res)

	res, err = call(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "cached", res)
	assert.Equal(t, 1, calls)
}

func TestThrottleFnWithReplayUpdatesReplayOnSuccess(t *testing.T) {
	th, err := NewThrottle[string](Settings{
		Maximum:  2,
		Duration: time.Second,
		Refill:   1,
	})
	require.NoError(t, err)

	call := th.ThrottleFnWithReplay(func(context.Context) (string, error) {
		return "latest", nil
	})

	res, err := call(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "latest", res)

	res, err = call(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "latest", res)

	res, err = call(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "latest", res)
}

func TestThrottleFnWithReplayDoesNotUpdateReplayOnEffectorError(t *testing.T) {
	th, err := NewThrottle[string](Settings{
		Maximum:  2,
		Duration: time.Second,
		Refill:   1,
	})
	require.NoError(t, err)

	wantErr := errors.New("downstream failed")
	var calls int
	call := th.ThrottleFnWithReplay(func(context.Context) (string, error) {
		calls++
		if calls == 1 {
			return "ok", nil
		}
		return "", wantErr
	})

	res, err := call(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "ok", res)

	_, err = call(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)

	res, err = call(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "ok", res)
	assert.Equal(t, 2, calls)
}

func TestThrottleFnWithReplayReturnsZeroValueWhenThrottledWithNoPriorSuccess(t *testing.T) {
	th, err := NewThrottle[string](Settings{
		Maximum:  1,
		Duration: time.Second,
		Refill:   1,
	})
	require.NoError(t, err)

	wantErr := errors.New("downstream failed")
	call := th.ThrottleFnWithReplay(func(context.Context) (string, error) {
		return "", wantErr
	})

	_, err = call(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)

	res, err := call(context.Background())
	require.NoError(t, err)
	assert.Empty(t, res)
}

func TestThrottleFnWithReplayContextCanceledBeforeCall(t *testing.T) {
	th, err := NewThrottle[string](Settings{
		Maximum:  1,
		Duration: time.Second,
		Refill:   1,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var calls int
	call := th.ThrottleFnWithReplay(func(context.Context) (string, error) {
		calls++
		return "ok", nil
	})

	res, err := call(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, res)
	assert.Equal(t, 0, calls)
}

func TestThrottleFnWithReplayRefillsAndCallsEffectorAgain(t *testing.T) {
	th, err := NewThrottle[string](Settings{
		Maximum:  1,
		Duration: 50 * time.Millisecond,
		Refill:   1,
	})
	require.NoError(t, err)

	var calls int
	call := th.ThrottleFnWithReplay(func(context.Context) (string, error) {
		calls++
		if calls == 1 {
			return "first", nil
		}
		return "second", nil
	})

	res, err := call(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "first", res)

	res, err = call(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "first", res)

	require.Eventually(t, func() bool {
		res, err := call(context.Background())
		return err == nil && res == "second"
	}, time.Second, 10*time.Millisecond)

	assert.Equal(t, 2, calls)
}

type queueCtxKey struct{}

func ctxWithLabel(label string) context.Context {
	return context.WithValue(context.Background(), queueCtxKey{}, label)
}

func labelFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(queueCtxKey{}).(string)
	return v
}

func TestThrottleFnWithQueueAllowsCallWhenTokenAvailable(t *testing.T) {
	th, err := NewThrottle[string](Settings{
		Maximum:  1,
		Duration: time.Second,
		Refill:   1,
	})
	require.NoError(t, err)

	var calls int
	call := th.ThrottleFnWithQueue(func(ctx context.Context) (string, error) {
		calls++
		return labelFromCtx(ctx), nil
	})

	res, err := call(ctxWithLabel("first"))
	require.NoError(t, err)
	assert.Equal(t, "first", res)
	assert.Equal(t, 1, calls)
}

func TestThrottleFnWithQueueReturnsTooManyCallsWhenThrottled(t *testing.T) {
	th, err := NewThrottle[string](Settings{
		Maximum:  1,
		Duration: time.Second,
		Refill:   1,
	})
	require.NoError(t, err)

	var calls int
	call := th.ThrottleFnWithQueue(func(ctx context.Context) (string, error) {
		calls++
		return labelFromCtx(ctx), nil
	})

	_, err = call(ctxWithLabel("first"))
	require.NoError(t, err)

	res, err := call(ctxWithLabel("second"))
	require.Error(t, err)
	assert.ErrorIs(t, err, tooManyCalls)
	assert.Empty(t, res)
	assert.Equal(t, 1, calls)
}

func TestThrottleFnWithQueueProcessesQueuedContextWhenTokenAvailable(t *testing.T) {
	th, err := NewThrottle[string](Settings{
		Maximum:  1,
		Duration: 50 * time.Millisecond,
		Refill:   1,
	})
	require.NoError(t, err)

	call := th.ThrottleFnWithQueue(func(ctx context.Context) (string, error) {
		return labelFromCtx(ctx), nil
	})

	_, err = call(ctxWithLabel("first"))
	require.NoError(t, err)

	_, err = call(ctxWithLabel("queued"))
	require.Error(t, err)
	assert.ErrorIs(t, err, tooManyCalls)

	time.Sleep(60 * time.Millisecond)

	res, err := call(ctxWithLabel("caller"))
	require.NoError(t, err)
	assert.Equal(t, "queued", res)
}

func TestThrottleFnWithQueueProcessesQueueInFIFOOrder(t *testing.T) {
	th, err := NewThrottle[string](Settings{
		Maximum:  1,
		Duration: 50 * time.Millisecond,
		Refill:   1,
	})
	require.NoError(t, err)

	call := th.ThrottleFnWithQueue(func(ctx context.Context) (string, error) {
		return labelFromCtx(ctx), nil
	})

	_, err = call(ctxWithLabel("first"))
	require.NoError(t, err)

	_, err = call(ctxWithLabel("second"))
	require.Error(t, err)
	assert.ErrorIs(t, err, tooManyCalls)

	_, err = call(ctxWithLabel("third"))
	require.Error(t, err)
	assert.ErrorIs(t, err, tooManyCalls)

	time.Sleep(60 * time.Millisecond)

	res, err := call(ctxWithLabel("processor-one"))
	require.NoError(t, err)
	assert.Equal(t, "second", res)

	time.Sleep(60 * time.Millisecond)

	res, err = call(ctxWithLabel("processor-two"))
	require.NoError(t, err)
	assert.Equal(t, "third", res)
}

func TestThrottleFnWithQueueContextCanceledBeforeCall(t *testing.T) {
	th, err := NewThrottle[string](Settings{
		Maximum:  1,
		Duration: time.Second,
		Refill:   1,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var calls int
	call := th.ThrottleFnWithQueue(func(context.Context) (string, error) {
		calls++
		return "ok", nil
	})

	res, err := call(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, res)
	assert.Equal(t, 0, calls)
}

func TestThrottleFnWithQueueDoesNotInvokeEffectorWhenThrottled(t *testing.T) {
	th, err := NewThrottle[string](Settings{
		Maximum:  1,
		Duration: time.Second,
		Refill:   1,
	})
	require.NoError(t, err)

	var calls int
	call := th.ThrottleFnWithQueue(func(context.Context) (string, error) {
		calls++
		return "ok", nil
	})

	_, err = call(context.Background())
	require.NoError(t, err)

	_, err = call(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, tooManyCalls)
	assert.Equal(t, 1, calls)
}
