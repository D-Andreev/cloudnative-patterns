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
			th, err := NewThrottle[struct{}, string](tc.settings)
			assert.Error(t, err)
			assert.Nil(t, th)
		})
	}
}

func TestNewThrottleValidSettings(t *testing.T) {
	th, err := NewThrottle[struct{}, string](Settings{
		Maximum:  3,
		Duration: 100 * time.Millisecond,
		Refill:   1,
	})
	require.NoError(t, err)
	require.NotNil(t, th)
}

func TestWithErrorAllowsCallsUpToMaximum(t *testing.T) {
	th, err := NewThrottle[struct{}, string](Settings{
		Maximum:  3,
		Duration: time.Second,
		Refill:   1,
	})
	require.NoError(t, err)

	var calls int
	call := th.WithError(func(context.Context, struct{}) (string, error) {
		calls++
		return "ok", nil
	})

	for range 3 {
		res, err := call(context.Background(), struct{}{})
		require.NoError(t, err)
		assert.Equal(t, "ok", res)
	}

	res, err := call(context.Background(), struct{}{})
	require.Error(t, err)
	assert.ErrorIs(t, err, tooManyCalls)
	assert.Empty(t, res)
	assert.Equal(t, 3, calls)
}

func TestWithErrorConsumesTokenOnEffectorError(t *testing.T) {
	th, err := NewThrottle[struct{}, string](Settings{
		Maximum:  1,
		Duration: time.Second,
		Refill:   1,
	})
	require.NoError(t, err)

	wantErr := errors.New("downstream failed")
	call := th.WithError(func(context.Context, struct{}) (string, error) {
		return "", wantErr
	})

	_, err = call(context.Background(), struct{}{})
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)

	_, err = call(context.Background(), struct{}{})
	require.Error(t, err)
	assert.ErrorIs(t, err, tooManyCalls)
}

func TestWithErrorRefillsTokens(t *testing.T) {
	th, err := NewThrottle[struct{}, string](Settings{
		Maximum:  2,
		Duration: 50 * time.Millisecond,
		Refill:   2,
	})
	require.NoError(t, err)

	var calls int
	call := th.WithError(func(context.Context, struct{}) (string, error) {
		calls++
		return "ok", nil
	})

	for range 2 {
		_, err := call(context.Background(), struct{}{})
		require.NoError(t, err)
	}

	_, err = call(context.Background(), struct{}{})
	require.Error(t, err)
	assert.ErrorIs(t, err, tooManyCalls)

	require.Eventually(t, func() bool {
		res, err := call(context.Background(), struct{}{})
		return err == nil && res == "ok"
	}, time.Second, 10*time.Millisecond)

	assert.Equal(t, 3, calls)
}

func TestWithErrorPartialRefill(t *testing.T) {
	th, err := NewThrottle[struct{}, string](Settings{
		Maximum:  3,
		Duration: 50 * time.Millisecond,
		Refill:   1,
	})
	require.NoError(t, err)

	call := th.WithError(func(context.Context, struct{}) (string, error) {
		return "ok", nil
	})

	for range 3 {
		_, err := call(context.Background(), struct{}{})
		require.NoError(t, err)
	}

	_, err = call(context.Background(), struct{}{})
	require.Error(t, err)
	assert.ErrorIs(t, err, tooManyCalls)

	time.Sleep(60 * time.Millisecond)

	_, err = call(context.Background(), struct{}{})
	require.NoError(t, err)

	_, err = call(context.Background(), struct{}{})
	require.Error(t, err)
	assert.ErrorIs(t, err, tooManyCalls)
}

