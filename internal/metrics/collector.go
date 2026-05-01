package metrics

import (
	"context"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/axopen/sonarqube-prometheus-exporter/internal/sonarqube"
	"github.com/prometheus/client_golang/prometheus"
)

// Collector collects SonarQube metrics and exposes them in Prometheus format
type Collector struct {
	client          *sonarqube.Client
	projectInfo     *prometheus.Desc
	branchInfo      *prometheus.Desc
	pullRequestInfo *prometheus.Desc

	// mu protects metricDescs and cache together: both are reset on each refresh
	// and read on each Collect, so they share a single lock to avoid deadlocks.
	mu          sync.RWMutex
	metricDescs map[string]*prometheus.Desc
	cache       *MetricsCache

	// Async collection state
	config              Config
	refreshCtx          context.Context
	refreshCancel       context.CancelFunc
	refreshWg           sync.WaitGroup
	lastRefreshTime     time.Time
	lastRefreshError    error
	lastRefreshDuration time.Duration
	cacheReady          chan struct{} // Closed when first cache population is complete
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
			[]string{"project_key", "project_name", "pr_key", "pr_status", "pr_branch", "pr_target"},
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
		cacheReady: make(chan struct{}),
	}

	// Start async refresh if interval is configured
	if cfg.RefreshInterval > 0 {
		c.StartAsyncRefresh()
	} else {
		// In sync mode, cache is always ready
		close(c.cacheReady)
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

// CacheReady returns a channel that is closed when the cache is first populated
func (c *Collector) CacheReady() <-chan struct{} {
	return c.cacheReady
}

// IsAsyncMode reports whether the collector is running in async refresh mode
func (c *Collector) IsAsyncMode() bool {
	return c.config.RefreshInterval > 0
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

// maxConcurrentFetches limits the number of simultaneous SonarQube API calls
// to avoid overwhelming the server when there are many projects/branches/PRs.
const maxConcurrentFetches = 20

// refreshMetrics fetches all data from SonarQube and updates the cache.
// Called by the background goroutine in async mode, or directly in Collect in sync mode.
func (c *Collector) refreshMetrics() {
	startTime := time.Now()
	mode := "async"
	if c.config.RefreshInterval == 0 {
		mode = "sync"
	}
	c.mu.Lock()
	c.lastRefreshTime = startTime
	c.mu.Unlock()

	// Create a new cache to avoid partial updates
	newCache := &MetricsCache{
		ProjectMeasures: make(map[string][]sonarqube.Measure),
		Branches:        make(map[string][]sonarqube.Branch),
		BranchMeasures:  make(map[string]map[string][]sonarqube.Measure),
		PullRequests:    make(map[string][]sonarqube.PullRequest),
		PRMeasures:      make(map[string]map[string][]sonarqube.Measure),
		LastUpdated:     startTime,
	}

	// Use a mutex to protect concurrent writes to newCache
	var newCacheMu sync.Mutex

	// Fetch metrics
	metrics, err := c.client.GetMetrics()
	if err != nil {
		c.mu.Lock()
		c.lastRefreshError = err
		c.lastRefreshDuration = time.Since(startTime)
		c.mu.Unlock()
		log.Printf("[%s] Error refreshing metrics: %v", mode, err)
		return
	}
	newCache.Metrics = metrics
	newCache.NumericMetricKeys = c.getNumericMetricKeys(metrics)
	log.Printf("[%s] Found %d numeric metrics", mode, len(newCache.NumericMetricKeys))

	// Fetch projects
	projects, err := c.client.GetProjects()
	if err != nil {
		c.mu.Lock()
		c.lastRefreshError = err
		c.lastRefreshDuration = time.Since(startTime)
		c.mu.Unlock()
		log.Printf("[%s] Error refreshing projects: %v", mode, err)
		return
	}
	newCache.Projects = projects
	log.Printf("[%s] Found %d projects — starting phase 1 (measures%s%s)",
		mode,
		len(projects),
		map[bool]string{true: ", branches", false: ""}[c.config.CollectBranches],
		map[bool]string{true: ", pull requests", false: ""}[c.config.CollectPullRequests],
	)

	// sem limits concurrent SonarQube API calls across all goroutines
	sem := make(chan struct{}, maxConcurrentFetches)

	// Pre-collect all fetch tasks so we can call wg.Add before launching goroutines.
	// This avoids the data race where wg.Add is called from within a running goroutine
	// after wg.Wait() might have already returned.
	type branchTask struct {
		projectKey string
		branch     sonarqube.Branch
	}
	type prTask struct {
		projectKey string
		pr         sonarqube.PullRequest
	}

	var (
		wg           sync.WaitGroup
		branchTasks  []branchTask
		prTasks      []prTask
		branchTaskMu sync.Mutex
		prTaskMu     sync.Mutex
		doneProjects atomic.Int32
	)
	total := int32(len(projects))

	// Phase 1: fetch project measures + branches/PRs lists concurrently
	for _, project := range projects {
		wg.Add(1)
		go func(p sonarqube.Component) {
			defer wg.Done()
			sem <- struct{}{}
			measures, err := c.client.GetProjectMeasures(p.Key, newCache.NumericMetricKeys)
			<-sem
			n := doneProjects.Add(1)
			if err != nil {
				log.Printf("[%s] [%d/%d] Error fetching measures for project %s: %v", mode, n, total, p.Key, err)
			} else {
				log.Printf("[%s] [%d/%d] Fetched measures for project %s", mode, n, total, p.Key)
				newCacheMu.Lock()
				newCache.ProjectMeasures[p.Key] = measures
				newCacheMu.Unlock()
			}
		}(project)

		if c.config.CollectBranches {
			wg.Add(1)
			go func(p sonarqube.Component) {
				defer wg.Done()
				sem <- struct{}{}
				branches, err := c.client.GetProjectBranches(p.Key)
				<-sem
				if err != nil {
					log.Printf("[%s] Error fetching branches for project %s: %v", mode, p.Key, err)
					return
				}
				newCacheMu.Lock()
				newCache.Branches[p.Key] = branches
				newCache.BranchMeasures[p.Key] = make(map[string][]sonarqube.Measure)
				newCacheMu.Unlock()

				branchTaskMu.Lock()
				for _, b := range branches {
					branchTasks = append(branchTasks, branchTask{projectKey: p.Key, branch: b})
				}
				branchTaskMu.Unlock()
			}(project)
		}

		if c.config.CollectPullRequests {
			wg.Add(1)
			go func(p sonarqube.Component) {
				defer wg.Done()
				sem <- struct{}{}
				prs, err := c.client.GetProjectPullRequests(p.Key)
				<-sem
				if err != nil {
					log.Printf("[%s] Error fetching pull requests for project %s: %v", mode, p.Key, err)
					return
				}
				newCacheMu.Lock()
				newCache.PullRequests[p.Key] = prs
				newCache.PRMeasures[p.Key] = make(map[string][]sonarqube.Measure)
				newCacheMu.Unlock()

				prTaskMu.Lock()
				for _, pr := range prs {
					prTasks = append(prTasks, prTask{projectKey: p.Key, pr: pr})
				}
				prTaskMu.Unlock()
			}(project)
		}
	}
	wg.Wait()

	// Phase 2: fetch branch and PR measures now that all lists are known
	if len(branchTasks) > 0 || len(prTasks) > 0 {
		log.Printf("[%s] Phase 1 done — starting phase 2 (%d branches, %d pull requests)", mode, len(branchTasks), len(prTasks))
	}

	var (
		doneBranches  atomic.Int32
		donePRs       atomic.Int32
		totalBranches = int32(len(branchTasks))
		totalPRs      = int32(len(prTasks))
	)

	for _, task := range branchTasks {
		wg.Add(1)
		go func(t branchTask) {
			defer wg.Done()
			sem <- struct{}{}
			measures, err := c.client.GetBranchMeasures(t.projectKey, t.branch.Name, newCache.NumericMetricKeys)
			<-sem
			n := doneBranches.Add(1)
			if err != nil {
				log.Printf("[%s] [branch %d/%d] Error fetching measures for %s:%s: %v", mode, n, totalBranches, t.projectKey, t.branch.Name, err)
				return
			}
			log.Printf("[%s] [branch %d/%d] Fetched measures for %s:%s", mode, n, totalBranches, t.projectKey, t.branch.Name)
			newCacheMu.Lock()
			if _, exists := newCache.BranchMeasures[t.projectKey]; !exists {
				newCache.BranchMeasures[t.projectKey] = make(map[string][]sonarqube.Measure)
			}
			newCache.BranchMeasures[t.projectKey][t.branch.Name] = measures
			newCacheMu.Unlock()
		}(task)
	}
	for _, task := range prTasks {
		wg.Add(1)
		go func(t prTask) {
			defer wg.Done()
			sem <- struct{}{}
			measures, err := c.client.GetPullRequestMeasures(t.projectKey, t.pr.Key, newCache.NumericMetricKeys)
			<-sem
			n := donePRs.Add(1)
			if err != nil {
				log.Printf("[%s] [PR %d/%d] Error fetching measures for %s:%s: %v", mode, n, totalPRs, t.projectKey, t.pr.Key, err)
				return
			}
			log.Printf("[%s] [PR %d/%d] Fetched measures for %s:%s", mode, n, totalPRs, t.projectKey, t.pr.Key)
			newCacheMu.Lock()
			if _, exists := newCache.PRMeasures[t.projectKey]; !exists {
				newCache.PRMeasures[t.projectKey] = make(map[string][]sonarqube.Measure)
			}
			newCache.PRMeasures[t.projectKey][t.pr.Key] = measures
			newCacheMu.Unlock()
		}(task)
	}

	// Wait for all fetches to complete
	wg.Wait()

	// Swap cache and reset metric descriptors atomically under a single lock.
	// metricDescs must be cleared so descriptors are rebuilt from the new metric
	// definitions — stale descriptors cause Prometheus to reject metrics whose
	// help text changed between refreshes.
	c.mu.Lock()
	c.cache = newCache
	c.metricDescs = make(map[string]*prometheus.Desc)
	c.lastRefreshError = nil
	c.lastRefreshDuration = time.Since(startTime)
	c.lastRefreshTime = startTime
	c.mu.Unlock()

	// Close cacheReady channel on first successful population
	select {
	case <-c.cacheReady:
		// Already closed, first population already done
	default:
		close(c.cacheReady)
	}

	log.Printf("[%s] Metrics cache refreshed successfully in %v", mode, c.lastRefreshDuration)
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

// Collect is called by the Prometheus registry when collecting metrics.
// In async mode the cache is pre-populated by the background goroutine, so we
// just read from it. In sync mode we refresh the cache on-demand before reading,
// which means the same parallel fetch logic is used in both cases.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	if c.config.RefreshInterval == 0 {
		c.refreshMetrics()
	}

	c.collectFromCache(ch)
}

// collectFromCache collects metrics from the cache.
// mu protects both cache and metricDescs.
func (c *Collector) collectFromCache(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	defer c.mu.Unlock()

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
					branch.Name,
					branch.Name,
					isMain,
				)

				// Export branch measures
				if branchMeasures, exists := c.cache.BranchMeasures[projectKey]; exists {
					if measures, exists := branchMeasures[branch.Name]; exists {
						for _, measure := range measures {
							c.exportMeasure(ch, projectKey, projectName, measure, c.cache.Metrics, "branch", branch.Name)
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
				// Note: pr.Title is omitted from labels to avoid high cardinality
				ch <- prometheus.MustNewConstMetric(
					c.pullRequestInfo,
					prometheus.GaugeValue,
					1,
					projectKey,
					projectName,
					pr.Key,
					pr.Status.QualityGateStatus,
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
	key := metric.Key

	// For backward compatibility, don't include entityType in the key for project metrics
	// This ensures existing metrics keep the same descriptor
	if entityType != "project" {
		key = metric.Key + "_" + entityType
	}

	if desc, exists := c.metricDescs[key]; exists {
		return desc
	}

	// Build the Prometheus metric name.
	// For branch/PR metrics we use a suffix (_branch, _pull_request) rather than a prefix
	// to avoid collisions with SonarQube metric keys that already start with "branch_"
	// (e.g. SonarQube "branch_coverage" would collide with "coverage" measured on a branch
	// if both were prefixed with "sonarqube_branch_").
	metricName := "sonarqube_" + sanitizeMetricName(metric.Key)
	if entityType != "project" {
		metricName = "sonarqube_" + sanitizeMetricName(metric.Key) + "_" + entityType
	}

	// For backward compatibility, don't add type label to existing project metrics
	var constantLabels prometheus.Labels
	if entityType == "project" {
		constantLabels = prometheus.Labels{"domain": metric.Domain}
	} else {
		constantLabels = prometheus.Labels{"domain": metric.Domain, "type": entityType}
	}

	desc := prometheus.NewDesc(
		metricName,
		metric.Description,
		c.getLabelNames(entityType),
		constantLabels,
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
