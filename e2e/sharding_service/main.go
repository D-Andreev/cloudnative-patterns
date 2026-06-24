// Dummy HTTP server that uses sharding. Called from e2e tests.
package main

import (
	"encoding/json"
	"io"
	"net/http"

	sharding "github.com/D-Andreev/cloudnative-patterns/pkg/sharding"
)

type reqBody struct {
	Shards  int               `json:"shards"`
	Entries map[string]string `json:"entries"`
	Delete  []string          `json:"delete"`
}

type respBody struct {
	Values              map[string]string `json:"values"`
	ContainsBeforeDelete map[string]bool  `json:"containsBeforeDelete"`
	ContainsAfterDelete  map[string]bool  `json:"containsAfterDelete"`
}

type errorBody struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func main() {
	http.HandleFunc("/sharding", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, errorBody{Error: "method not allowed"})
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorBody{Error: "bad request"})
			return
		}

		var req reqBody
		if err := json.Unmarshal(body, &req); err != nil || req.Shards < 1 || len(req.Entries) < 1 {
			writeJSON(w, http.StatusBadRequest, errorBody{Error: "bad request"})
			return
		}

		sm := sharding.NewShardedMap[string, string](req.Shards)

		for key, value := range req.Entries {
			sm.Set(key, value)
		}

		values := make(map[string]string, len(req.Entries))
		containsBeforeDelete := make(map[string]bool, len(req.Entries))
		for key := range req.Entries {
			values[key] = sm.Get(key)
			containsBeforeDelete[key] = sm.Contains(key)
		}

		for _, key := range req.Delete {
			sm.Delete(key)
		}

		containsAfterDelete := make(map[string]bool, len(req.Entries))
		for key := range req.Entries {
			containsAfterDelete[key] = sm.Contains(key)
		}

		writeJSON(w, http.StatusOK, respBody{
			Values:               values,
			ContainsBeforeDelete: containsBeforeDelete,
			ContainsAfterDelete:  containsAfterDelete,
		})
	})

	if err := http.ListenAndServe(":8098", nil); err != nil {
		panic(err)
	}
}
