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

const shardingBaseURL = "http://localhost:8098"

var shardingServiceStarted bool

type ShardingReqBody struct {
	Shards  int               `json:"shards"`
	Entries map[string]string `json:"entries"`
	Delete  []string          `json:"delete"`
}

type ShardingRespBody struct {
	Values               map[string]string `json:"values"`
	ContainsBeforeDelete map[string]bool   `json:"containsBeforeDelete"`
	ContainsAfterDelete  map[string]bool   `json:"containsAfterDelete"`
}

type ShardingErrorBody struct {
	Error string `json:"error"`
}

func startShardingService(t *testing.T) {
	t.Helper()
	if shardingServiceStarted {
		return
	}

	cmd := exec.Command("go", "build", "-o", "sharding-service", "./sharding_service/main.go")
	if err := cmd.Run(); err != nil {
		log.Fatal("Failed to compile sharding service: ", err.Error())
	}
	cmd = exec.Command("./sharding-service")
	if err := cmd.Start(); err != nil {
		log.Fatal("ERR ", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			shardingServiceStarted = false
		}
	})
	shardingServiceStarted = true
	time.Sleep(2 * time.Second)
}

func postSharding(t *testing.T, req ShardingReqBody) (int, ShardingRespBody, ShardingErrorBody, error) {
	t.Helper()
	payload, err := json.Marshal(req)
	require.NoError(t, err)

	httpReq, err := http.NewRequest(http.MethodPost, shardingBaseURL+"/sharding", bytes.NewBuffer(payload))
	if err != nil {
		return 0, ShardingRespBody{}, ShardingErrorBody{}, err
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return 0, ShardingRespBody{}, ShardingErrorBody{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, ShardingRespBody{}, ShardingErrorBody{}, err
	}

	if resp.StatusCode == http.StatusOK {
		var ok ShardingRespBody
		err = json.Unmarshal(body, &ok)
		return resp.StatusCode, ok, ShardingErrorBody{}, err
	}

	var errBody ShardingErrorBody
	err = json.Unmarshal(body, &errBody)
	return resp.StatusCode, ShardingRespBody{}, errBody, err
}

func TestShardingStoresAndRetrievesEntries(t *testing.T) {
	startShardingService(t)

	statusCode, body, errBody, err := postSharding(t, ShardingReqBody{
		Shards: 8,
		Entries: map[string]string{
			"user:1": "alice",
			"user:2": "bob",
			"user:3": "carol",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Empty(t, errBody.Error)
	assert.Equal(t, map[string]string{
		"user:1": "alice",
		"user:2": "bob",
		"user:3": "carol",
	}, body.Values)
	assert.Equal(t, map[string]bool{
		"user:1": true,
		"user:2": true,
		"user:3": true,
	}, body.ContainsBeforeDelete)
	assert.Equal(t, map[string]bool{
		"user:1": true,
		"user:2": true,
		"user:3": true,
	}, body.ContainsAfterDelete)
}

func TestShardingDeleteRemovesKey(t *testing.T) {
	startShardingService(t)

	statusCode, body, errBody, err := postSharding(t, ShardingReqBody{
		Shards: 4,
		Entries: map[string]string{
			"a": "alpha",
			"b": "beta",
		},
		Delete: []string{"a"},
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Empty(t, errBody.Error)
	assert.Equal(t, map[string]bool{
		"a": true,
		"b": true,
	}, body.ContainsBeforeDelete)
	assert.Equal(t, map[string]bool{
		"a": false,
		"b": true,
	}, body.ContainsAfterDelete)
}

func TestShardingBadRequest(t *testing.T) {
	startShardingService(t)

	statusCode, body, errBody, err := postSharding(t, ShardingReqBody{
		Shards:  0,
		Entries: map[string]string{"a": "alpha"},
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode)
	assert.Equal(t, "bad request", errBody.Error)
	assert.Empty(t, body.Values)
}
