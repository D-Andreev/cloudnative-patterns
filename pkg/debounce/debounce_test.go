package debounce

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDebounceInvalidSettings(t *testing.T) {
	testCases := []struct {
		name     string
		settings Settings
		wantErr  bool
	}{
		{
			name: "below min DebounceType",
			settings: Settings{
				Duration:     0,
				DebounceType: -1,
			},
			wantErr: true,
		},
		{
			name: "above max DebounceType",
			settings: Settings{
				Duration:     0,
				DebounceType: 2,
			},
			wantErr: true,
		},
		{
			name: "valid settings",
			settings: Settings{
				Duration:     2 * time.Second,
				DebounceType: 0,
			},
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := NewDebounce[string](tc.settings)
			if tc.wantErr {
				assert.Error(t, err)
				assert.Nil(t, d)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, d)
		})
	}
}

func TestDebounceFnFunctionFirst(t *testing.T) {
	d, err := NewDebounce[string](Settings{
		DebounceType: FunctionFirst,
		Duration:     1 * time.Second,
	})
	require.NoError(t, err)

	var calls int
	call := d.DebounceFirstFn(func(ctx context.Context) (string, error) {
		calls++
		return "ok", nil
	})

	res1, err1 := call(context.Background())
	res2, err2 := call(context.Background())
	res3, err3 := call(context.Background())

	require.NoError(t, err1)
	require.NoError(t, err2)
	require.NoError(t, err3)
	assert.Equal(t, "ok", res1)
	assert.Equal(t, "ok", res2)
	assert.Equal(t, "ok", res3)
	assert.Equal(t, 1, calls)
}

func TestDebounceFnFunctionLast(t *testing.T) {
	d, err := NewDebounce[string](Settings{
		DebounceType: FunctionLast,
		Duration:     50 * time.Millisecond,
	})
	require.NoError(t, err)

	var calls int
	call := d.DebounceLastFn(func(ctx context.Context) (string, error) {
		calls++
		return "ok", nil
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = call(context.Background())
	}()

	time.Sleep(10 * time.Millisecond)
	go func() { _, _ = call(context.Background()) }()
	time.Sleep(10 * time.Millisecond)

	res, err := call(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "ok", res)
	assert.Equal(t, 1, calls)

	<-done
}

func TestDebounceLastFnCancelsSupersededCall(t *testing.T) {
	d, err := NewDebounce[string](Settings{
		DebounceType: FunctionLast,
		Duration:     200 * time.Millisecond,
	})
	require.NoError(t, err)

	call := d.DebounceLastFn(func(ctx context.Context) (string, error) {
		select {
		case <-time.After(500 * time.Millisecond):
			return "ok", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})

	go func() {
		_, err := call(context.Background())
		assert.Error(t, err)
		assert.True(t, errors.Is(err, context.Canceled))
	}()

	time.Sleep(20 * time.Millisecond)
	res, err := call(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "ok", res)
}
