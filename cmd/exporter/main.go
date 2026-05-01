package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/axopen/sonarqube-prometheus-exporter/internal/config"
	"github.com/axopen/sonarqube-prometheus-exporter/internal/metrics"
	"github.com/axopen/sonarqube-prometheus-exporter/internal/server"
	"github.com/axopen/sonarqube-prometheus-exporter/internal/sonarqube"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Starting SonarQube Prometheus Exporter")
	log.Printf("SonarQube URL: %s", cfg.SonarQubeURL)
	log.Printf("Server address: %s", cfg.Address())
	log.Printf("Collect branches: %v", cfg.CollectBranches)
	log.Printf("Collect pull requests: %v", cfg.CollectPullRequests)
	log.Printf("Refresh interval: %d seconds", cfg.RefreshInterval)

	// Create SonarQube client
	sqClient := sonarqube.NewClient(cfg.SonarQubeURL, cfg.SonarQubeToken)

	// Create Prometheus collector with configuration
	collectorConfig := metrics.Config{
		CollectBranches:     cfg.CollectBranches,
		CollectPullRequests: cfg.CollectPullRequests,
		RefreshInterval:     time.Duration(cfg.RefreshInterval) * time.Second,
	}
	collector := metrics.NewCollector(sqClient, collectorConfig)

	// Create HTTP server
	srv := server.New(cfg.Address(), collector)

	// Start server in a goroutine
	go func() {
		if err := srv.Start(); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	log.Printf("Server started successfully on %s", cfg.Address())
	log.Printf("Metrics available at http://%s/metrics", cfg.Address())

	// Wait for interrupt signal to gracefully shut down the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// Stop async refresh before shutdown
	if collectorConfig.RefreshInterval > 0 {
		collector.StopAsyncRefresh()
	}

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}