func TestWithErrorContextCanceledBeforeCall(t *testing.T) {
	th, err := NewThrottle[struct{}, string](Settings{
		Maximum:  3,
		Duration: time.Second,
		Refill:   1,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var calls int
	call := th.WithError(func(context.Context, struct{}) (string, error) {
		calls++
		return "ok", nil
	})

	res, err := call(ctx, struct{}{})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, res)
	assert.Equal(t, 0, calls)
}

func TestWithErrorDoesNotInvokeEffectorWhenThrottled(t *testing.T) {
	th, err := NewThrottle[struct{}, string](Settings{
		Maximum:  1,
		Duration: time.Second,
		Refill:   1,
	})
	require.NoError(t, err)

	var calls int
	call := th.WithError(func(context.Context, struct{}) (string, error) {
		calls++
		return "ok", nil
	})

	_, err = call(context.Background(), struct{}{})
	require.NoError(t, err)

	_, err = call(context.Background(), struct{}{})
	require.Error(t, err)
	assert.ErrorIs(t, err, tooManyCalls)
	assert.Equal(t, 1, calls)
}

func TestWithReplayReturnsLastSuccessWhenThrottled(t *testing.T) {
	th, err := NewThrottle[struct{}, string](Settings{
		Maximum:  1,
		Duration: time.Second,
		Refill:   1,
	})
	require.NoError(t, err)

	var calls int
	call := th.WithReplay(func(context.Context, struct{}) (string, error) {
		calls++
		return "cached", nil
	})

	res, err := call(context.Background(), struct{}{})
	require.NoError(t, err)
	assert.Equal(t, "cached", res)

	res, err = call(context.Background(), struct{}{})
	require.NoError(t, err)
	assert.Equal(t, "cached", res)
	assert.Equal(t, 1, calls)
}

func TestWithReplayUpdatesReplayOnSuccess(t *testing.T) {
	th, err := NewThrottle[struct{}, string](Settings{
		Maximum:  2,
		Duration: time.Second,
		Refill:   1,
	})
	require.NoError(t, err)

	call := th.WithReplay(func(context.Context, struct{}) (string, error) {
		return "latest", nil
	})

	res, err := call(context.Background(), struct{}{})
	require.NoError(t, err)
	assert.Equal(t, "latest", res)

	res, err = call(context.Background(), struct{}{})
	require.NoError(t, err)
	assert.Equal(t, "latest", res)

	res, err = call(context.Background(), struct{}{})
	require.NoError(t, err)
	assert.Equal(t, "latest", res)
}

func TestWithReplayDoesNotUpdateReplayOnEffectorError(t *testing.T) {
	th, err := NewThrottle[struct{}, string](Settings{
		Maximum:  2,
		Duration: time.Second,
		Refill:   1,
	})
	require.NoError(t, err)

	wantErr := errors.New("downstream failed")
	var calls int
	call := th.WithReplay(func(context.Context, struct{}) (string, error) {
		calls++
		if calls == 1 {
			return "ok", nil
		}
		return "", wantErr
	})

	res, err := call(context.Background(), struct{}{})
	require.NoError(t, err)
	assert.Equal(t, "ok", res)

	_, err = call(context.Background(), struct{}{})
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)

	res, err = call(context.Background(), struct{}{})
	require.NoError(t, err)
	assert.Equal(t, "ok", res)
	assert.Equal(t, 2, calls)
}

func TestWithReplayReturnsZeroValueWhenThrottledWithNoPriorSuccess(t *testing.T) {
	th, err := NewThrottle[struct{}, string](Settings{
		Maximum:  1,
		Duration: time.Second,
		Refill:   1,
	})
	require.NoError(t, err)

	wantErr := errors.New("downstream failed")
	call := th.WithReplay(func(context.Context, struct{}) (string, error) {
		return "", wantErr
	})

	_, err = call(context.Background(), struct{}{})
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)

	res, err := call(context.Background(), struct{}{})
	require.NoError(t, err)
	assert.Empty(t, res)
}

