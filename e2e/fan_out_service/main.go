// Dummy HTTP server that uses fan-out. Called from e2e tests.
package main

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"sync"

	fanout "github.com/D-Andreev/cloudnative-patterns/pkg/fan_out"
)

type reqBody struct {
	Destinations int `json:"destinations"`
	Values       int `json:"values"`
}

type respBody struct {
	Values       []int `json:"values"`
	Count        int   `json:"count"`
	Destinations int   `json:"destinations"`
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
	fo := fanout.NewFanOut[int]()

	http.HandleFunc("/fan-out", func(w http.ResponseWriter, r *http.Request) {
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
		if err := json.Unmarshal(body, &req); err != nil || req.Destinations < 1 || req.Values < 1 {
			writeJSON(w, http.StatusBadRequest, errorBody{Error: "bad request"})
			return
		}

		source := make(chan int)
		dests := fo.Split(source, req.Destinations)

		var (
			mu     sync.Mutex
			values []int
			wg     sync.WaitGroup
		)

		for _, dest := range dests {
			wg.Add(1)
			go func(dest fanout.Destination[int]) {
				defer wg.Done()
				for v := range dest {
					mu.Lock()
					values = append(values, v)
					mu.Unlock()
				}
			}(dest)
		}

		for v := range req.Values {
			source <- v
		}
		close(source)
		wg.Wait()

		slices.Sort(values)

		writeJSON(w, http.StatusOK, respBody{
			Values:       values,
			Count:        len(values),
			Destinations: len(dests),
		})
	})

	if err := http.ListenAndServe(":8096", nil); err != nil {
		panic(err)
	}
}
