package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const throttleBaseURL = "http://localhost:8093"

var throttleServiceStarted bool

type ThrottleRespBody struct {
	Message string `json:"message"`
	Calls   int    `json:"calls"`
}

type ThrottleErrorBody struct {
	Error string `json:"error"`
}

type ThrottleQueueReqBody struct {
	Label string `json:"label"`
}

func startThrottleService(t *testing.T) {
	t.Helper()
	if throttleServiceStarted {
		return
	}

	cmd := exec.Command("go", "build", "-o", "throttle-service", "./throttle_service/main.go")
	if err := cmd.Run(); err != nil {
		log.Fatal("Failed to compile throttle service: ", err.Error())
	}
	cmd = exec.Command("./throttle-service")
	if err := cmd.Start(); err != nil {
		log.Fatal("ERR ", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			throttleServiceStarted = false
		}
	})
	throttleServiceStarted = true
	time.Sleep(2 * time.Second)
}

func resetThrottleService(t *testing.T) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, throttleBaseURL+"/reset", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func postThrottle(t *testing.T) (int, ThrottleRespBody, ThrottleErrorBody, error) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, throttleBaseURL+"/throttle", nil)
	if err != nil {
		return 0, ThrottleRespBody{}, ThrottleErrorBody{}, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, ThrottleRespBody{}, ThrottleErrorBody{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, ThrottleRespBody{}, ThrottleErrorBody{}, err
	}

	if resp.StatusCode == http.StatusOK {
		var ok ThrottleRespBody
		err = json.Unmarshal(body, &ok)
		return resp.StatusCode, ok, ThrottleErrorBody{}, err
	}

	var errBody ThrottleErrorBody
	err = json.Unmarshal(body, &errBody)
	return resp.StatusCode, ThrottleRespBody{}, errBody, err
}

func postThrottleReplay(t *testing.T) (int, ThrottleRespBody, ThrottleErrorBody, error) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, throttleBaseURL+"/throttle/replay", nil)
	if err != nil {
		return 0, ThrottleRespBody{}, ThrottleErrorBody{}, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, ThrottleRespBody{}, ThrottleErrorBody{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, ThrottleRespBody{}, ThrottleErrorBody{}, err
	}

	if resp.StatusCode == http.StatusOK {
		var ok ThrottleRespBody
		err = json.Unmarshal(body, &ok)
		return resp.StatusCode, ok, ThrottleErrorBody{}, err
	}

	var errBody ThrottleErrorBody
	err = json.Unmarshal(body, &errBody)
	return resp.StatusCode, ThrottleRespBody{}, errBody, err
}

func postThrottleQueue(t *testing.T, label string) (int, ThrottleRespBody, ThrottleErrorBody, error) {
	t.Helper()
	payload, err := json.Marshal(ThrottleQueueReqBody{Label: label})
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, throttleBaseURL+"/throttle/queue", bytes.NewBuffer(payload))
	if err != nil {
		return 0, ThrottleRespBody{}, ThrottleErrorBody{}, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, ThrottleRespBody{}, ThrottleErrorBody{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, ThrottleRespBody{}, ThrottleErrorBody{}, err
	}

	if resp.StatusCode == http.StatusOK {
		var ok ThrottleRespBody
		err = json.Unmarshal(body, &ok)
		return resp.StatusCode, ok, ThrottleErrorBody{}, err
	}

	var errBody ThrottleErrorBody
	err = json.Unmarshal(body, &errBody)
	return resp.StatusCode, ThrottleRespBody{}, errBody, err
}

func TestThrottleAllowsCallsUpToMaximum(t *testing.T) {
	startThrottleService(t)
	resetThrottleService(t)

	for i := 1; i <= 3; i++ {
		statusCode, body, errBody, err := postThrottle(t)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, statusCode)
		assert.Equal(t, "ok", body.Message)
		assert.Equal(t, i, body.Calls)
		assert.Empty(t, errBody.Error)
	}

	statusCode, body, errBody, err := postThrottle(t)
	require.NoError(t, err)
	assert.Equal(t, http.StatusTooManyRequests, statusCode)
	assert.Equal(t, "too many calls", errBody.Error)
	assert.Empty(t, body.Message)
}

