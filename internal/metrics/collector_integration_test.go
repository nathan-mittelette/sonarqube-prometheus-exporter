package metrics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/axopen/sonarqube-prometheus-exporter/internal/sonarqube"
	"github.com/prometheus/client_golang/prometheus"
)

// TestCollect_Integration tests the full Collect workflow with a mock SonarQube server
func TestCollect_Integration(t *testing.T) {
	// Create mock SonarQube server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/api/metrics/search":
			response := sonarqube.MetricsResponse{
				Metrics: []sonarqube.Metric{
					{
						ID:          "1",
						Key:         "bugs",
						Type:        "INT",
						Name:        "Bugs",
						Description: "Number of bugs",
						Domain:      "Reliability",
						Hidden:      false,
					},
					{
						ID:          "2",
						Key:         "coverage",
						Type:        "PERCENT",
						Name:        "Coverage",
						Description: "Test coverage",
						Domain:      "Coverage",
						Hidden:      false,
					},
				},
				Total: 2,
			}
			json.NewEncoder(w).Encode(response)

		case "/api/components/search_projects":
			response := sonarqube.ComponentsResponse{
				Paging: sonarqube.Paging{
					PageIndex: 1,
					PageSize:  500,
					Total:     2,
				},
				Components: []sonarqube.Component{
					{
						Key:        "project1",
						Name:       "Project 1",
						Qualifier:  "TRK",
						Visibility: "private",
						Tags:       []string{"tag1"},
					},
					{
						Key:        "project2",
						Name:       "Project 2",
						Qualifier:  "TRK",
						Visibility: "public",
						Tags:       []string{},
					},
				},
			}
			json.NewEncoder(w).Encode(response)

		case "/api/measures/component":
			component := r.URL.Query().Get("component")
			var measures []sonarqube.Measure

			if component == "project1" {
				measures = []sonarqube.Measure{
					{Metric: "bugs", Value: "5"},
					{Metric: "coverage", Value: "80.5"},
				}
			} else if component == "project2" {
				measures = []sonarqube.Measure{
					{Metric: "bugs", Value: "10"},
					{Metric: "coverage", Value: "65.0"},
				}
			}

			response := sonarqube.MeasuresResponse{
				Component: sonarqube.ComponentMeasures{
					Key:      component,
					Name:     "Test Project",
					Measures: measures,
				},
			}
			json.NewEncoder(w).Encode(response)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Create client and collector
	client := sonarqube.NewClient(server.URL, "test-token")
	collector := NewCollector(client)

	// Collect metrics
	ch := make(chan prometheus.Metric, 100)
	go func() {
		collector.Collect(ch)
		close(ch)
	}()

	// Count collected metrics
	count := 0
	for range ch {
		count++
	}

	// Expected metrics:
	// - 2 project_info metrics (one for each project)
	// - 2 bugs metrics (one for each project)
	// - 2 coverage metrics (one for each project)
	// Total: 6 metrics
	expectedCount := 6
	if count != expectedCount {
		t.Errorf("Expected %d metrics, got: %d", expectedCount, count)
	}
}

// TestCollect_MetricsError tests Collect when fetching metrics fails
func TestCollect_MetricsError(t *testing.T) {
	// Create mock server that returns error for metrics
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/metrics/search" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized"))
			return
		}
	}))
	defer server.Close()

	client := sonarqube.NewClient(server.URL, "invalid-token")
	collector := NewCollector(client)

	ch := make(chan prometheus.Metric, 100)
	go func() {
		collector.Collect(ch)
		close(ch)
	}()

	// Should not collect any metrics when fetching metrics fails
	count := 0
	for range ch {
		count++
	}

	if count != 0 {
		t.Errorf("Expected 0 metrics when metrics fetch fails, got: %d", count)
	}
}

// TestCollect_ProjectsError tests Collect when fetching projects fails
func TestCollect_ProjectsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/api/metrics/search" {
			response := sonarqube.MetricsResponse{
				Metrics: []sonarqube.Metric{
					{
						Key:    "bugs",
						Type:   "INT",
						Name:   "Bugs",
						Domain: "Reliability",
						Hidden: false,
					},
				},
				Total: 1,
			}
			json.NewEncoder(w).Encode(response)
		} else if r.URL.Path == "/api/components/search_projects" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized"))
		}
	}))
	defer server.Close()

	client := sonarqube.NewClient(server.URL, "test-token")
	collector := NewCollector(client)

	ch := make(chan prometheus.Metric, 100)
	go func() {
		collector.Collect(ch)
		close(ch)
	}()

	count := 0
	for range ch {
		count++
	}

	// Should not collect any metrics when fetching projects fails
	if count != 0 {
		t.Errorf("Expected 0 metrics when projects fetch fails, got: %d", count)
	}
}

// TestCollect_MeasuresError tests Collect when fetching measures for a project fails
func TestCollect_MeasuresError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/api/metrics/search":
			response := sonarqube.MetricsResponse{
				Metrics: []sonarqube.Metric{
					{
						Key:    "bugs",
						Type:   "INT",
						Name:   "Bugs",
						Domain: "Reliability",
						Hidden: false,
					},
				},
				Total: 1,
			}
			json.NewEncoder(w).Encode(response)

		case "/api/components/search_projects":
			response := sonarqube.ComponentsResponse{
				Paging: sonarqube.Paging{
					PageIndex: 1,
					PageSize:  500,
					Total:     1,
				},
				Components: []sonarqube.Component{
					{
						Key:        "project1",
						Name:       "Project 1",
						Qualifier:  "TRK",
						Visibility: "private",
					},
				},
			}
			json.NewEncoder(w).Encode(response)

		case "/api/measures/component":
			// Return error for measures
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("Project not found"))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := sonarqube.NewClient(server.URL, "test-token")
	collector := NewCollector(client)

	ch := make(chan prometheus.Metric, 100)
	go func() {
		collector.Collect(ch)
		close(ch)
	}()

	count := 0
	for range ch {
		count++
	}

	// Should still export project_info metric even if measures fail
	// Expected: 1 project_info metric
	if count != 1 {
		t.Errorf("Expected 1 metric (project_info), got: %d", count)
	}
}
