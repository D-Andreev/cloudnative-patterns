// Dummy HTTP server that uses future. Called from e2e tests.
package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	future "github.com/D-Andreev/cloudnative-patterns/pkg/future"
)

type reqBody struct {
	DelaysMs []int `json:"delaysMs"`
	FailOn   *int  `json:"failOn,omitempty"`
}

type respBody struct {
	Results   []int `json:"results"`
	ElapsedMs int   `json:"elapsedMs"`
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
	http.HandleFunc("/future", func(w http.ResponseWriter, r *http.Request) {
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
		if err := json.Unmarshal(body, &req); err != nil || len(req.DelaysMs) < 1 {
			writeJSON(w, http.StatusBadRequest, errorBody{Error: "bad request"})
			return
		}
		for _, delay := range req.DelaysMs {
			if delay < 0 {
				writeJSON(w, http.StatusBadRequest, errorBody{Error: "bad request"})
				return
			}
		}
		if req.FailOn != nil && (*req.FailOn < 0 || *req.FailOn >= len(req.DelaysMs)) {
			writeJSON(w, http.StatusBadRequest, errorBody{Error: "bad request"})
			return
		}

		ctx := r.Context()
		start := time.Now()

		futures := make([]future.Future[int], len(req.DelaysMs))
		for i, delayMs := range req.DelaysMs {
			delayMs := delayMs
			index := i
			futures[i] = future.Async(ctx, func(ctx context.Context) (int, error) {
				select {
				case <-time.After(time.Duration(delayMs) * time.Millisecond):
					if req.FailOn != nil && *req.FailOn == index {
						return 0, context.Canceled
					}
					return delayMs, nil
				case <-ctx.Done():
					return 0, ctx.Err()
				}
			})
		}

		results := make([]int, len(futures))
		for i, f := range futures {
			result, err := f.Result()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, errorBody{Error: err.Error()})
				return
			}
			results[i] = result
		}

		writeJSON(w, http.StatusOK, respBody{
			Results:   results,
			ElapsedMs: int(time.Since(start).Milliseconds()),
		})
	})

	if err := http.ListenAndServe(":8097", nil); err != nil {
		panic(err)
	}
}
