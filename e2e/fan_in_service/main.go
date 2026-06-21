// Dummy HTTP server that uses fan-in. Called from e2e tests.
package main

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"sync"

	fanin "github.com/D-Andreev/cloudnative-patterns/pkg/fan_in"
)

type reqBody struct {
	Sources         int `json:"sources"`
	ValuesPerSource int `json:"valuesPerSource"`
}

type respBody struct {
	Values []int `json:"values"`
	Count  int   `json:"count"`
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
	fi := fanin.NewFanIn[int]()

	http.HandleFunc("/fan-in", func(w http.ResponseWriter, r *http.Request) {
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
		if err := json.Unmarshal(body, &req); err != nil || req.Sources < 1 || req.ValuesPerSource < 1 {
			writeJSON(w, http.StatusBadRequest, errorBody{Error: "bad request"})
			return
		}

		sources := make([]fanin.Source[int], req.Sources)
		var wg sync.WaitGroup
		wg.Add(req.Sources)

		for i := range sources {
			ch := make(chan int)
			sources[i] = ch

			go func() {
				defer wg.Done()
				defer close(ch)

				for v := 1; v <= req.ValuesPerSource; v++ {
					ch <- v
				}
			}()
		}

		dest := fi.Funnel(sources...)

		values := make([]int, 0, req.Sources*req.ValuesPerSource)
		for v := range dest {
			values = append(values, v)
		}

		wg.Wait()
		slices.Sort(values)

		writeJSON(w, http.StatusOK, respBody{
			Values: values,
			Count:  len(values),
		})
	})

	if err := http.ListenAndServe(":8095", nil); err != nil {
		panic(err)
	}
}
