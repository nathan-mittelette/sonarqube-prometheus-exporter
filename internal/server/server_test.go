package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/axopen/sonarqube-prometheus-exporter/internal/metrics"
	"github.com/axopen/sonarqube-prometheus-exporter/internal/sonarqube"
)

func TestNew(t *testing.T) {
	client := sonarqube.NewClient("https://sonar.example.com", "test-token")
	collector := metrics.NewCollector(client)

	srv := New("localhost:9090", collector)

	if srv == nil {
		t.Fatal("Expected server to be created, got nil")
	}

	if srv.httpServer == nil {
		t.Error("Expected httpServer to be initialized")
	}

	if srv.registry == nil {
		t.Error("Expected registry to be initialized")
	}

	if srv.httpServer.Addr != "localhost:9090" {
		t.Errorf("Expected server address 'localhost:9090', got: %s", srv.httpServer.Addr)
	}
}

func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	healthHandler(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got: %d", http.StatusOK, resp.StatusCode)
	}

	body := w.Body.String()
	if body != "OK" {
		t.Errorf("Expected body 'OK', got: %s", body)
	}
}

func TestRootHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	rootHandler(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got: %d", http.StatusOK, resp.StatusCode)
	}

	body := w.Body.String()

	// Check that the response contains expected content
	expectedStrings := []string{
		"SonarQube Prometheus Exporter",
		"/metrics",
		"/health",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(body, expected) {
			t.Errorf("Expected body to contain '%s'", expected)
		}
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "text/html" {
		t.Errorf("Expected Content-Type 'text/html', got: %s", contentType)
	}
}

func TestServerEndpoints(t *testing.T) {
	client := sonarqube.NewClient("https://sonar.example.com", "test-token")
	collector := metrics.NewCollector(client)

	srv := New("localhost:0", collector)

	tests := []struct {
		name           string
		path           string
		expectedStatus int
	}{
		{
			name:           "root endpoint",
			path:           "/",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "health endpoint",
			path:           "/health",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "metrics endpoint",
			path:           "/metrics",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			w := httptest.NewRecorder()

			srv.httpServer.Handler.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status code %d, got: %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestServerShutdown(t *testing.T) {
	client := sonarqube.NewClient("https://sonar.example.com", "test-token")
	collector := metrics.NewCollector(client)

	srv := New("localhost:0", collector)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test shutdown without starting the server
	err := srv.Shutdown(ctx)
	if err != nil && err != http.ErrServerClosed {
		t.Errorf("Expected no error or ErrServerClosed, got: %v", err)
	}
}
