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

const retryBaseURL = "http://localhost:8092"

var retryServiceStarted bool

type RetryReqBody struct {
	FailUntil int `json:"failUntil"`
}

type RetryRespBody struct {
	Message  string `json:"message"`
	Attempts int    `json:"attempts"`
}

type RetryErrorBody struct {
	Error    string `json:"error"`
	Attempts int    `json:"attempts"`
}

func startRetryService(t *testing.T) {
	t.Helper()
	if retryServiceStarted {
		return
	}

	cmd := exec.Command("go", "build", "-o", "retry-service", "./retry_service/main.go")
	if err := cmd.Run(); err != nil {
		log.Fatal("Failed to compile retry service: ", err.Error())
	}
	cmd = exec.Command("./retry-service")
	if err := cmd.Start(); err != nil {
		log.Fatal("ERR ", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			retryServiceStarted = false
		}
	})
	retryServiceStarted = true
	time.Sleep(2 * time.Second)
}

func postRetry(t *testing.T, failUntil int) (int, RetryRespBody, RetryErrorBody, error) {
	t.Helper()
	payload, err := json.Marshal(RetryReqBody{FailUntil: failUntil})
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, retryBaseURL+"/retry", bytes.NewBuffer(payload))
	if err != nil {
		return 0, RetryRespBody{}, RetryErrorBody{}, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, RetryRespBody{}, RetryErrorBody{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, RetryRespBody{}, RetryErrorBody{}, err
	}

	if resp.StatusCode == http.StatusOK {
		var ok RetryRespBody
		err = json.Unmarshal(body, &ok)
		return resp.StatusCode, ok, RetryErrorBody{}, err
	}

	var errBody RetryErrorBody
	err = json.Unmarshal(body, &errBody)
	return resp.StatusCode, RetryRespBody{}, errBody, err
}

func TestRetrySuccessAfterFailures(t *testing.T) {
	startRetryService(t)

	start := time.Now()
	statusCode, body, errBody, err := postRetry(t, 3)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "ok", body.Message)
	assert.Equal(t, 3, body.Attempts)
	assert.Empty(t, errBody.Error)
	assert.GreaterOrEqual(t, elapsed, 100*time.Millisecond)
}

func TestRetryExhaustsRetries(t *testing.T) {
	startRetryService(t)

	statusCode, body, errBody, err := postRetry(t, 10)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, statusCode)
	assert.Equal(t, "temporary", errBody.Error)
	assert.Equal(t, 4, errBody.Attempts)
	assert.Empty(t, body.Message)
}
