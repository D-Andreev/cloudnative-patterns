package debounce

import (
	"context"
	"sync/atomic"
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

	var calls atomic.Int32
	call := d.DebounceFirstFn(func(ctx context.Context) (string, error) {
		calls.Add(1)
		return "ok", nil
	})

	call(context.Background())
	call(context.Background())
	call(context.Background())

	assert.Equal(t, int32(1), calls.Load())
}

func TestDebounceFnFunctionLast(t *testing.T) {
	d, err := NewDebounce[string](Settings{
		DebounceType: FunctionLast,
		Duration:     1 * time.Second,
	})
	require.NoError(t, err)

	var calls int
	call := d.DebounceLasttFn(func(ctx context.Context) (string, error) {
		calls++
		return "ok", nil
	})

	call(context.Background())
	call(context.Background())
	call(context.Background())

	assert.Equal(t, 1, calls)
}
