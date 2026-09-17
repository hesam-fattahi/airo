package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var requestDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name: "http_request_duration_seconds",
		Help: "HTTP request latency distribution in seconds",
		Buckets: []float64{
			0.0005, // 0.5ms
			0.001,  // 1ms
			0.0025, // 2.5ms
			0.005,  // 5ms
			0.01,   // 10ms
			0.025,  // 25ms
			0.05,   // 50ms — SLO threshold
			0.1,    // 100ms
			0.25,   // 250ms
			0.5,    // 500ms
			1.0,    // 1s
			2.5,    // 2.5s
		},
	},
	[]string{"path", "status"},
)

func init() {
	prometheus.MustRegister(requestDuration)
}

type paymentResponse struct {
	Status string `json:"status"`
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/pay", handlePay)
	mux.HandleFunc("/healthz", handleHealth)
	mux.Handle("/metrics", promhttp.Handler())

	server := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Println("Payment API listening on :8080")

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server failed: %v", err)
	}
}

func handlePay(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	status := http.StatusOK

	defer func() {
		requestDuration.WithLabelValues(
			"/api/v1/pay",
			strconv.Itoa(status),
		).Observe(time.Since(start).Seconds())
	}()

	if r.Method != http.MethodPost {
		status = http.StatusMethodNotAllowed
		http.Error(w, "method not allowed", status)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	response := paymentResponse{
		Status: "accepted",
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("failed to write payment response: %v", err)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write([]byte("ok\n")); err != nil {
		log.Printf("failed to write health response: %v", err)
	}
}
