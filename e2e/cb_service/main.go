// This is a dummy http server that uses circuit breaker pattern. It's called from e2e tests.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	circuitbreaker "github.com/D-Andreev/cloudnative-patterns/pkg/circuit_breaker"
)

var isFailure = func(err error) bool {
	if err == nil {
		return false
	}
	if err.Error() == "504" {
		return true
	}

	return true
}
var settings = circuitbreaker.Settings{
	IsFailure: isFailure,
	Threshold: 3,
}

type HelloRequest struct {
	Body string
}

type ReqBody struct {
	Input string
}

type RespBody struct {
	Message string
}

func hello(ctx context.Context, req HelloRequest) (string, error) {
	decoder := json.NewDecoder(strings.NewReader(req.Body))
	var b ReqBody
	err := decoder.Decode(&b)
	if err != nil {
		return "bad request", errors.New("400")
	}
	if b.Input == "fail" {
		return "fail on purpose", errors.New("504")
	}

	return "success", nil
}

func main() {
	b, err := circuitbreaker.NewBreaker[HelloRequest, string](settings)
	if err != nil {
		panic(err)
	}
	helloWithBreaker := b.Wrap(hello)
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		bytedata, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad request")
			return
		}

		res, err := helloWithBreaker(r.Context(), HelloRequest{Body: string(bytedata)})

		if errors.Is(err, circuitbreaker.BreakerErrResponse) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusGatewayTimeout)
			resp := RespBody{
				Message: err.Error(),
			}
			json.NewEncoder(w).Encode(resp)
			return
		}
		statusCode := 200
		if err != nil {
			statusCode, err = strconv.Atoi(err.Error())
			if err != nil {
				panic("Could not parse status code")
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		resp := RespBody{
			Message: res,
		}
		json.NewEncoder(w).Encode(resp)
	})
	http.ListenAndServe(":8090", nil)
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(RespBody{Message: message})
}
