// Dummy HTTP server that uses timeout. Called from e2e tests.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	timeout "github.com/D-Andreev/cloudnative-patterns/pkg/timeout"
)

const callTimeout = 100 * time.Millisecond

type reqBody struct {
	DelayMs int `json:"delayMs"`
}

type respBody struct {
	Message string `json:"message"`
	Elapsed int    `json:"elapsedMs"`
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
	tm, err := timeout.NewTimeout[reqBody, respBody]()
	if err != nil {
		panic(err)
	}

	http.HandleFunc("/timeout", func(w http.ResponseWriter, r *http.Request) {
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
		if err := json.Unmarshal(body, &req); err != nil || req.DelayMs < 0 {
			writeJSON(w, http.StatusBadRequest, errorBody{Error: "bad request"})
			return
		}

		call := tm.TimeoutFn(func(req reqBody) (respBody, error) {
			start := time.Now()
			time.Sleep(time.Duration(req.DelayMs) * time.Millisecond)
			return respBody{
				Message: "ok",
				Elapsed: int(time.Since(start).Milliseconds()),
			}, nil
		})

		ctx, cancel := context.WithTimeout(r.Context(), callTimeout)
		defer cancel()

		res, err := call(ctx, req)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				writeJSON(w, http.StatusGatewayTimeout, errorBody{Error: err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, errorBody{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	if err := http.ListenAndServe(":8094", nil); err != nil {
		panic(err)
	}
}
