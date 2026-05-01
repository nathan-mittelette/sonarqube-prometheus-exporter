package metrics

import (
	"context"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/axopen/sonarqube-prometheus-exporter/internal/sonarqube"
	"github.com/prometheus/client_golang/prometheus"
)

// Collector collects SonarQube metrics and exposes them in Prometheus format
type Collector struct {
	client          *sonarqube.Client
	projectInfo     *prometheus.Desc
	metricDescs     map[string]*prometheus.Desc
	branchInfo      *prometheus.Desc
	pullRequestInfo *prometheus.Desc
	mu              sync.RWMutex

	// Async collection state
	config              Config
	cache               *MetricsCache
	cacheMu             sync.RWMutex
	refreshCtx          context.Context
	refreshCancel       context.CancelFunc
	refreshWg           sync.WaitGroup
	lastRefreshTime     time.Time
	lastRefreshError    error
	lastRefreshDuration time.Duration
}

// Config holds collector configuration
type Config struct {
	CollectBranches     bool
	CollectPullRequests bool
	RefreshInterval     time.Duration
}

// MetricsCache holds cached metrics data
type MetricsCache struct {
	Metrics           []sonarqube.Metric
	Projects          []sonarqube.Component
	ProjectMeasures   map[string][]sonarqube.Measure            // projectKey -> measures
	Branches          map[string][]sonarqube.Branch             // projectKey -> branches
	BranchMeasures    map[string]map[string][]sonarqube.Measure // projectKey -> branchName -> measures
	PullRequests      map[string][]sonarqube.PullRequest        // projectKey -> pull requests
	PRMeasures        map[string]map[string][]sonarqube.Measure // projectKey -> prKey -> measures
	NumericMetricKeys []string
	LastUpdated       time.Time
}

// NewCollector creates a new Prometheus collector for SonarQube metrics
func NewCollector(client *sonarqube.Client, cfg Config) *Collector {
	c := &Collector{
		client: client,
		config: cfg,
		projectInfo: prometheus.NewDesc(
			"sonarqube_project_info",
			"Information about SonarQube projects",
			[]string{"project_key", "project_name", "qualifier", "visibility"},
			nil,
		),
		branchInfo: prometheus.NewDesc(
			"sonarqube_branch_info",
			"Information about SonarQube branches",
			[]string{"project_key", "project_name", "branch_key", "branch_name", "is_main"},
			nil,
		),
		pullRequestInfo: prometheus.NewDesc(
			"sonarqube_pull_request_info",
			"Information about SonarQube pull requests",
			[]string{"project_key", "project_name", "pr_key", "pr_title", "pr_status", "pr_branch", "pr_target"},
			nil,
		),
		metricDescs: make(map[string]*prometheus.Desc),
		cache: &MetricsCache{
			ProjectMeasures: make(map[string][]sonarqube.Measure),
			Branches:        make(map[string][]sonarqube.Branch),
			BranchMeasures:  make(map[string]map[string][]sonarqube.Measure),
			PullRequests:    make(map[string][]sonarqube.PullRequest),
			PRMeasures:      make(map[string]map[string][]sonarqube.Measure),
		},
	}

	// Start async refresh if interval is configured
	if cfg.RefreshInterval > 0 {
		c.StartAsyncRefresh()
	}

	return c
}

// StartAsyncRefresh starts the background refresh goroutine
func (c *Collector) StartAsyncRefresh() {
	c.refreshCtx, c.refreshCancel = context.WithCancel(context.Background())
	c.refreshWg.Add(1)
	go c.refreshLoop()
}

// StopAsyncRefresh stops the background refresh goroutine
func (c *Collector) StopAsyncRefresh() {
	if c.refreshCancel != nil {
		c.refreshCancel()
		c.refreshWg.Wait()
	}
}

// refreshLoop runs the background refresh loop
func (c *Collector) refreshLoop() {
	defer c.refreshWg.Done()

	ticker := time.NewTicker(c.config.RefreshInterval)
	defer ticker.Stop()

	// Initial refresh
	c.refreshMetrics()

	for {
		select {
		case <-c.refreshCtx.Done():
			return
		case <-ticker.C:
			c.refreshMetrics()
		}
	}
}

