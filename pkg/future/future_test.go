package future

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAsyncSuccess(t *testing.T) {
	f := Async(context.Background(), func(context.Context) (string, error) {
		return "ok", nil
	})

	res, err := f.Result()
	require.NoError(t, err)
	assert.Equal(t, "ok", res)
}

func TestAsyncError(t *testing.T) {
	wantErr := errors.New("boom")
	f := Async(context.Background(), func(context.Context) (string, error) {
		return "", wantErr
	})

	res, err := f.Result()
	assert.ErrorIs(t, err, wantErr)
	assert.Empty(t, res)
}

func TestAsyncContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	f := Async(ctx, func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})

	res, err := f.Result()
	assert.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, res)
}

func TestAsyncRunsTasksConcurrently(t *testing.T) {
	ctx := context.Background()
	const delay = 100 * time.Millisecond

	start := time.Now()
	f1 := Async(ctx, func(context.Context) (string, error) {
		time.Sleep(delay)
		return "a", nil
	})
	f2 := Async(ctx, func(context.Context) (string, error) {
		time.Sleep(delay)
		return "b", nil
	})

	res1, err := f1.Result()
	require.NoError(t, err)
	res2, err := f2.Result()
	require.NoError(t, err)
	elapsed := time.Since(start)

	assert.Equal(t, "a", res1)
	assert.Equal(t, "b", res2)
	assert.Less(t, elapsed, 180*time.Millisecond)
}

func TestResultSafeForConcurrentCallers(t *testing.T) {
	var calls atomic.Int32
	f := Async(context.Background(), func(context.Context) (int, error) {
		calls.Add(1)
		time.Sleep(20 * time.Millisecond)
		return 42, nil
	})

	const callers = 10
	var wg sync.WaitGroup
	wg.Add(callers)
	for range callers {
		go func() {
			defer wg.Done()
			v, err := f.Result()
			require.NoError(t, err)
			assert.Equal(t, 42, v)
		}()
	}
	wg.Wait()

	assert.Equal(t, int32(1), calls.Load())
}

func TestAsyncWithoutResultDoesNotLeakGoroutine(t *testing.T) {
	done := make(chan struct{})
	f := Async(context.Background(), func(context.Context) (string, error) {
		close(done)
		return "ok", nil
	})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("task did not complete when Result was not called")
	}

	_ = f
}
