package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

type receivedEvent struct {
	ReceivedAt time.Time       `json:"received_at"`
	Payload    json.RawMessage `json:"payload"`
}

func main() {
	healthcheck := flag.Bool("healthcheck", false, "check whether the receiver is ready")
	flag.Parse()

	if *healthcheck {
		response, err := http.Get("http://127.0.0.1:8081/healthz")
		if err != nil {
			log.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			log.Fatalf("health check returned %s", response.Status)
		}
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.HandleFunc("POST /events", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
		if err != nil {
			http.Error(w, "event body too large or unreadable", http.StatusRequestEntityTooLarge)
			return
		}
		if !json.Valid(body) {
			http.Error(w, "event body must be valid JSON", http.StatusBadRequest)
			return
		}

		entry, err := json.Marshal(receivedEvent{
			ReceivedAt: time.Now().UTC(),
			Payload:    json.RawMessage(body),
		})
		if err != nil {
			http.Error(w, "could not encode event log entry", http.StatusInternalServerError)
			return
		}
		if _, err := os.Stdout.Write(append(entry, '\n')); err != nil {
			http.Error(w, "could not write event log entry", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})

	server := &http.Server{
		Addr:              ":8081",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("receiver listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}