// refreshMetrics fetches all metrics asynchronously and updates the cache
func (c *Collector) refreshMetrics() {
	startTime := time.Now()
	c.cacheMu.Lock()
	c.lastRefreshTime = startTime
	c.cacheMu.Unlock()

	// Create a new cache to avoid partial updates
	newCache := &MetricsCache{
		ProjectMeasures: make(map[string][]sonarqube.Measure),
		Branches:        make(map[string][]sonarqube.Branch),
		BranchMeasures:  make(map[string]map[string][]sonarqube.Measure),
		PullRequests:    make(map[string][]sonarqube.PullRequest),
		PRMeasures:      make(map[string]map[string][]sonarqube.Measure),
		LastUpdated:     startTime,
	}

	// Fetch metrics
	metrics, err := c.client.GetMetrics()
	if err != nil {
		c.cacheMu.Lock()
		c.lastRefreshError = err
		c.lastRefreshDuration = time.Since(startTime)
		c.cacheMu.Unlock()
		log.Printf("Error refreshing metrics: %v", err)
		return
	}
	newCache.Metrics = metrics
	newCache.NumericMetricKeys = c.getNumericMetricKeys(metrics)

	// Fetch projects
	projects, err := c.client.GetProjects()
	if err != nil {
		c.cacheMu.Lock()
		c.lastRefreshError = err
		c.lastRefreshDuration = time.Since(startTime)
		c.cacheMu.Unlock()
		log.Printf("Error refreshing projects: %v", err)
		return
	}
	newCache.Projects = projects

	// Fetch project measures
	var wg sync.WaitGroup
	for _, project := range projects {
		wg.Add(1)
		go func(p sonarqube.Component) {
			defer wg.Done()
			measures, err := c.client.GetProjectMeasures(p.Key, newCache.NumericMetricKeys)
			if err != nil {
				log.Printf("Error fetching measures for project %s: %v", p.Key, err)
				return
			}
			newCache.ProjectMeasures[p.Key] = measures
		}(project)
	}

	// Fetch branches if enabled
	if c.config.CollectBranches {
		for _, project := range projects {
			wg.Add(1)
			go func(p sonarqube.Component) {
				defer wg.Done()
				branches, err := c.client.GetProjectBranches(p.Key)
				if err != nil {
					log.Printf("Error fetching branches for project %s: %v", p.Key, err)
					return
				}
				newCache.Branches[p.Key] = branches

				// Initialize branch measures map
				newCache.BranchMeasures[p.Key] = make(map[string][]sonarqube.Measure)

				// Fetch measures for each branch
				for _, branch := range branches {
					wg.Add(1)
					go func(projectKey string, b sonarqube.Branch) {
						defer wg.Done()
						measures, err := c.client.GetBranchMeasures(projectKey, b.Key, newCache.NumericMetricKeys)
						if err != nil {
							log.Printf("Error fetching measures for branch %s:%s: %v", projectKey, b.Key, err)
							return
						}
						newCache.BranchMeasures[projectKey][b.Key] = measures
					}(p.Key, branch)
				}
			}(project)
		}
	}

	// Fetch pull requests if enabled
	if c.config.CollectPullRequests {
		for _, project := range projects {
			wg.Add(1)
			go func(p sonarqube.Component) {
				defer wg.Done()
				prs, err := c.client.GetProjectPullRequests(p.Key)
				if err != nil {
					log.Printf("Error fetching pull requests for project %s: %v", p.Key, err)
					return
				}
				newCache.PullRequests[p.Key] = prs

				// Initialize PR measures map
				newCache.PRMeasures[p.Key] = make(map[string][]sonarqube.Measure)

				// Fetch measures for each pull request
				for _, pr := range prs {
					wg.Add(1)
					go func(projectKey string, pr sonarqube.PullRequest) {
						defer wg.Done()
						measures, err := c.client.GetPullRequestMeasures(projectKey, pr.Key, newCache.NumericMetricKeys)
						if err != nil {
							log.Printf("Error fetching measures for pull request %s:%s: %v", projectKey, pr.Key, err)
							return
						}
						newCache.PRMeasures[projectKey][pr.Key] = measures
					}(p.Key, pr)
				}
			}(project)
		}
	}

	// Wait for all fetches to complete
	wg.Wait()

	// Update cache atomically
	c.cacheMu.Lock()
	c.cache = newCache
	c.lastRefreshError = nil
	c.lastRefreshDuration = time.Since(startTime)
	c.lastRefreshTime = startTime
	c.cacheMu.Unlock()

	log.Printf("Metrics cache refreshed successfully in %v", c.lastRefreshDuration)
}

