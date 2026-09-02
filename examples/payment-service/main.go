package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// Histogram recording HTTP request duration in seconds.
	requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency distribution in seconds",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5},
		},
		[]string{"path", "status"},
	)

	// Thread-safe artificial latency in milliseconds.
	chaosLatencyMs int64
)

func init() {
	prometheus.MustRegister(requestDuration)
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/pay", handlePay)
	mux.HandleFunc("/chaos/delay", handleChaosDelay)
	mux.HandleFunc("/healthz", handleHealth)
	mux.Handle("/metrics", promhttp.Handler())

	server := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Println("Payment Service starting on port 8080...")

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed to start: %v", err)
	}

}

func handlePay(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	status := http.StatusOK

	// Baseline normal response duration: 5ms to 15ms.
	baseDelay := time.Duration(5+rand.Intn(10)) * time.Millisecond
	time.Sleep(baseDelay)

	// Add injected chaos latency if enabled.
	extraDelay := atomic.LoadInt64(&chaosLatencyMs)
	if extraDelay > 0 {
		time.Sleep(time.Duration(extraDelay) * time.Millisecond)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if _, err := fmt.Fprintln(w, `{"status":"success","transaction_id":"tx_98765"}`); err != nil {
		log.Printf("Failed to write response: %v", err)
	}

	duration := time.Since(start).Seconds()

	requestDuration.WithLabelValues(
		"/api/v1/pay",
		strconv.Itoa(status),
	).Observe(duration)

}

func handleChaosDelay(w http.ResponseWriter, r *http.Request) {
	durationStr := r.URL.Query().Get("duration_ms")

	if durationStr == "" {
		atomic.StoreInt64(&chaosLatencyMs, 0)
		fmt.Fprintln(w, "Chaos delay disabled.")
		return
	}

	ms, err := strconv.ParseInt(durationStr, 10, 64)
	if err != nil || ms < 0 {
		http.Error(w, "duration_ms must be a non-negative integer", http.StatusBadRequest)
		return
	}

	atomic.StoreInt64(&chaosLatencyMs, ms)

	fmt.Fprintf(w, "Chaos delay set to %d ms\n", ms)

}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)

	if _, err := fmt.Fprintln(w, "ok"); err != nil {
		log.Printf("Failed to write health response: %v", err)
	}

}
