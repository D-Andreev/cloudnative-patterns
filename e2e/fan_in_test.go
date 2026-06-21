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

const fanInBaseURL = "http://localhost:8095"

var fanInServiceStarted bool

type FanInReqBody struct {
	Sources         int `json:"sources"`
	ValuesPerSource int `json:"valuesPerSource"`
}

type FanInRespBody struct {
	Values []int `json:"values"`
	Count  int   `json:"count"`
}

type FanInErrorBody struct {
	Error string `json:"error"`
}

func startFanInService(t *testing.T) {
	t.Helper()
	if fanInServiceStarted {
		return
	}

	cmd := exec.Command("go", "build", "-o", "fan-in-service", "./fan_in_service/main.go")
	if err := cmd.Run(); err != nil {
		log.Fatal("Failed to compile fan-in service: ", err.Error())
	}
	cmd = exec.Command("./fan-in-service")
	if err := cmd.Start(); err != nil {
		log.Fatal("ERR ", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			fanInServiceStarted = false
		}
	})
	fanInServiceStarted = true
	time.Sleep(2 * time.Second)
}

func postFanIn(t *testing.T, sources, valuesPerSource int) (int, FanInRespBody, FanInErrorBody, error) {
	t.Helper()
	payload, err := json.Marshal(FanInReqBody{
		Sources:         sources,
		ValuesPerSource: valuesPerSource,
	})
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, fanInBaseURL+"/fan-in", bytes.NewBuffer(payload))
	if err != nil {
		return 0, FanInRespBody{}, FanInErrorBody{}, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, FanInRespBody{}, FanInErrorBody{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, FanInRespBody{}, FanInErrorBody{}, err
	}

	if resp.StatusCode == http.StatusOK {
		var ok FanInRespBody
		err = json.Unmarshal(body, &ok)
		return resp.StatusCode, ok, FanInErrorBody{}, err
	}

	var errBody FanInErrorBody
	err = json.Unmarshal(body, &errBody)
	return resp.StatusCode, FanInRespBody{}, errBody, err
}

func TestFanInMergesMultipleSources(t *testing.T) {
	startFanInService(t)

	statusCode, body, errBody, err := postFanIn(t, 3, 5)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Empty(t, errBody.Error)
	assert.Equal(t, 15, body.Count)
	assert.Equal(t, []int{1, 1, 1, 2, 2, 2, 3, 3, 3, 4, 4, 4, 5, 5, 5}, body.Values)
}

func TestFanInSingleSource(t *testing.T) {
	startFanInService(t)

	statusCode, body, errBody, err := postFanIn(t, 1, 3)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Empty(t, errBody.Error)
	assert.Equal(t, 3, body.Count)
	assert.Equal(t, []int{1, 2, 3}, body.Values)
}

func TestFanInBadRequest(t *testing.T) {
	startFanInService(t)

	statusCode, body, errBody, err := postFanIn(t, 0, 5)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode)
	assert.Equal(t, "bad request", errBody.Error)
	assert.Empty(t, body.Values)
}
