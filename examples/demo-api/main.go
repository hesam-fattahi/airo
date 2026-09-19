package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var requestDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name: "http_request_duration_seconds",
		Help: "HTTP request latency distribution in seconds",
		Buckets: []float64{
			0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025,
			0.05, 0.1, 0.25, 0.5, 1.0, 2.5,
		},
	},
	[]string{"path", "status"},
)

func init() {
	prometheus.MustRegister(requestDuration)
}

type messageResponse struct {
	Message string `json:"message"`
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/message", handleMessage)
	mux.HandleFunc("/healthz", handleHealth)
	mux.Handle("/metrics", promhttp.Handler())

	server := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Println("Demo API listening on :8080")

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server failed: %v", err)
	}
}

func handleMessage(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	status := http.StatusOK

	defer func() {
		requestDuration.WithLabelValues(
			"/api/v1/message",
			strconv.Itoa(status),
		).Observe(time.Since(start).Seconds())
	}()

	if r.Method != http.MethodGet {
		status = http.StatusMethodNotAllowed
		http.Error(w, "method not allowed", status)
		return
	}

	applyInjectedLatency()

	w.Header().Set("Content-Type", "application/json")

	response := messageResponse{
		Message: "hello from demo-api",
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("failed to write response: %v", err)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write([]byte("ok\n")); err != nil {
		log.Printf("failed to write health response: %v", err)
	}
}

func applyInjectedLatency() {
	data, err := os.ReadFile("/tmp/latency_ms")
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("failed to read latency configuration: %v", err)
		}
		return
	}

	value := strings.TrimSpace(string(data))
	if value == "" {
		return
	}

	latency, err := strconv.Atoi(value)
	if err != nil {
		log.Printf("invalid latency value in /tmp/latency_ms: %q", value)
		return
	}

	if latency <= 0 {
		return
	}

	time.Sleep(time.Duration(latency) * time.Millisecond)
}