// Describe sends the descriptors of each metric to the provided channel
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.projectInfo
	if c.config.CollectBranches {
		ch <- c.branchInfo
	}
	if c.config.CollectPullRequests {
		ch <- c.pullRequestInfo
	}
}

// Collect is called by the Prometheus registry when collecting metrics
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If async refresh is enabled, use cached data
	if c.config.RefreshInterval > 0 {
		c.collectFromCache(ch)
		return
	}

	// Otherwise, fetch synchronously (old behavior)
	c.collectSync(ch)
}

// collectFromCache collects metrics from the cache
func (c *Collector) collectFromCache(ch chan<- prometheus.Metric) {
	c.cacheMu.RLock()
	defer c.cacheMu.RUnlock()

	if c.cache == nil {
		return
	}

	// Export project info and measures
	for _, project := range c.cache.Projects {
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

		// Export project measures
		if measures, exists := c.cache.ProjectMeasures[project.Key]; exists {
			for _, measure := range measures {
				c.exportMeasure(ch, project.Key, project.Name, measure, c.cache.Metrics, "project")
			}
		}
	}

	// Export branch info and measures if enabled
	if c.config.CollectBranches {
		for projectKey, branches := range c.cache.Branches {
			projectName := c.getProjectName(projectKey)
			for _, branch := range branches {
				// Export branch info metric
				isMain := "false"
				if branch.IsMain {
					isMain = "true"
				}

				ch <- prometheus.MustNewConstMetric(
					c.branchInfo,
					prometheus.GaugeValue,
					1,
					projectKey,
					projectName,
					branch.Key,
					branch.Name,
					isMain,
				)

				// Export branch measures
				if branchMeasures, exists := c.cache.BranchMeasures[projectKey]; exists {
					if measures, exists := branchMeasures[branch.Key]; exists {
						for _, measure := range measures {
							c.exportMeasure(ch, projectKey, projectName, measure, c.cache.Metrics, "branch", branch.Key)
						}
					}
				}
			}
		}
	}

	// Export pull request info and measures if enabled
	if c.config.CollectPullRequests {
		for projectKey, prs := range c.cache.PullRequests {
			projectName := c.getProjectName(projectKey)
			for _, pr := range prs {
				// Export pull request info metric
				ch <- prometheus.MustNewConstMetric(
					c.pullRequestInfo,
					prometheus.GaugeValue,
					1,
					projectKey,
					projectName,
					pr.Key,
					pr.Title,
					pr.Status,
					pr.Branch,
					pr.Target,
				)

				// Export pull request measures
				if prMeasures, exists := c.cache.PRMeasures[projectKey]; exists {
					if measures, exists := prMeasures[pr.Key]; exists {
						for _, measure := range measures {
							c.exportMeasure(ch, projectKey, projectName, measure, c.cache.Metrics, "pull_request", pr.Key)
						}
					}
				}
			}
		}
	}
}

