package sonarqube

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetMetrics_Success(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/metrics/search" {
			t.Errorf("Expected path '/api/metrics/search', got: %s", r.URL.Path)
		}

		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Expected Authorization header 'Bearer test-token', got: %s", r.Header.Get("Authorization"))
		}

		response := MetricsResponse{
			Metrics: []Metric{
				{
					ID:          "1",
					Key:         "bugs",
					Type:        "INT",
					Name:        "Bugs",
					Description: "Number of bugs",
					Domain:      "Reliability",
					Direction:   -1,
					Qualitative: false,
					Hidden:      false,
				},
			},
			Total: 1,
			P:     1,
			PS:    500,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	metrics, err := client.GetMetrics()

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(metrics) != 1 {
		t.Fatalf("Expected 1 metric, got: %d", len(metrics))
	}

	if metrics[0].Key != "bugs" {
		t.Errorf("Expected metric key 'bugs', got: %s", metrics[0].Key)
	}
}

func TestGetMetrics_Unauthorized(t *testing.T) {
	// Create mock server that returns 401
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "invalid-token")
	_, err := client.GetMetrics()

	if err == nil {
		t.Fatal("Expected error, got nil")
	}
}

func TestGetProjects_Success(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/components/search_projects" {
			t.Errorf("Expected path '/api/components/search_projects', got: %s", r.URL.Path)
		}

		response := ComponentsResponse{
			Paging: Paging{
				PageIndex: 1,
				PageSize:  500,
				Total:     2,
			},
			Components: []Component{
				{
					Key:        "project1",
					Name:       "Project 1",
					Qualifier:  "TRK",
					IsFavorite: false,
					Tags:       []string{"tag1"},
					Visibility: "private",
				},
				{
					Key:        "project2",
					Name:       "Project 2",
					Qualifier:  "TRK",
					IsFavorite: false,
					Tags:       []string{},
					Visibility: "public",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	projects, err := client.GetProjects()

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(projects) != 2 {
		t.Fatalf("Expected 2 projects, got: %d", len(projects))
	}

	if projects[0].Key != "project1" {
		t.Errorf("Expected project key 'project1', got: %s", projects[0].Key)
	}
}

func TestGetProjectMeasures_Success(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/measures/component" {
			t.Errorf("Expected path '/api/measures/component', got: %s", r.URL.Path)
		}

		response := MeasuresResponse{
			Component: ComponentMeasures{
				Key:  "project1",
				Name: "Project 1",
				Measures: []Measure{
					{
						Metric: "bugs",
						Value:  "10",
					},
					{
						Metric: "code_smells",
						Value:  "25",
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	measures, err := client.GetProjectMeasures("project1", []string{"bugs", "code_smells"})

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(measures) != 2 {
		t.Fatalf("Expected 2 measures, got: %d", len(measures))
	}

	if measures[0].Metric != "bugs" {
		t.Errorf("Expected metric 'bugs', got: %s", measures[0].Metric)
	}

	if measures[0].Value != "10" {
		t.Errorf("Expected value '10', got: %s", measures[0].Value)
	}
}

func TestGetProjectMeasures_EmptyMetricKeys(t *testing.T) {
	client := NewClient("https://sonar.example.com", "test-token")
	measures, err := client.GetProjectMeasures("project1", []string{})

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(measures) != 0 {
		t.Fatalf("Expected 0 measures, got: %d", len(measures))
	}
}

func TestNewClient(t *testing.T) {
	client := NewClient("https://sonar.example.com", "test-token")

	if client.baseURL != "https://sonar.example.com" {
		t.Errorf("Expected baseURL 'https://sonar.example.com', got: %s", client.baseURL)
	}

	if client.token != "test-token" {
		t.Errorf("Expected token 'test-token', got: %s", client.token)
	}

	if client.httpClient == nil {
		t.Error("Expected httpClient to be initialized")
	}
}
