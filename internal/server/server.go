package server

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/axopen/sonarqube-prometheus-exporter/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server represents the HTTP server
type Server struct {
	httpServer  *http.Server
	registry    *prometheus.Registry
	collector   *metrics.Collector
	cacheReady  <-chan struct{}
	isAsyncMode bool
}

// New creates a new HTTP server
func New(address string, collector *metrics.Collector) *Server {
	// Create a new Prometheus registry
	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)

	// Check if we're in async mode
	isAsyncMode := collector != nil && collector.IsAsyncMode()

	// Create HTTP mux
	mux := http.NewServeMux()

	// Add /metrics endpoint
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))

	// Add health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if isAsyncMode {
			healthHandlerAsync(w, r, collector.CacheReady())
		} else {
			healthHandler(w, r)
		}
	})

	// Add root endpoint
	mux.HandleFunc("/", rootHandler)

	var cacheReady <-chan struct{}
	if collector != nil {
		cacheReady = collector.CacheReady()
	}

	// WriteTimeout is intentionally 0 (disabled) because in sync mode the /metrics
	// handler fetches all data from SonarQube before writing — this can take minutes
	// on large instances. A fixed WriteTimeout would cut the connection before the
	// response is sent. ReadTimeout still protects against slow/malicious clients.
	return &Server{
		httpServer: &http.Server{
			Addr:        address,
			Handler:     mux,
			ReadTimeout: 15 * time.Second,
			IdleTimeout: 60 * time.Second,
		},
		registry:    registry,
		collector:   collector,
		cacheReady:  cacheReady,
		isAsyncMode: isAsyncMode,
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	log.Printf("Starting server on %s", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	log.Println("Shutting down server...")
	return s.httpServer.Shutdown(ctx)
}

// healthHandlerAsync handles health check requests in async mode
// Returns 503 until cache is populated, then 200
func healthHandlerAsync(w http.ResponseWriter, r *http.Request, cacheReady <-chan struct{}) {
	select {
	case <-cacheReady:
		// Cache is ready
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	default:
		// Cache not ready yet
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Cache not ready"))
	}
}
