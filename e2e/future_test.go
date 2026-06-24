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

const futureBaseURL = "http://localhost:8097"

var futureServiceStarted bool

type FutureReqBody struct {
	DelaysMs []int `json:"delaysMs"`
	FailOn   *int  `json:"failOn,omitempty"`
}

type FutureRespBody struct {
	Results   []int `json:"results"`
	ElapsedMs int   `json:"elapsedMs"`
}

type FutureErrorBody struct {
	Error string `json:"error"`
}

func startFutureService(t *testing.T) {
	t.Helper()
	if futureServiceStarted {
		return
	}

	cmd := exec.Command("go", "build", "-o", "future-service", "./future_service/main.go")
	if err := cmd.Run(); err != nil {
		log.Fatal("Failed to compile future service: ", err.Error())
	}
	cmd = exec.Command("./future-service")
	if err := cmd.Start(); err != nil {
		log.Fatal("ERR ", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			futureServiceStarted = false
		}
	})
	futureServiceStarted = true
	time.Sleep(2 * time.Second)
}

func postFuture(t *testing.T, req FutureReqBody) (int, FutureRespBody, FutureErrorBody, error) {
	t.Helper()
	payload, err := json.Marshal(req)
	require.NoError(t, err)

	httpReq, err := http.NewRequest(http.MethodPost, futureBaseURL+"/future", bytes.NewBuffer(payload))
	if err != nil {
		return 0, FutureRespBody{}, FutureErrorBody{}, err
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return 0, FutureRespBody{}, FutureErrorBody{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, FutureRespBody{}, FutureErrorBody{}, err
	}

	if resp.StatusCode == http.StatusOK {
		var ok FutureRespBody
		err = json.Unmarshal(body, &ok)
		return resp.StatusCode, ok, FutureErrorBody{}, err
	}

	var errBody FutureErrorBody
	err = json.Unmarshal(body, &errBody)
	return resp.StatusCode, FutureRespBody{}, errBody, err
}

func TestFutureRunsTasksConcurrently(t *testing.T) {
	startFutureService(t)

	start := time.Now()
	statusCode, body, errBody, err := postFuture(t, FutureReqBody{
		DelaysMs: []int{100, 150},
	})
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Empty(t, errBody.Error)
	assert.Equal(t, []int{100, 150}, body.Results)
	assert.GreaterOrEqual(t, body.ElapsedMs, 150)
	assert.Less(t, body.ElapsedMs, 250)
	assert.Less(t, elapsed, 250*time.Millisecond)
}

func TestFutureSingleTask(t *testing.T) {
	startFutureService(t)

	statusCode, body, errBody, err := postFuture(t, FutureReqBody{
		DelaysMs: []int{50},
	})

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Empty(t, errBody.Error)
	assert.Equal(t, []int{50}, body.Results)
	assert.GreaterOrEqual(t, body.ElapsedMs, 50)
}

func TestFutureTaskError(t *testing.T) {
	startFutureService(t)

	failOn := 1
	statusCode, body, errBody, err := postFuture(t, FutureReqBody{
		DelaysMs: []int{20, 20},
		FailOn:   &failOn,
	})

	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, statusCode)
	assert.Equal(t, "context canceled", errBody.Error)
	assert.Empty(t, body.Results)
}

func TestFutureBadRequest(t *testing.T) {
	startFutureService(t)

	statusCode, body, errBody, err := postFuture(t, FutureReqBody{
		DelaysMs: []int{},
	})

	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode)
	assert.Equal(t, "bad request", errBody.Error)
	assert.Empty(t, body.Results)
}
