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

const timeoutBaseURL = "http://localhost:8094"

var timeoutServiceStarted bool

type TimeoutReqBody struct {
	DelayMs int `json:"delayMs"`
}

type TimeoutRespBody struct {
	Message  string `json:"message"`
	Elapsed  int    `json:"elapsedMs"`
}

type TimeoutErrorBody struct {
	Error string `json:"error"`
}

func startTimeoutService(t *testing.T) {
	t.Helper()
	if timeoutServiceStarted {
		return
	}

	cmd := exec.Command("go", "build", "-o", "timeout-service", "./timeout_service/main.go")
	if err := cmd.Run(); err != nil {
		log.Fatal("Failed to compile timeout service: ", err.Error())
	}
	cmd = exec.Command("./timeout-service")
	if err := cmd.Start(); err != nil {
		log.Fatal("ERR ", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			timeoutServiceStarted = false
		}
	})
	timeoutServiceStarted = true
	time.Sleep(2 * time.Second)
}

func postTimeout(t *testing.T, delayMs int) (int, TimeoutRespBody, TimeoutErrorBody, error) {
	t.Helper()
	payload, err := json.Marshal(TimeoutReqBody{DelayMs: delayMs})
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, timeoutBaseURL+"/timeout", bytes.NewBuffer(payload))
	if err != nil {
		return 0, TimeoutRespBody{}, TimeoutErrorBody{}, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, TimeoutRespBody{}, TimeoutErrorBody{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, TimeoutRespBody{}, TimeoutErrorBody{}, err
	}

	if resp.StatusCode == http.StatusOK {
		var ok TimeoutRespBody
		err = json.Unmarshal(body, &ok)
		return resp.StatusCode, ok, TimeoutErrorBody{}, err
	}

	var errBody TimeoutErrorBody
	err = json.Unmarshal(body, &errBody)
	return resp.StatusCode, TimeoutRespBody{}, errBody, err
}

func TestTimeoutCompletesBeforeDeadline(t *testing.T) {
	startTimeoutService(t)

	start := time.Now()
	statusCode, body, errBody, err := postTimeout(t, 20)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "ok", body.Message)
	assert.GreaterOrEqual(t, body.Elapsed, 20)
	assert.Empty(t, errBody.Error)
	assert.Less(t, elapsed, 100*time.Millisecond)
}

func TestTimeoutDeadlineExceeded(t *testing.T) {
	startTimeoutService(t)

	start := time.Now()
	statusCode, body, errBody, err := postTimeout(t, 500)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, http.StatusGatewayTimeout, statusCode)
	assert.Equal(t, "context deadline exceeded", errBody.Error)
	assert.Empty(t, body.Message)
	assert.GreaterOrEqual(t, elapsed, 100*time.Millisecond)
	assert.Less(t, elapsed, 200*time.Millisecond)
}
