# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go application that exports SonarQube metrics in Prometheus format. It acts as a bridge between SonarQube's REST API and Prometheus, exposing metrics at `/metrics` endpoint.

## Development Commands

### Building
```bash
make build          # Build binary to bin/sonarqube-exporter
make build-all      # Cross-compile for Linux, macOS (amd64/arm64), Windows
```

### Testing
```bash
make test           # Run all tests with race detector and coverage
make coverage       # Generate HTML coverage report (coverage.html)

# Run specific test
go test -v ./internal/config -run TestLoad

# Run integration tests only
go test -v ./internal/metrics -run Integration
```

### Code Quality
```bash
make fmt            # Format code with gofmt
make vet            # Run go vet
make lint           # Run golangci-lint (requires installation)
make all            # Run clean, fmt, vet, test, build in sequence
```

### Running Locally
```bash
# Set required environment variables
export SONARQUBE_URL=https://sonar.example.com
export SONARQUBE_TOKEN=your-token-here

make run            # Build and run
# Or run binary directly
./bin/sonarqube-exporter -host 0.0.0.0 -port 9090
```

## Architecture

### Component Flow
1. **main.go** - Application entry point, sets up graceful shutdown
2. **config** - Loads configuration from CLI flags or environment variables
3. **sonarqube.Client** - HTTP client for SonarQube REST API
4. **metrics.Collector** - Implements prometheus.Collector interface
5. **server** - HTTP server exposing /metrics, /health, / endpoints

### Key Design Patterns

**Prometheus Integration**: The `metrics.Collector` implements the Prometheus `Collector` interface. In sync mode, each `/metrics` scrape calls `Collect()` which fetches live data. In async mode, a background goroutine pre-populates a cache at the configured interval and scrapes read from it instantly.

**Sync vs Async collection flow:**
- **Sync (default)**: `Collect()` → fetch metrics/projects/measures on demand, blocks until done
- **Async (`-refresh-interval N`)**: background `refreshLoop` runs every N seconds → populates `MetricsCache` → `Collect()` reads from cache

**Async cache phases:**
1. Phase 1: fetch metric definitions, projects, branch lists, PR lists concurrently (max 20 concurrent API calls via semaphore)
2. Phase 2: fetch measures per branch and per PR concurrently
3. Atomic swap: replace old cache under write lock

**Dynamic Metric Discovery**: Rather than hardcoding metrics, the collector dynamically discovers all available metrics from SonarQube's `/api/metrics/search` endpoint and filters for numeric types (INT, FLOAT, PERCENT, RATING, MILLISEC, WORK_DUR). Hidden metrics are excluded. This makes it compatible with different SonarQube versions and plugins.

**Metric Naming**: SonarQube metric keys are sanitized for Prometheus:
- Prefix with `sonarqube_`
- Replace `-` and `.` with `_`
- Convert to lowercase
- Branch metrics append `_branch`, PR metrics append `_pull_request`
- Example: `new_violations` → `sonarqube_new_violations`, `sonarqube_new_violations_branch`

**Labels**:
- Project metrics: `project_key`, `project_name`, `domain`
- Branch metrics (`sonarqube_<metric>_branch`): `project_key`, `project_name`, `branch_key`, `domain`, `type="branch"`
- PR metrics (`sonarqube_<metric>_pull_request`): `project_key`, `project_name`, `pr_key`, `domain`, `type="pull_request"`
- `sonarqube_branch_info`: adds `branch_name`, `is_main` ("true"/"false")
- `sonarqube_pull_request_info`: adds `pr_branch`, `pr_target`, `pr_status` (quality gate: OK/ERROR)

### SonarQube API Endpoints Used
- `/api/metrics/search` - Get all available metric definitions (ps=500)
- `/api/components/search_projects` - Get all projects (paginated, ps=500)
- `/api/measures/component` - Get measures for a project (adds `branch=` or `pullRequest=` param for branch/PR)
- `/api/project_branches/list` - List branches for a project (requires `-collect-branches`)
- `/api/project_pull_requests/list` - List pull requests for a project (requires `-collect-pull-requests`)

### Configuration
Config supports both CLI flags and environment variables (env vars take precedence as defaults):
- `-host` / `EXPORTER_HOST` (default: 0.0.0.0)
- `-port` / `EXPORTER_PORT` (default: 9090)
- `-sonarqube-url` / `SONARQUBE_URL` (required)
- `-sonarqube-token` / `SONARQUBE_TOKEN` (required)
- `-refresh-interval` / `REFRESH_INTERVAL` (default: 0, disabled — sync mode)
- `-collect-branches` / `COLLECT_BRANCHES` (default: false)
- `-collect-pull-requests` / `COLLECT_PULL_REQUESTS` (default: false)

### Testing Strategy
- Unit tests mock the SonarQube client
- Integration tests (`collector_integration_test.go`) use an HTTP test server simulating SonarQube — no real instance needed
  - `TestCollect_WithBranches` — branch collection
  - `TestCollect_WithPullRequests` — PR collection
  - `TestCollect_AsyncMode` — async cache behavior
- Use `LoadWithFlagSet()` in config package for testable flag parsing
- HTTP client has 30s timeout

## Important Implementation Details

- The collector uses `sync.RWMutex` to safely cache metric descriptors and data across scrapes
- In async mode, `/health` returns 503 until the first cache refresh completes (`cacheReady` channel)
- In sync mode, server `WriteTimeout` is disabled to allow long-running scrapes on large instances
- Project pagination handles large SonarQube instances (500 projects per page)
- Branch and PR list endpoints have no pagination (all returned in one response)
- Empty metric values are returned as 0.0
- Server implements graceful shutdown with 30s timeout; async refresh goroutine is stopped before shutdown
- All HTTP responses include proper error handling with status codes
- Max concurrent SonarQube API calls: 20 (`maxConcurrentFetches` constant in collector.go)

## Docker

Build and run with Docker:
```bash
docker build -t sonarqube-exporter .
docker run -p 9090:9090 \
  -e SONARQUBE_URL=https://sonar.example.com \
  -e SONARQUBE_TOKEN=your-token \
  sonarqube-exporter
```

## CI/CD

GitHub Actions workflow (`.github/workflows/ci.yml`) runs on push/PR:
- Format check (gofmt)
- go vet
- Tests with race detector and coverage
- Build verification
- Docker image build (verification only, not pushed)
- Coverage upload to Codecov