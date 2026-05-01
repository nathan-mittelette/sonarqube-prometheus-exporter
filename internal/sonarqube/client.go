package sonarqube

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client represents a SonarQube API client
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a new SonarQube client
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetMetrics retrieves all available metrics from SonarQube
func (c *Client) GetMetrics() ([]Metric, error) {
	url := fmt.Sprintf("%s/api/metrics/search?ps=500", c.baseURL)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var metricsResp MetricsResponse
	if err := json.NewDecoder(resp.Body).Decode(&metricsResp); err != nil {
		return nil, fmt.Errorf("failed to decode metrics response: %w", err)
	}

	return metricsResp.Metrics, nil
}

// GetProjects retrieves all projects from SonarQube
func (c *Client) GetProjects() ([]Component, error) {
	var allComponents []Component
	pageIndex := 1
	pageSize := 500

	for {
		url := fmt.Sprintf("%s/api/components/search_projects?ps=%d&p=%d", c.baseURL, pageSize, pageIndex)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch projects: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
		}

		var componentsResp ComponentsResponse
		if err := json.NewDecoder(resp.Body).Decode(&componentsResp); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to decode components response: %w", err)
		}

		allComponents = append(allComponents, componentsResp.Components...)

		// Close response body after processing
		resp.Body.Close()

		// Check if we've retrieved all projects
		if len(allComponents) >= componentsResp.Paging.Total {
			break
		}

		pageIndex++
	}

	return allComponents, nil
}

// GetProjectMeasures retrieves measures for a specific project
func (c *Client) GetProjectMeasures(projectKey string, metricKeys []string) ([]Measure, error) {
	if len(metricKeys) == 0 {
		return []Measure{}, nil
	}

	// Build metric keys parameter
	var metricsParam strings.Builder
	for i, key := range metricKeys {
		if i > 0 {
			metricsParam.WriteString(",")
		}
		metricsParam.WriteString(key)
	}

	reqURL := fmt.Sprintf("%s/api/measures/component?component=%s&metricKeys=%s", c.baseURL, url.QueryEscape(projectKey), metricsParam.String())

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch project measures: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var measuresResp MeasuresResponse
	if err := json.NewDecoder(resp.Body).Decode(&measuresResp); err != nil {
		return nil, fmt.Errorf("failed to decode measures response: %w", err)
	}

	return measuresResp.Component.Measures, nil
}

// GetProjectBranches retrieves all branches for a specific project.
// The SonarQube /api/project_branches/list endpoint returns all branches in a single
// response without pagination support, so no loop is needed.
func (c *Client) GetProjectBranches(projectKey string) ([]Branch, error) {
	reqURL := fmt.Sprintf("%s/api/project_branches/list?project=%s", c.baseURL, url.QueryEscape(projectKey))

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch branches: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusNotFound {
			return []Branch{}, nil
		}
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var branchesResp BranchesResponse
	if err := json.NewDecoder(resp.Body).Decode(&branchesResp); err != nil {
		return nil, fmt.Errorf("failed to decode branches response: %w", err)
	}

	return branchesResp.Branches, nil
}

// GetProjectPullRequests retrieves all pull requests for a specific project.
// The SonarQube /api/project_pull_requests/list endpoint returns all PRs in a single
// response without pagination support, so no loop is needed.
func (c *Client) GetProjectPullRequests(projectKey string) ([]PullRequest, error) {
	reqURL := fmt.Sprintf("%s/api/project_pull_requests/list?project=%s", c.baseURL, url.QueryEscape(projectKey))

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pull requests: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusNotFound {
			return []PullRequest{}, nil
		}
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var prsResp PullRequestsResponse
	if err := json.NewDecoder(resp.Body).Decode(&prsResp); err != nil {
		return nil, fmt.Errorf("failed to decode pull requests response: %w", err)
	}

	return prsResp.PullRequests, nil
}

// GetBranchMeasures retrieves measures for a specific branch
func (c *Client) GetBranchMeasures(projectKey, branchName string, metricKeys []string) ([]Measure, error) {
	if len(metricKeys) == 0 {
		return []Measure{}, nil
	}

	// Build metric keys parameter
	var metricsParam strings.Builder
	for i, key := range metricKeys {
		if i > 0 {
			metricsParam.WriteString(",")
		}
		metricsParam.WriteString(key)
	}

	// Use branch query parameter instead of concatenating to component key
	reqURL := fmt.Sprintf("%s/api/measures/component?component=%s&metricKeys=%s&branch=%s", c.baseURL, url.QueryEscape(projectKey), metricsParam.String(), url.QueryEscape(branchName))

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch branch measures: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var measuresResp MeasuresResponse
	if err := json.NewDecoder(resp.Body).Decode(&measuresResp); err != nil {
		return nil, fmt.Errorf("failed to decode measures response: %w", err)
	}

	return measuresResp.Component.Measures, nil
}

// GetPullRequestMeasures retrieves measures for a specific pull request
func (c *Client) GetPullRequestMeasures(projectKey, prKey string, metricKeys []string) ([]Measure, error) {
	if len(metricKeys) == 0 {
		return []Measure{}, nil
	}

	// Build metric keys parameter
	var metricsParam strings.Builder
	for i, key := range metricKeys {
		if i > 0 {
			metricsParam.WriteString(",")
		}
		metricsParam.WriteString(key)
	}

	// Use pullRequest query parameter instead of concatenating to component key
	reqURL := fmt.Sprintf("%s/api/measures/component?component=%s&metricKeys=%s&pullRequest=%s", c.baseURL, url.QueryEscape(projectKey), metricsParam.String(), url.QueryEscape(prKey))

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pull request measures: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var measuresResp MeasuresResponse
	if err := json.NewDecoder(resp.Body).Decode(&measuresResp); err != nil {
		return nil, fmt.Errorf("failed to decode measures response: %w", err)
	}

	return measuresResp.Component.Measures, nil
}
