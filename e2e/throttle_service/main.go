// Dummy HTTP server that uses throttle. Called from e2e tests.
package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	throttle "github.com/D-Andreev/cloudnative-patterns/pkg/throttle"
)

const (
	throttleMaximum  = 3
	throttleRefill   = 3
	throttleDuration = 100 * time.Millisecond
	replayMaximum    = 1
	replayRefill     = 1
	queueMaximum     = 1
	queueRefill      = 1
)

type ctxKey struct{}

type reqBody struct {
	Label string `json:"label"`
}

type respBody struct {
	Message string `json:"message"`
	Calls   int    `json:"calls"`
}

type errorBody struct {
	Error string `json:"error"`
}

type service struct {
	errorCalls atomic.Int32
	replayCalls atomic.Int32
	queueCalls  atomic.Int32

	callWithError  func(context.Context) (respBody, error)
	callWithReplay func(context.Context) (respBody, error)
	callWithQueue  func(context.Context) (respBody, error)
}

func newService() (*service, error) {
	s := &service{}
	if err := s.reset(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *service) reset() error {
	thError, err := throttle.NewThrottle[respBody](throttle.Settings{
		Maximum:  throttleMaximum,
		Duration: throttleDuration,
		Refill:   throttleRefill,
	})
	if err != nil {
		return err
	}

	thReplay, err := throttle.NewThrottle[respBody](throttle.Settings{
		Maximum:  replayMaximum,
		Duration: throttleDuration,
		Refill:   replayRefill,
	})
	if err != nil {
		return err
	}

	thQueue, err := throttle.NewThrottle[respBody](throttle.Settings{
		Maximum:  queueMaximum,
		Duration: throttleDuration,
		Refill:   queueRefill,
	})
	if err != nil {
		return err
	}

	s.errorCalls.Store(0)
	s.replayCalls.Store(0)
	s.queueCalls.Store(0)

	s.callWithError = thError.ThrottleFnWithError(func(context.Context) (respBody, error) {
		n := int(s.errorCalls.Add(1))
		return respBody{Message: "ok", Calls: n}, nil
	})
	s.callWithReplay = thReplay.ThrottleFnWithReplay(func(context.Context) (respBody, error) {
		n := int(s.replayCalls.Add(1))
		return respBody{Message: "ok", Calls: n}, nil
	})
	s.callWithQueue = thQueue.ThrottleFnWithQueue(func(ctx context.Context) (respBody, error) {
		n := int(s.queueCalls.Add(1))
		label, _ := ctx.Value(ctxKey{}).(string)
		return respBody{Message: label, Calls: n}, nil
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

	http.HandleFunc("/throttle", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, errorBody{Error: "method not allowed"})
			return
		}

		res, err := svc.callWithError(context.Background())
		if err != nil {
			if err.Error() == "too many calls" {
				writeJSON(w, http.StatusTooManyRequests, errorBody{Error: err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, errorBody{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	http.HandleFunc("/throttle/replay", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, errorBody{Error: "method not allowed"})
			return
		}

		res, err := svc.callWithReplay(context.Background())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorBody{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	http.HandleFunc("/throttle/queue", func(w http.ResponseWriter, r *http.Request) {
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
		if err := json.Unmarshal(body, &req); err != nil || req.Label == "" {
			writeJSON(w, http.StatusBadRequest, errorBody{Error: "bad request"})
			return
		}

		ctx := context.WithValue(context.Background(), ctxKey{}, req.Label)
		res, err := svc.callWithQueue(ctx)
		if err != nil {
			if err.Error() == "too many calls" {
				writeJSON(w, http.StatusTooManyRequests, errorBody{Error: err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, errorBody{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	if err := http.ListenAndServe(":8093", nil); err != nil {
		panic(err)
	}
}
