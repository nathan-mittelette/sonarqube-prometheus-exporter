package metrics

import (
	"testing"

	"github.com/axopen/sonarqube-prometheus-exporter/internal/sonarqube"
	"github.com/prometheus/client_golang/prometheus"
)

func TestSanitizeMetricName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "new_bugs",
			expected: "new_bugs",
		},
		{
			input:    "code-smells",
			expected: "code_smells",
		},
		{
			input:    "CXX-public_api",
			expected: "cxx_public_api",
		},
		{
			input:    "sqale.index",
			expected: "sqale_index",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeMetricName(tt.input)
			if result != tt.expected {
				t.Errorf("Expected '%s', got: '%s'", tt.expected, result)
			}
		})
	}
}

func TestParseMetricValue(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		metricType string
		expected   float64
		shouldErr  bool
	}{
		{
			name:       "parse INT",
			value:      "42",
			metricType: "INT",
			expected:   42.0,
			shouldErr:  false,
		},
		{
			name:       "parse FLOAT",
			value:      "3.14",
			metricType: "FLOAT",
			expected:   3.14,
			shouldErr:  false,
		},
		{
			name:       "parse PERCENT",
			value:      "85.5",
			metricType: "PERCENT",
			expected:   85.5,
			shouldErr:  false,
		},
		{
			name:       "parse RATING",
			value:      "2.0",
			metricType: "RATING",
			expected:   2.0,
			shouldErr:  false,
		},
		{
			name:       "parse MILLISEC",
			value:      "1000",
			metricType: "MILLISEC",
			expected:   1000.0,
			shouldErr:  false,
		},
		{
			name:       "parse WORK_DUR",
			value:      "120",
			metricType: "WORK_DUR",
			expected:   120.0,
			shouldErr:  false,
		},
		{
			name:       "invalid INT",
			value:      "not-a-number",
			metricType: "INT",
			expected:   0,
			shouldErr:  true,
		},
		{
			name:       "invalid FLOAT",
			value:      "not-a-number",
			metricType: "FLOAT",
			expected:   0,
			shouldErr:  true,
		},
		{
			name:       "empty value INT",
			value:      "",
			metricType: "INT",
			expected:   0,
			shouldErr:  false,
		},
		{
			name:       "empty value PERCENT",
			value:      "",
			metricType: "PERCENT",
			expected:   0,
			shouldErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseMetricValue(tt.value, tt.metricType)

			if tt.shouldErr && err == nil {
				t.Error("Expected error, got nil")
			}

			if !tt.shouldErr && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}

			if !tt.shouldErr && result != tt.expected {
				t.Errorf("Expected %f, got: %f", tt.expected, result)
			}
		})
	}
}

func TestGetNumericMetricKeys(t *testing.T) {
	collector := &Collector{}

	metrics := []sonarqube.Metric{
		{Key: "bugs", Type: "INT", Hidden: false},
		{Key: "coverage", Type: "PERCENT", Hidden: false},
		{Key: "ncloc", Type: "INT", Hidden: false},
		{Key: "description", Type: "STRING", Hidden: false}, // Non-numeric, should be excluded
		{Key: "hidden_metric", Type: "INT", Hidden: true},   // Hidden, should be excluded
		{Key: "sqale_index", Type: "WORK_DUR", Hidden: false},
		{Key: "complexity", Type: "INT", Hidden: false},
	}

	keys := collector.getNumericMetricKeys(metrics)

	expectedCount := 5 // bugs, coverage, ncloc, sqale_index, complexity
	if len(keys) != expectedCount {
		t.Errorf("Expected %d numeric metric keys, got: %d", expectedCount, len(keys))
	}

	// Check that non-numeric and hidden metrics are excluded
	for _, key := range keys {
		if key == "description" {
			t.Error("Expected 'description' to be excluded (non-numeric)")
		}
		if key == "hidden_metric" {
			t.Error("Expected 'hidden_metric' to be excluded (hidden)")
		}
	}
}

