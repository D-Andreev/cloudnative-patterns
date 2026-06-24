package timeout

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTimeout(t *testing.T) {
	tm, err := NewTimeout[struct{}, string]()
	require.NoError(t, err)
	require.NotNil(t, tm)
}

func TestREADMEUsage(t *testing.T) {
	type FetchRequest struct {
		URL string
	}

	fetchRemote := func(_ FetchRequest) (string, error) {
		time.Sleep(500 * time.Millisecond)
		return "payload", nil
	}

	tm, err := NewTimeout[FetchRequest, string]()
	require.NoError(t, err)

	call := tm.TimeoutFn(fetchRemote)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = call(ctx, FetchRequest{URL: "https://example.com"})
	elapsed := time.Since(start)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.GreaterOrEqual(t, elapsed, 100*time.Millisecond)
	assert.Less(t, elapsed, 200*time.Millisecond)
}

func TestTimeoutFnSuccess(t *testing.T) {
	tm, err := NewTimeout[struct{}, string]()
	require.NoError(t, err)

	call := tm.TimeoutFn(func(_ struct{}) (string, error) {
		return "ok", nil
	})

	res, err := call(context.Background(), struct{}{})
	require.NoError(t, err)
	assert.Equal(t, "ok", res)
}

func TestTimeoutFnError(t *testing.T) {
	tm, err := NewTimeout[struct{}, string]()
	require.NoError(t, err)

	wantErr := errors.New("boom")
	call := tm.TimeoutFn(func(_ struct{}) (string, error) {
		return "", wantErr
	})

	res, err := call(context.Background(), struct{}{})
	assert.ErrorIs(t, err, wantErr)
	assert.Empty(t, res)
}

func TestTimeoutFnWithRequestArg(t *testing.T) {
	tm, err := NewTimeout[int, int]()
	require.NoError(t, err)

	call := tm.TimeoutFn(func(n int) (int, error) {
		return n * 2, nil
	})

	res, err := call(context.Background(), 21)
	require.NoError(t, err)
	assert.Equal(t, 42, res)
}

func TestTimeoutFnContextDeadlineExceeded(t *testing.T) {
	tm, err := NewTimeout[struct{}, string]()
	require.NoError(t, err)

	call := tm.TimeoutFn(func(_ struct{}) (string, error) {
		time.Sleep(200 * time.Millisecond)
		return "late", nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	res, err := call(ctx, struct{}{})
	elapsed := time.Since(start)

	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Empty(t, res)
	assert.Less(t, elapsed, 150*time.Millisecond)
}

func TestTimeoutFnCompletesBeforeDeadline(t *testing.T) {
	tm, err := NewTimeout[struct{}, string]()
	require.NoError(t, err)

	call := tm.TimeoutFn(func(_ struct{}) (string, error) {
		time.Sleep(20 * time.Millisecond)
		return "ok", nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	res, err := call(ctx, struct{}{})
	require.NoError(t, err)
	assert.Equal(t, "ok", res)
}
