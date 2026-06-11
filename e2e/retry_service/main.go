// Dummy HTTP server that uses retry. Called from e2e tests.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	retry "github.com/D-Andreev/cloudnative-patterns/pkg/retry"
)

const (
	retryDelay   = 50 * time.Millisecond
	maxFailures  = 3
	errTemporary = "temporary"
)

type reqBody struct {
	FailUntil int `json:"failUntil"`
}

type respBody struct {
	Message  string `json:"message"`
	Attempts int    `json:"attempts"`
}

type errorBody struct {
	Error    string `json:"error"`
	Attempts int    `json:"attempts"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func main() {
	retrier, err := retry.NewRetry[respBody](retry.Settings{
		Delay:       retryDelay,
		MaxFailures: maxFailures,
	})
	if err != nil {
		panic(err)
	}

	http.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, errorBody{Error: "method not allowed"})
			return
		}
		writeJSON(w, http.StatusOK, respBody{Message: "reset"})
	})

	http.HandleFunc("/retry", func(w http.ResponseWriter, r *http.Request) {
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
		if err := json.Unmarshal(body, &req); err != nil || req.FailUntil < 1 {
			writeJSON(w, http.StatusBadRequest, errorBody{Error: "bad request"})
			return
		}

		failUntil := req.FailUntil
		var attempts int
		call := retrier.RetryFn(func(context.Context) (respBody, error) {
			attempts++
			if attempts < failUntil {
				return respBody{}, errors.New(errTemporary)
			}
			return respBody{Message: "ok", Attempts: attempts}, nil
		})

		res, err := call(r.Context())
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, errorBody{
				Error:    err.Error(),
				Attempts: attempts,
			})
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	if err := http.ListenAndServe(":8092", nil); err != nil {
		panic(err)
	}
}
