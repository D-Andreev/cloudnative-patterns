// Dummy HTTP server that uses debounce. Called from e2e tests.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"
	"time"

	debounce "github.com/D-Andreev/cloudnative-patterns/pkg/debounce"
)

const debounceDuration = 200 * time.Millisecond

type emptyRequest struct{}

type respBody struct {
	Message string `json:"message"`
	Calls   int    `json:"calls"`
}

type errorBody struct {
	Error string `json:"error"`
}

type service struct {
	firstCalls atomic.Int32
	lastCalls  atomic.Int32

	firstDeb *debounce.Debounce[emptyRequest, respBody]
	lastDeb  *debounce.Debounce[emptyRequest, respBody]

	callFirst debounce.Circuit[emptyRequest, respBody]
	callLast  debounce.Circuit[emptyRequest, respBody]
}

func newService() (*service, error) {
	s := &service{}
	if err := s.reset(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *service) reset() error {
	firstDeb, err := debounce.NewDebounce[emptyRequest, respBody](debounce.Settings{
		DebounceType: debounce.FunctionFirst,
		Duration:     debounceDuration,
	})
	if err != nil {
		return err
	}

	lastDeb, err := debounce.NewDebounce[emptyRequest, respBody](debounce.Settings{
		DebounceType: debounce.FunctionLast,
		Duration:     debounceDuration,
	})
	if err != nil {
		return err
	}

	s.firstCalls.Store(0)
	s.lastCalls.Store(0)
	s.firstDeb = firstDeb
	s.lastDeb = lastDeb

	s.callFirst = firstDeb.First(func(context.Context, emptyRequest) (respBody, error) {
		n := int(s.firstCalls.Add(1))
		return respBody{Message: "ok", Calls: n}, nil
	})
	s.callLast = lastDeb.Last(func(ctx context.Context, _ emptyRequest) (respBody, error) {
		n := int(s.lastCalls.Add(1))
		return respBody{Message: "ok", Calls: n}, nil
	})

	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func main() {
	svc, err := newService()
	if err != nil {
		panic(err)
	}

	http.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, errorBody{Error: "method not allowed"})
			return
		}
		if err := svc.reset(); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorBody{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, respBody{Message: "reset"})
	})

	http.HandleFunc("/debounce/first", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, errorBody{Error: "method not allowed"})
			return
		}
		res, err := svc.callFirst(r.Context(), emptyRequest{})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorBody{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	http.HandleFunc("/debounce/last", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, errorBody{Error: "method not allowed"})
			return
		}
		res, err := svc.callLast(r.Context(), emptyRequest{})
		if err != nil {
			if errors.Is(err, context.Canceled) {
				writeJSON(w, http.StatusConflict, errorBody{Error: err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, errorBody{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	if err := http.ListenAndServe(":8091", nil); err != nil {
		panic(err)
	}
}
