package metrics

import (
	"log"
	"strconv"
	"strings"
	"sync"

	"github.com/axopen/sonarqube-prometheus-exporter/internal/sonarqube"
	"github.com/prometheus/client_golang/prometheus"
)

// Collector collects SonarQube metrics and exposes them in Prometheus format
type Collector struct {
	client      *sonarqube.Client
	projectInfo *prometheus.Desc
	metricDescs map[string]*prometheus.Desc
	mu          sync.RWMutex
}

// NewCollector creates a new Prometheus collector for SonarQube metrics
func NewCollector(client *sonarqube.Client) *Collector {
	return &Collector{
		client: client,
		projectInfo: prometheus.NewDesc(
			"sonarqube_project_info",
			"Information about SonarQube projects",
			[]string{"project_key", "project_name", "qualifier", "visibility"},
			nil,
		),
		metricDescs: make(map[string]*prometheus.Desc),
	}
}

// Describe sends the descriptors of each metric to the provided channel
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.projectInfo
}

// Collect is called by the Prometheus registry when collecting metrics
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Fetch available metrics from SonarQube
	metrics, err := c.client.GetMetrics()
	if err != nil {
		log.Printf("Error fetching metrics: %v", err)
		return
	}

	// Build list of numeric metric keys to fetch
	numericMetricKeys := c.getNumericMetricKeys(metrics)

	// Fetch all projects
	projects, err := c.client.GetProjects()
	if err != nil {
		log.Printf("Error fetching projects: %v", err)
		return
	}

	// For each project, fetch its measures and expose them
	for _, project := range projects {
		// Export project info metric
		ch <- prometheus.MustNewConstMetric(
			c.projectInfo,
			prometheus.GaugeValue,
			1,
			project.Key,
			project.Name,
			project.Qualifier,
			project.Visibility,
		)

		// Fetch measures for this project
		measures, err := c.client.GetProjectMeasures(project.Key, numericMetricKeys)
		if err != nil {
			log.Printf("Error fetching measures for project %s: %v", project.Key, err)
			continue
		}

		// Export each measure
		for _, measure := range measures {
			c.exportMeasure(ch, project.Key, project.Name, measure, metrics)
		}
	}
}

// getNumericMetricKeys returns the keys of metrics that have numeric values
func (c *Collector) getNumericMetricKeys(metrics []sonarqube.Metric) []string {
	var keys []string
	numericTypes := map[string]bool{
		"INT":      true,
		"FLOAT":    true,
		"PERCENT":  true,
		"RATING":   true,
		"MILLISEC": true,
		"WORK_DUR": true,
	}

	for _, metric := range metrics {
		if !metric.Hidden && numericTypes[metric.Type] {
			keys = append(keys, metric.Key)
		}
	}

	return keys
}

// exportMeasure exports a single measure as a Prometheus metric
func (c *Collector) exportMeasure(ch chan<- prometheus.Metric, projectKey, projectName string, measure sonarqube.Measure, allMetrics []sonarqube.Metric) {
	// Find the metric definition
	var metricDef *sonarqube.Metric
	for i := range allMetrics {
		if allMetrics[i].Key == measure.Metric {
			metricDef = &allMetrics[i]
			break
		}
	}

	if metricDef == nil {
		return
	}

	// Parse the value
	value, err := parseMetricValue(measure.Value, metricDef.Type)
	if err != nil {
		log.Printf("Error parsing value for metric %s: %v", measure.Metric, err)
		return
	}

	// Get or create metric descriptor
	desc := c.getOrCreateMetricDesc(metricDef)

	// Export the metric
	ch <- prometheus.MustNewConstMetric(
		desc,
		prometheus.GaugeValue,
		value,
		projectKey,
		projectName,
	)
}

// getOrCreateMetricDesc gets or creates a Prometheus metric descriptor
func (c *Collector) getOrCreateMetricDesc(metric *sonarqube.Metric) *prometheus.Desc {
	if desc, exists := c.metricDescs[metric.Key]; exists {
		return desc
	}

	// Sanitize metric name for Prometheus
	metricName := "sonarqube_" + sanitizeMetricName(metric.Key)

	desc := prometheus.NewDesc(
		metricName,
		metric.Description,
		[]string{"project_key", "project_name"},
		prometheus.Labels{"domain": metric.Domain},
	)

	c.metricDescs[metric.Key] = desc
	return desc
}

// sanitizeMetricName converts a SonarQube metric key to a valid Prometheus metric name
func sanitizeMetricName(key string) string {
	// Replace invalid characters with underscores
	key = strings.ReplaceAll(key, "-", "_")
	key = strings.ReplaceAll(key, ".", "_")
	key = strings.ToLower(key)
	return key
}

// parseMetricValue parses a metric value string to float64
func parseMetricValue(value, metricType string) (float64, error) {
	// Handle empty values - return 0 for metrics without values yet
	if value == "" {
		return 0, nil
	}

	switch metricType {
	case "INT", "MILLISEC":
		i, err := strconv.ParseInt(value, 10, 64)
		return float64(i), err
	case "FLOAT", "PERCENT":
		return strconv.ParseFloat(value, 64)
	case "RATING":
		// Ratings in SonarQube are A=1.0, B=2.0, C=3.0, D=4.0, E=5.0
		return strconv.ParseFloat(value, 64)
	case "WORK_DUR":
		// Work duration is in minutes
		i, err := strconv.ParseInt(value, 10, 64)
		return float64(i), err
	default:
		return 0, nil
	}
}
