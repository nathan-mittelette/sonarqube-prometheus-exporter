package server

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server represents the HTTP server
type Server struct {
	httpServer *http.Server
	registry   *prometheus.Registry
}

// New creates a new HTTP server
func New(address string, collector prometheus.Collector) *Server {
	// Create a new Prometheus registry
	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)

	// Create HTTP mux
	mux := http.NewServeMux()

	// Add /metrics endpoint
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))

	// Add health check endpoint
	mux.HandleFunc("/health", healthHandler)

	// Add root endpoint
	mux.HandleFunc("/", rootHandler)

	return &Server{
		httpServer: &http.Server{
			Addr:         address,
			Handler:      mux,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		registry: registry,
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