func TestThrottleRefillsTokens(t *testing.T) {
	startThrottleService(t)
	resetThrottleService(t)

	for range 3 {
		statusCode, _, errBody, err := postThrottle(t)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, statusCode)
		assert.Empty(t, errBody.Error)
	}

	statusCode, _, errBody, err := postThrottle(t)
	require.NoError(t, err)
	require.Equal(t, http.StatusTooManyRequests, statusCode)
	assert.Equal(t, "too many calls", errBody.Error)

	require.Eventually(t, func() bool {
		statusCode, body, errBody, err := postThrottle(t)
		return err == nil &&
			statusCode == http.StatusOK &&
			body.Message == "ok" &&
			body.Calls == 4 &&
			errBody.Error == ""
	}, time.Second, 10*time.Millisecond)
}

func TestThrottleReplayReturnsLastSuccessWhenThrottled(t *testing.T) {
	startThrottleService(t)
	resetThrottleService(t)

	statusCode, body, errBody, err := postThrottleReplay(t)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "ok", body.Message)
	assert.Equal(t, 1, body.Calls)
	assert.Empty(t, errBody.Error)

	statusCode, body, errBody, err = postThrottleReplay(t)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "ok", body.Message)
	assert.Equal(t, 1, body.Calls)
	assert.Empty(t, errBody.Error)
}

func TestThrottleReplayRefillsAndUpdates(t *testing.T) {
	startThrottleService(t)
	resetThrottleService(t)

	statusCode, body, errBody, err := postThrottleReplay(t)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, 1, body.Calls)
	assert.Empty(t, errBody.Error)

	statusCode, body, errBody, err = postThrottleReplay(t)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, 1, body.Calls)
	assert.Empty(t, errBody.Error)

	require.Eventually(t, func() bool {
		statusCode, body, errBody, err := postThrottleReplay(t)
		return err == nil &&
			statusCode == http.StatusOK &&
			body.Message == "ok" &&
			body.Calls == 2 &&
			errBody.Error == ""
	}, time.Second, 10*time.Millisecond)
}

func TestThrottleQueueReturnsTooManyCallsWhenThrottled(t *testing.T) {
	startThrottleService(t)
	resetThrottleService(t)

	statusCode, body, errBody, err := postThrottleQueue(t, "first")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "first", body.Message)
	assert.Equal(t, 1, body.Calls)
	assert.Empty(t, errBody.Error)

	statusCode, body, errBody, err = postThrottleQueue(t, "queued")
	require.NoError(t, err)
	assert.Equal(t, http.StatusTooManyRequests, statusCode)
	assert.Equal(t, "too many calls", errBody.Error)
	assert.Empty(t, body.Message)
}

func TestThrottleQueueProcessesQueuedRequestAfterRefill(t *testing.T) {
	startThrottleService(t)
	resetThrottleService(t)

	statusCode, body, errBody, err := postThrottleQueue(t, "first")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "first", body.Message)
	assert.Empty(t, errBody.Error)

	statusCode, _, errBody, err = postThrottleQueue(t, "queued")
	require.NoError(t, err)
	assert.Equal(t, http.StatusTooManyRequests, statusCode)
	assert.Equal(t, "too many calls", errBody.Error)

	time.Sleep(150 * time.Millisecond)

	statusCode, body, errBody, err = postThrottleQueue(t, "caller")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "queued", body.Message)
	assert.Equal(t, 2, body.Calls)
	assert.Empty(t, errBody.Error)
}

func TestThrottleQueueProcessesFIFO(t *testing.T) {
	startThrottleService(t)
	resetThrottleService(t)

	statusCode, _, errBody, err := postThrottleQueue(t, "first")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, statusCode)
	assert.Empty(t, errBody.Error)

	statusCode, _, errBody, err = postThrottleQueue(t, "second")
	require.NoError(t, err)
	require.Equal(t, http.StatusTooManyRequests, statusCode)
	assert.Equal(t, "too many calls", errBody.Error)

	statusCode, _, errBody, err = postThrottleQueue(t, "third")
	require.NoError(t, err)
	require.Equal(t, http.StatusTooManyRequests, statusCode)
	assert.Equal(t, "too many calls", errBody.Error)

	time.Sleep(150 * time.Millisecond)

	statusCode, body, errBody, err := postThrottleQueue(t, "processor-one")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "second", body.Message)
	assert.Equal(t, 2, body.Calls)
	assert.Empty(t, errBody.Error)

	time.Sleep(150 * time.Millisecond)

	statusCode, body, errBody, err = postThrottleQueue(t, "processor-two")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "third", body.Message)
	assert.Equal(t, 3, body.Calls)
	assert.Empty(t, errBody.Error)
}
