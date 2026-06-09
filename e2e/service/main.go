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

	circuitbreaker "github.com/cloudnative-patterns/pkg/circuit_breaker"
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
var b = circuitbreaker.NewBreaker[string](isFailure, 3)
var helloWithBreaker = b.BreakerFn(hello)

type ReqBody struct {
	Input string
}

type RespBody struct {
	Message string
}

func hello(ctx context.Context) (string, error) {
	body := ctx.Value("body")
	var r io.Reader = strings.NewReader(body.(string))
	decoder := json.NewDecoder(r)
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
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		bytedata, err := io.ReadAll(r.Body)
		reqBodyString := string(bytedata)
		c := context.WithValue(context.Background(), "body", reqBodyString)
		res, err := helloWithBreaker(c)

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
