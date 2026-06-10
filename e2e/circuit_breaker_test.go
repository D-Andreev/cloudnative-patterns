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
)

var serviceStarted = false

type ReqBody struct {
	Input string `json:"input"`
}

type RespBody struct {
	Message string `json:"message"`
}

func startService(t *testing.T) {
	t.Helper()
	if serviceStarted == true {
		return
	}

	cmd := exec.Command("go", "build", "-o", "cb_service", "./cb_service/main.go")
	if err := cmd.Run(); err != nil {
		log.Fatal("Failed to compile service: ", err.Error())
	}
	cmd = exec.Command("./cb_service")

	if err := cmd.Start(); err != nil {
		log.Fatal("ERR ", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			serviceStarted = false
		}
	})
	serviceStarted = true
	time.Sleep(2 * time.Second)
}

func callService(t *testing.T, jsonValue []byte) (int, RespBody, error) {
	t.Helper()
	req, err := http.NewRequest("POST", "http://localhost:8090/hello", bytes.NewBuffer(jsonValue))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	defer resp.Body.Close()

	respBody := RespBody{}
	err = json.Unmarshal([]byte(body), &respBody)

	return resp.StatusCode, respBody, err
}

func TestCircuitBreakerNoServiceFailures(t *testing.T) {
	startService(t)

	jsonValue, _ := json.Marshal(ReqBody{
		Input: "test",
	})
	for range 10 {
		statusCode, body, err := callService(t, jsonValue)
		assert.Equal(t, nil, err)
		assert.Equal(t, http.StatusOK, statusCode)
		assert.Equal(t, "success", body.Message)
	}
}

func TestCircuitBreakerOpenState(t *testing.T) {
	startService(t)

	// 3 consecutive fails, breaker will open circuit after that
	for range 3 {
		jsonValue, _ := json.Marshal(ReqBody{
			Input: "fail",
		})
		statusCode, body, err := callService(t, jsonValue)
		assert.Equal(t, nil, err)
		assert.Equal(t, http.StatusGatewayTimeout, statusCode)
		assert.Equal(t, "fail on purpose", body.Message)
	}

	// When making the 4th request we expect fallback from breaker, not making actual call
	jsonValue, _ := json.Marshal(ReqBody{
		Input: "fail",
	})
	statusCode, body, err := callService(t, jsonValue)
	assert.Equal(t, nil, err)
	assert.Equal(t, http.StatusGatewayTimeout, statusCode)
	assert.Equal(t, "service unavailable", body.Message)
}

func TestCircuitBreakerHalfOpenState(t *testing.T) {
	startService(t)

	// 3 consecutive fails, breaker will open circuit after that
	for range 3 {
		jsonValue, _ := json.Marshal(ReqBody{
			Input: "fail",
		})
		statusCode, body, err := callService(t, jsonValue)
		assert.Equal(t, nil, err)
		assert.Equal(t, http.StatusGatewayTimeout, statusCode)
		assert.Equal(t, "fail on purpose", body.Message)
	}

	// If we persist to make requests after some duration breaker changes to half open state and makes actual calls in case service has recovered
	time.Sleep(3 * time.Second)
	jsonValue, _ := json.Marshal(ReqBody{
		Input: "test",
	})
	statusCode, body, err := callService(t, jsonValue)
	assert.Equal(t, nil, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "success", body.Message)
}

func TestCircuitBreakerHalfOpenStateToOpenAgain(t *testing.T) {
	startService(t)

	// 3 consecutive fails, breaker will open circuit after that
	for range 3 {
		jsonValue, _ := json.Marshal(ReqBody{
			Input: "fail",
		})
		statusCode, body, err := callService(t, jsonValue)
		assert.Equal(t, nil, err)
		assert.Equal(t, http.StatusGatewayTimeout, statusCode)
		assert.Equal(t, "fail on purpose", body.Message)
	}

	// We wait 3 seconds so that breaker can go into half open state and requests fail again
	time.Sleep(3 * time.Second)
	jsonValue, _ := json.Marshal(ReqBody{
		Input: "fail",
	})
	statusCode, body, err := callService(t, jsonValue)
	assert.Equal(t, nil, err)
	assert.Equal(t, http.StatusGatewayTimeout, statusCode)
	assert.Equal(t, "fail on purpose", body.Message)

	// Finally we're in open state again and we get service unavailable
	statusCode, body, err = callService(t, jsonValue)
	assert.Equal(t, nil, err)
	assert.Equal(t, http.StatusGatewayTimeout, statusCode)
	assert.Equal(t, "service unavailable", body.Message)
}
