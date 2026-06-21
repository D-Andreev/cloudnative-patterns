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

const fanOutBaseURL = "http://localhost:8096"

var fanOutServiceStarted bool

type FanOutReqBody struct {
	Destinations int `json:"destinations"`
	Values       int `json:"values"`
}

type FanOutRespBody struct {
	Values       []int `json:"values"`
	Count        int   `json:"count"`
	Destinations int   `json:"destinations"`
}

type FanOutErrorBody struct {
	Error string `json:"error"`
}

func startFanOutService(t *testing.T) {
	t.Helper()
	if fanOutServiceStarted {
		return
	}

	cmd := exec.Command("go", "build", "-o", "fan-out-service", "./fan_out_service/main.go")
	if err := cmd.Run(); err != nil {
		log.Fatal("Failed to compile fan-out service: ", err.Error())
	}
	cmd = exec.Command("./fan-out-service")
	if err := cmd.Start(); err != nil {
		log.Fatal("ERR ", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			fanOutServiceStarted = false
		}
	})
	fanOutServiceStarted = true
	time.Sleep(2 * time.Second)
}

func postFanOut(t *testing.T, destinations, values int) (int, FanOutRespBody, FanOutErrorBody, error) {
	t.Helper()
	payload, err := json.Marshal(FanOutReqBody{
		Destinations: destinations,
		Values:       values,
	})
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, fanOutBaseURL+"/fan-out", bytes.NewBuffer(payload))
	if err != nil {
		return 0, FanOutRespBody{}, FanOutErrorBody{}, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, FanOutRespBody{}, FanOutErrorBody{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, FanOutRespBody{}, FanOutErrorBody{}, err
	}

	if resp.StatusCode == http.StatusOK {
		var ok FanOutRespBody
		err = json.Unmarshal(body, &ok)
		return resp.StatusCode, ok, FanOutErrorBody{}, err
	}

	var errBody FanOutErrorBody
	err = json.Unmarshal(body, &errBody)
	return resp.StatusCode, FanOutRespBody{}, errBody, err
}

func TestFanOutDistributesValues(t *testing.T) {
	startFanOutService(t)

	statusCode, body, errBody, err := postFanOut(t, 3, 10)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Empty(t, errBody.Error)
	assert.Equal(t, 3, body.Destinations)
	assert.Equal(t, 10, body.Count)
	assert.Equal(t, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, body.Values)
}

func TestFanOutSingleDestination(t *testing.T) {
	startFanOutService(t)

	statusCode, body, errBody, err := postFanOut(t, 1, 5)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Empty(t, errBody.Error)
	assert.Equal(t, 1, body.Destinations)
	assert.Equal(t, 5, body.Count)
	assert.Equal(t, []int{0, 1, 2, 3, 4}, body.Values)
}

func TestFanOutBadRequest(t *testing.T) {
	startFanOutService(t)

	statusCode, body, errBody, err := postFanOut(t, 0, 5)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode)
	assert.Equal(t, "bad request", errBody.Error)
	assert.Empty(t, body.Values)
}