func TestWithReplayContextCanceledBeforeCall(t *testing.T) {
	th, err := NewThrottle[struct{}, string](Settings{
		Maximum:  1,
		Duration: time.Second,
		Refill:   1,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var calls int
	call := th.WithReplay(func(context.Context, struct{}) (string, error) {
		calls++
		return "ok", nil
	})

	res, err := call(ctx, struct{}{})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, res)
	assert.Equal(t, 0, calls)
}

func TestWithReplayRefillsAndCallsEffectorAgain(t *testing.T) {
	th, err := NewThrottle[struct{}, string](Settings{
		Maximum:  1,
		Duration: 50 * time.Millisecond,
		Refill:   1,
	})
	require.NoError(t, err)

	var calls int
	call := th.WithReplay(func(context.Context, struct{}) (string, error) {
		calls++
		if calls == 1 {
			return "first", nil
		}
		return "second", nil
	})

	res, err := call(context.Background(), struct{}{})
	require.NoError(t, err)
	assert.Equal(t, "first", res)

	res, err = call(context.Background(), struct{}{})
	require.NoError(t, err)
	assert.Equal(t, "first", res)

	require.Eventually(t, func() bool {
		res, err := call(context.Background(), struct{}{})
		return err == nil && res == "second"
	}, time.Second, 10*time.Millisecond)

	assert.Equal(t, 2, calls)
}

type labelRequest struct {
	Label string
}

func TestWithQueueAllowsCallWhenTokenAvailable(t *testing.T) {
	th, err := NewThrottle[labelRequest, string](Settings{
		Maximum:  1,
		Duration: time.Second,
		Refill:   1,
	})
	require.NoError(t, err)

	var calls int
	call := th.WithQueue(func(_ context.Context, req labelRequest) (string, error) {
		calls++
		return req.Label, nil
	})

	res, err := call(context.Background(), labelRequest{Label: "first"})
	require.NoError(t, err)
	assert.Equal(t, "first", res)
	assert.Equal(t, 1, calls)
}

func TestWithQueueReturnsTooManyCallsWhenThrottled(t *testing.T) {
	th, err := NewThrottle[labelRequest, string](Settings{
		Maximum:  1,
		Duration: time.Second,
		Refill:   1,
	})
	require.NoError(t, err)

	var calls int
	call := th.WithQueue(func(_ context.Context, req labelRequest) (string, error) {
		calls++
		return req.Label, nil
	})

	_, err = call(context.Background(), labelRequest{Label: "first"})
	require.NoError(t, err)

	res, err := call(context.Background(), labelRequest{Label: "second"})
	require.Error(t, err)
	assert.ErrorIs(t, err, tooManyCalls)
	assert.Empty(t, res)
	assert.Equal(t, 1, calls)
}

func TestWithQueueProcessesQueuedRequestWhenTokenAvailable(t *testing.T) {
	th, err := NewThrottle[labelRequest, string](Settings{
		Maximum:  1,
		Duration: 50 * time.Millisecond,
		Refill:   1,
	})
	require.NoError(t, err)

	call := th.WithQueue(func(_ context.Context, req labelRequest) (string, error) {
		return req.Label, nil
	})

	_, err = call(context.Background(), labelRequest{Label: "first"})
	require.NoError(t, err)

	_, err = call(context.Background(), labelRequest{Label: "queued"})
	require.Error(t, err)
	assert.ErrorIs(t, err, tooManyCalls)

	time.Sleep(60 * time.Millisecond)

	res, err := call(context.Background(), labelRequest{Label: "caller"})
	require.NoError(t, err)
	assert.Equal(t, "queued", res)
}

func TestWithQueueProcessesQueueInFIFOOrder(t *testing.T) {
	th, err := NewThrottle[labelRequest, string](Settings{
		Maximum:  1,
		Duration: 50 * time.Millisecond,
		Refill:   1,
	})
	require.NoError(t, err)

	call := th.WithQueue(func(_ context.Context, req labelRequest) (string, error) {
		return req.Label, nil
	})

	_, err = call(context.Background(), labelRequest{Label: "first"})
	require.NoError(t, err)

	_, err = call(context.Background(), labelRequest{Label: "second"})
	require.Error(t, err)
	assert.ErrorIs(t, err, tooManyCalls)

	_, err = call(context.Background(), labelRequest{Label: "third"})
	require.Error(t, err)
	assert.ErrorIs(t, err, tooManyCalls)

	time.Sleep(60 * time.Millisecond)

	res, err := call(context.Background(), labelRequest{Label: "processor-one"})
	require.NoError(t, err)
	assert.Equal(t, "second", res)

	time.Sleep(60 * time.Millisecond)

	res, err = call(context.Background(), labelRequest{Label: "processor-two"})
	require.NoError(t, err)
	assert.Equal(t, "third", res)
}

func TestWithQueueContextCanceledBeforeCall(t *testing.T) {
	th, err := NewThrottle[struct{}, string](Settings{
		Maximum:  1,
		Duration: time.Second,
		Refill:   1,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var calls int
	call := th.WithQueue(func(context.Context, struct{}) (string, error) {
		calls++
		return "ok", nil
	})

	res, err := call(ctx, struct{}{})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, res)
	assert.Equal(t, 0, calls)
}

func TestWithQueueDoesNotInvokeEffectorWhenThrottled(t *testing.T) {
	th, err := NewThrottle[struct{}, string](Settings{
		Maximum:  1,
		Duration: time.Second,
		Refill:   1,
	})
	require.NoError(t, err)

	var calls int
	call := th.WithQueue(func(context.Context, struct{}) (string, error) {
		calls++
		return "ok", nil
	})

	_, err = call(context.Background(), struct{}{})
	require.NoError(t, err)

	_, err = call(context.Background(), struct{}{})
	require.Error(t, err)
	assert.ErrorIs(t, err, tooManyCalls)
	assert.Equal(t, 1, calls)
}