func TestNewCollector(t *testing.T) {
	client := sonarqube.NewClient("https://sonar.example.com", "test-token")
	collector := NewCollector(client)

	if collector == nil {
		t.Fatal("Expected collector to be created, got nil")
	}

	if collector.client != client {
		t.Error("Expected collector to have the provided client")
	}

	if collector.projectInfo == nil {
		t.Error("Expected projectInfo descriptor to be initialized")
	}

	if collector.metricDescs == nil {
		t.Error("Expected metricDescs map to be initialized")
	}
}

func TestDescribe(t *testing.T) {
	client := sonarqube.NewClient("https://sonar.example.com", "test-token")
	collector := NewCollector(client)

	ch := make(chan *prometheus.Desc, 10)
	go func() {
		collector.Describe(ch)
		close(ch)
	}()

	count := 0
	for range ch {
		count++
	}

	if count != 1 {
		t.Errorf("Expected 1 descriptor, got: %d", count)
	}
}

func TestGetOrCreateMetricDesc(t *testing.T) {
	client := sonarqube.NewClient("https://sonar.example.com", "test-token")
	collector := NewCollector(client)

	metric := &sonarqube.Metric{
		Key:         "bugs",
		Name:        "Bugs",
		Description: "Number of bugs",
		Domain:      "Reliability",
	}

	// First call should create the descriptor
	desc1 := collector.getOrCreateMetricDesc(metric)
	if desc1 == nil {
		t.Fatal("Expected descriptor to be created, got nil")
	}

	// Second call should return the cached descriptor
	desc2 := collector.getOrCreateMetricDesc(metric)
	if desc1 != desc2 {
		t.Error("Expected cached descriptor to be returned")
	}

	// Check that the descriptor was cached
	if len(collector.metricDescs) != 1 {
		t.Errorf("Expected 1 cached descriptor, got: %d", len(collector.metricDescs))
	}
}

func TestExportMeasure(t *testing.T) {
	client := sonarqube.NewClient("https://sonar.example.com", "test-token")
	collector := NewCollector(client)

	allMetrics := []sonarqube.Metric{
		{
			Key:         "bugs",
			Type:        "INT",
			Name:        "Bugs",
			Description: "Number of bugs",
			Domain:      "Reliability",
		},
	}

	measure := sonarqube.Measure{
		Metric: "bugs",
		Value:  "10",
	}

	ch := make(chan prometheus.Metric, 10)
	collector.exportMeasure(ch, "project1", "Project 1", measure, allMetrics)
	close(ch)

	count := 0
	for range ch {
		count++
	}

	if count != 1 {
		t.Errorf("Expected 1 metric to be exported, got: %d", count)
	}
}

func TestExportMeasure_InvalidMetric(t *testing.T) {
	client := sonarqube.NewClient("https://sonar.example.com", "test-token")
	collector := NewCollector(client)

	allMetrics := []sonarqube.Metric{
		{
			Key:         "bugs",
			Type:        "INT",
			Name:        "Bugs",
			Description: "Number of bugs",
			Domain:      "Reliability",
		},
	}

	// Measure with non-existent metric
	measure := sonarqube.Measure{
		Metric: "non_existent",
		Value:  "10",
	}

	ch := make(chan prometheus.Metric, 10)
	collector.exportMeasure(ch, "project1", "Project 1", measure, allMetrics)
	close(ch)

	count := 0
	for range ch {
		count++
	}

	if count != 0 {
		t.Errorf("Expected 0 metrics to be exported for invalid metric, got: %d", count)
	}
}

func TestExportMeasure_InvalidValue(t *testing.T) {
	client := sonarqube.NewClient("https://sonar.example.com", "test-token")
	collector := NewCollector(client)

	allMetrics := []sonarqube.Metric{
		{
			Key:         "bugs",
			Type:        "INT",
			Name:        "Bugs",
			Description: "Number of bugs",
			Domain:      "Reliability",
		},
	}

	// Measure with invalid value
	measure := sonarqube.Measure{
		Metric: "bugs",
		Value:  "not-a-number",
	}

	ch := make(chan prometheus.Metric, 10)
	collector.exportMeasure(ch, "project1", "Project 1", measure, allMetrics)
	close(ch)

	count := 0
	for range ch {
		count++
	}

	// Should not export metric with invalid value
	if count != 0 {
		t.Errorf("Expected 0 metrics to be exported for invalid value, got: %d", count)
	}
}
