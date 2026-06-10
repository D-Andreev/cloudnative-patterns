package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const debounceBaseURL = "http://localhost:8091"

var debounceServiceStarted bool

type DebounceRespBody struct {
	Message string `json:"message"`
	Calls   int    `json:"calls"`
}

type DebounceErrorBody struct {
	Error string `json:"error"`
}

func startDebounceService(t *testing.T) {
	t.Helper()
	if debounceServiceStarted {
		return
	}

	cmd := exec.Command("go", "build", "-o", "debounce-service", "./debounce_service/main.go")
	if err := cmd.Run(); err != nil {
		log.Fatal("Failed to compile debounce service: ", err.Error())
	}
	cmd = exec.Command("./debounce-service")
	if err := cmd.Start(); err != nil {
		log.Fatal("ERR ", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			debounceServiceStarted = false
		}
	})
	debounceServiceStarted = true
	time.Sleep(2 * time.Second)
}

func resetDebounceService(t *testing.T) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, debounceBaseURL+"/reset", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func postDebounceFirst(t *testing.T) (int, DebounceRespBody, error) {
	t.Helper()
	return postDebounce(t, debounceBaseURL+"/debounce/first")
}

func postDebounceLast(t *testing.T) (int, DebounceRespBody, DebounceErrorBody, error) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, debounceBaseURL+"/debounce/last", bytes.NewBuffer([]byte("{}")))
	if err != nil {
		return 0, DebounceRespBody{}, DebounceErrorBody{}, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, DebounceRespBody{}, DebounceErrorBody{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, DebounceRespBody{}, DebounceErrorBody{}, err
	}

	if resp.StatusCode == http.StatusOK {
		var ok DebounceRespBody
		err = json.Unmarshal(body, &ok)
		return resp.StatusCode, ok, DebounceErrorBody{}, err
	}

	var errBody DebounceErrorBody
	err = json.Unmarshal(body, &errBody)
	return resp.StatusCode, DebounceRespBody{}, errBody, err
}

func postDebounce(t *testing.T, url string) (int, DebounceRespBody, error) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer([]byte("{}")))
	if err != nil {
		return 0, DebounceRespBody{}, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, DebounceRespBody{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, DebounceRespBody{}, err
	}

	var respBody DebounceRespBody
	err = json.Unmarshal(body, &respBody)
	return resp.StatusCode, respBody, err
}

func TestDebounceFirstRapidCalls(t *testing.T) {
	startDebounceService(t)
	resetDebounceService(t)

	for range 5 {
		statusCode, body, err := postDebounceFirst(t)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, statusCode)
		assert.Equal(t, "ok", body.Message)
		assert.Equal(t, 1, body.Calls)
	}
}

func TestDebounceLastBurst(t *testing.T) {
	startDebounceService(t)
	resetDebounceService(t)

	go func() { _, _, _, _ = postDebounceLast(t) }()
	time.Sleep(20 * time.Millisecond)
	go func() { _, _, _, _ = postDebounceLast(t) }()
	time.Sleep(20 * time.Millisecond)

	statusCode, body, errBody, err := postDebounceLast(t)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "ok", body.Message)
	assert.Equal(t, 1, body.Calls)
	assert.Empty(t, errBody.Error)
}

func TestDebounceLastSupersededCall(t *testing.T) {
	startDebounceService(t)
	resetDebounceService(t)

	var wg sync.WaitGroup
	wg.Go(func() {
		statusCode, _, errBody, err := postDebounceLast(t)
		require.NoError(t, err)
		assert.Equal(t, http.StatusConflict, statusCode)
		assert.Equal(t, "context canceled", errBody.Error)
	})

	time.Sleep(20 * time.Millisecond)

	statusCode, body, _, err := postDebounceLast(t)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "ok", body.Message)
	assert.Equal(t, 1, body.Calls)

	wg.Wait()
}