// collectSync collects metrics synchronously (old behavior)
func (c *Collector) collectSync(ch chan<- prometheus.Metric) {
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

	// For each project, fetch its measures and export them
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
			c.exportMeasure(ch, project.Key, project.Name, measure, metrics, "project")
		}

		// Fetch branches if enabled
		if c.config.CollectBranches {
			branches, err := c.client.GetProjectBranches(project.Key)
			if err != nil {
				log.Printf("Error fetching branches for project %s: %v", project.Key, err)
			} else {
				for _, branch := range branches {
					// Export branch info metric
					isMain := "false"
					if branch.IsMain {
						isMain = "true"
					}

					ch <- prometheus.MustNewConstMetric(
						c.branchInfo,
						prometheus.GaugeValue,
						1,
						project.Key,
						project.Name,
						branch.Key,
						branch.Name,
						isMain,
					)

					// Fetch measures for this branch
					branchMeasures, err := c.client.GetBranchMeasures(project.Key, branch.Key, numericMetricKeys)
					if err != nil {
						log.Printf("Error fetching measures for branch %s:%s: %v", project.Key, branch.Key, err)
						continue
					}

					// Export each branch measure
					for _, measure := range branchMeasures {
						c.exportMeasure(ch, project.Key, project.Name, measure, metrics, "branch", branch.Key)
					}
				}
			}
		}

		// Fetch pull requests if enabled
		if c.config.CollectPullRequests {
			prs, err := c.client.GetProjectPullRequests(project.Key)
			if err != nil {
				log.Printf("Error fetching pull requests for project %s: %v", project.Key, err)
			} else {
				for _, pr := range prs {
					// Export pull request info metric
					ch <- prometheus.MustNewConstMetric(
						c.pullRequestInfo,
						prometheus.GaugeValue,
						1,
						project.Key,
						project.Name,
						pr.Key,
						pr.Title,
						pr.Status,
						pr.Branch,
						pr.Target,
					)

					// Fetch measures for this pull request
					prMeasures, err := c.client.GetPullRequestMeasures(project.Key, pr.Key, numericMetricKeys)
					if err != nil {
						log.Printf("Error fetching measures for pull request %s:%s: %v", project.Key, pr.Key, err)
						continue
					}

					// Export each pull request measure
					for _, measure := range prMeasures {
						c.exportMeasure(ch, project.Key, project.Name, measure, metrics, "pull_request", pr.Key)
					}
				}
			}
		}
	}
}

// getProjectName returns the project name from cache or a default
func (c *Collector) getProjectName(projectKey string) string {
	for _, p := range c.cache.Projects {
		if p.Key == projectKey {
			return p.Name
		}
	}
	return projectKey
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
func (c *Collector) exportMeasure(ch chan<- prometheus.Metric, projectKey, projectName string, measure sonarqube.Measure, allMetrics []sonarqube.Metric, entityType string, entityKey ...string) {
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
	desc := c.getOrCreateMetricDesc(metricDef, entityType)

	// Build labels based on entity type
	var labels []string
	switch entityType {
	case "project":
		labels = []string{projectKey, projectName}
	case "branch":
		if len(entityKey) > 0 {
			labels = []string{projectKey, projectName, entityKey[0]}
		} else {
			labels = []string{projectKey, projectName, ""}
		}
	case "pull_request":
		if len(entityKey) > 0 {
			labels = []string{projectKey, projectName, entityKey[0]}
		} else {
			labels = []string{projectKey, projectName, ""}
		}
	default:
		labels = []string{projectKey, projectName}
	}

	// Export the metric
	ch <- prometheus.MustNewConstMetric(
		desc,
		prometheus.GaugeValue,
		value,
		labels...,
	)
}

// getOrCreateMetricDesc gets or creates a Prometheus metric descriptor
func (c *Collector) getOrCreateMetricDesc(metric *sonarqube.Metric, entityType string) *prometheus.Desc {
	key := metric.Key + "_" + entityType

	if desc, exists := c.metricDescs[key]; exists {
		return desc
	}

	// Sanitize metric name for Prometheus
	metricName := "sonarqube_" + sanitizeMetricName(metric.Key)

	// Add entity type prefix if not project
	if entityType != "project" {
		metricName = "sonarqube_" + entityType + "_" + sanitizeMetricName(metric.Key)
	}

	desc := prometheus.NewDesc(
		metricName,
		metric.Description,
		c.getLabelNames(entityType),
		prometheus.Labels{"domain": metric.Domain, "type": entityType},
	)

	c.metricDescs[key] = desc
	return desc
}

// getLabelNames returns label names based on entity type
func (c *Collector) getLabelNames(entityType string) []string {
	switch entityType {
	case "project":
		return []string{"project_key", "project_name"}
	case "branch":
		return []string{"project_key", "project_name", "branch_key"}
	case "pull_request":
		return []string{"project_key", "project_name", "pr_key"}
	default:
		return []string{"project_key", "project_name"}
	}
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
