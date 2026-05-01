# AGENTS.md

This file provides guidance to AI coding agents (Claude, Copilot, Vibe, etc.) when working with this SonarQube Prometheus Exporter codebase.

## Project Overview

A **Go** application that exports **SonarQube metrics** in **Prometheus format**. It bridges SonarQube's REST API with Prometheus by exposing a `/metrics` HTTP endpoint that scrapers can consume.

**Core value**: Dynamic metric discovery from SonarQube, no hardcoded metrics — compatible across versions and plugins.

## Quick Start

### Prerequisites
- Go 1.26.2+
- SonarQube instance (v7.9+ for full API support)
- SonarQube user token with "Browse" permissions

### Development Commands

```bash
# Build
make build                    # Binary to bin/sonarqube-exporter
make build-all                # Cross-compile: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64

# Test
make test                     # All tests with race detector
make coverage                 # HTML coverage report (coverage.html)

# Run single test
go test -v ./internal/config -run TestLoad
go test -v ./internal/metrics -run TestCollect_WithBranches

# Quality
make fmt                      # gofmt
make vet                      # go vet
make lint                    # golangci-lint (if installed)
make all                      # clean, fmt, vet, test, build

# Run locally
make run                      # Build and run
# Or directly (requires env vars)
SONARQUBE_URL=https://sonar.example.com \
  SONARQUBE_TOKEN=your-token \
  ./bin/sonarqube-exporter -host 0.0.0.0 -port 9090
```

### Environment Variables & Flags

| Config | Flag | Env Var | Default | Description |
|--------|------|---------|---------|-------------|
| Host | `-host` | `EXPORTER_HOST` | `0.0.0.0` | Server bind address |
| Port | `-port` | `EXPORTER_PORT` | `9090` | Server port |
| SonarQube URL | `-sonarqube-url` | `SONARQUBE_URL` | *required* | Base URL of SonarQube |
| SonarQube Token | `-sonarqube-token` | `SONARQUBE_TOKEN` | *required* | API token |
| Refresh Interval | `-refresh-interval` | `REFRESH_INTERVAL` | `0` | Seconds between cache refreshes (0 = sync mode) |
| Collect Branches | `-collect-branches` | `COLLECT_BRANCHES` | `false` | Enable branch metrics |
| Collect PRs | `-collect-pull-requests` | `COLLECT_PULL_REQUESTS` | `false` | Enable pull request metrics |

## Architecture

### Component Flow

```
┌─────────────────────────────────────────────────────────┐
│                    cmd/exporter/main.go                    │
│  Load config → Create SonarQube client → Create collector │
│  Start HTTP server → Handle graceful shutdown            │
└─────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────┐
│                 internal/config/config.go                 │
│  CLI flags with env var defaults, validation            │
│  Load() and LoadWithFlagSet() for testability            │
└─────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────┐
│              internal/sonarqube/client.go                  │
│  Thin HTTP wrapper around SonarQube REST API             │
│  - 30s HTTP timeout                                         │
│  - Pagination for /api/components/search_projects (ps=500)│
│  - 404 on branch/PR endpoints → empty slice (no error)    │
│  - Endpoints: metrics/search, components/search_projects │
│               measures/component, project_branches/list   │
│               project_pull_requests/list                  │
└─────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────┐
│               internal/metrics/collector.go               │
│  Prometheus Collector implementation                       │
│  - Discovers metrics dynamically from SonarQube          │
│  - Filters for numeric types: INT, FLOAT, PERCENT,       │
│    RATING, MILLISEC, WORK_DUR                              │
│  - Excludes hidden metrics                                 │
│  - Two modes: sync (on-demand) and async (cached)          │
│  - Concurrent fetches with semaphore (max 20)            │
│  - Atomic cache swap with RWMutex                         │
└─────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────┐
│                 internal/server/server.go                  │
│  HTTP server with custom Prometheus registry             │
│  - /metrics: Prometheus metrics                           │
│  - /health: 200 OK (503 in async mode until cache ready)  │
│  - /: Basic info                                          │
│  - No WriteTimeout in sync mode (long scrapes)           │
│  - ReadTimeout: 15s, IdleTimeout: 60s                     │
└─────────────────────────────────────────────────────────┘
```

### Sync vs Async Collection Modes

| Aspect | Sync Mode (`refresh-interval=0`) | Async Mode (`refresh-interval>0`) |
|--------|----------------------------------|-----------------------------------|
| Data freshness | Always fresh (on-demand) | Cached, refreshed at interval |
| Scrape latency | High (blocks until fetch complete) | Low (reads from cache) |
| Server load | High per scrape | Spread over time |
| `/health` | Always 200 | 503 until first refresh complete |
| WriteTimeout | Disabled (0) | Can be set |
| Use case | Small instances, few projects | Large instances, many projects |

**Async refresh phases:**
1. Phase 1: Concurrent fetch of metric definitions, projects, branch lists, PR lists (max 20 concurrent calls)
2. Phase 2: Concurrent fetch of measures for each branch and PR
3. Atomic swap: Old cache replaced under write lock

## Key Design Decisions

### Dynamic Metric Discovery
- NO hardcoded metric lists
- Fetches all metrics from `/api/metrics/search?ps=500`
- Filters to numeric, non-hidden metrics
- Compatible with any SonarQube version and plugin

### Metric Naming Convention
- **Prefix**: `sonarqube_`
- **Sanitization**: `-` → `_`, `.` → `_`, lowercase
- **Branch metrics**: `sonarqube_<metric>_branch`
- **PR metrics**: `sonarqube_<metric>_pull_request`
- **Example**: `new_violations` → `sonarqube_new_violations`
- **Rationale for suffix**: Avoids collision with SonarQube metrics like `branch_coverage`

### Label Strategy

| Entity | Metric Pattern | Labels | Constant Labels |
|--------|---------------|--------|-----------------|
| Project | `sonarqube_<metric>` | `project_key`, `project_name` | `domain` |
| Branch | `sonarqube_<metric>_branch` | `project_key`, `project_name`, `branch_key` | `domain`, `type="branch"` |
| PR | `sonarqube_<metric>_pull_request` | `project_key`, `project_name`, `pr_key` | `domain`, `type="pull_request"` |

**Info metrics (cardinality control):**
- `sonarqube_project_info`: `project_key`, `project_name`, `qualifier`, `visibility`
- `sonarqube_branch_info`: `project_key`, `project_name`, `branch_key`, `branch_name`, `is_main`
- `sonarqube_pull_request_info`: `project_key`, `project_name`, `pr_key`, `pr_status`, `pr_branch`, `pr_target`

**Note**: PR `title` is intentionally excluded to avoid high cardinality.

### Empty Values
- Empty SonarQube measure values → exported as `0.0`
- Only numeric metric types are exported (INT, FLOAT, PERCENT, RATING, MILLISEC, WORK_DUR)

### Concurrency Control
- Max 20 concurrent SonarQube API calls (`maxConcurrentFetches`)
- Semaphore pattern with buffered channel
- Prevents overwhelming SonarQube server

### Backward Compatibility
- Project metrics: NO `type` constant label (keeps original shape)
- Branch/PR metrics: Include `type` constant label
- Metric descriptors rebuilt on each cache refresh to handle definition changes
- Cache and descriptors swapped together under single lock to avoid stale help text

## SonarQube API Endpoints

| Endpoint | Purpose | Pagination | Notes |
|----------|---------|------------|-------|
| `GET /api/metrics/search?ps=500` | All metric definitions | No | Filters for numeric, non-hidden |
| `GET /api/components/search_projects?ps=500&p={page}` | All projects | Yes (ps=500) | Handles large instances |
| `GET /api/measures/component?component={key}&metricKeys={keys}` | Project measures | No | Fetches all numeric metrics at once |
| `GET /api/project_branches/list?project={key}` | Project branches | No | Returns all in one response |
| `GET /api/project_pull_requests/list?project={key}` | Project PRs | No | Returns all in one response |
| `GET /api/measures/component?component={key}&metricKeys={keys}&branch={name}` | Branch measures | No | Uses `branch` query param |
| `GET /api/measures/component?component={key}&metricKeys={keys}&pullRequest={key}` | PR measures | No | Uses `pullRequest` query param |

## Testing Strategy

### Unit Tests
- Mock the SonarQube client
- Test individual functions in isolation
- Use `LoadWithFlagSet()` for config parsing tests

### Integration Tests
- Use `httptest.NewServer` with mocked SonarQube JSON responses
- No real SonarQube instance required
- Located in `*_integration_test.go` files
- Key tests: `TestCollect_WithBranches`, `TestCollect_WithPullRequests`, `TestCollect_AsyncMode`

### Test Commands
```bash
# All tests with race detector and coverage
go test -v -race -coverprofile=coverage.out ./...

# Generate HTML coverage report
go tool cover -html=coverage.out -o coverage.html

# Single package
make test  # Uses: go test -v -race ./...
```

### HTTP Client Timeout
- 30 seconds for all SonarQube API calls
- Configurable via `http.Client.Timeout` in client creation

## Conventions & Best Practices

### Code Style
- Standard Go formatting (`gofmt`)
- Use `go vet` for static analysis
- `golangci-lint` for additional checks (optional)
- No external linter configs in repo

### Error Handling
- Wrapped errors with `fmt.Errorf("...: %w", err)`
- HTTP errors include status code and response body
- 404 on branch/PR endpoints → return empty slice (no error)
- Log errors but don't fail entire collection for single project errors

### Logging
- Use `log` package (standard library)
- Prefix log messages with mode context: `[sync]` or `[async]`
- Include progress indicators: `[1/10]`, `[branch 5/20]`
- Duration logging for refresh operations

### Concurrency
- Use `sync.RWMutex` for shared state (cache + descriptors)
- Use `sync.WaitGroup` for goroutine coordination
- Use buffered channels for semaphores (concurrency limiting)
- Use `atomic` package for counters in concurrent loops
- Avoid `wg.Add()` inside running goroutines (data race risk)

### Testing
- Prefer `httptest.NewServer` over real API instances
- Use `LoadWithFlagSet()` for config tests with fresh `flag.FlagSet`
- Mock responses with proper JSON structures
- Test both sync and async modes

## Docker

### Build
```bash
# Single platform
docker build -t sonarqube-exporter .

# Multi-platform (uses buildx)
docker buildx build --platform linux/amd64,linux/arm64 -t sonarqube-exporter .
```

### Run
```bash
docker run -p 9090:9090 \
  -e SONARQUBE_URL=https://sonar.example.com \
  -e SONARQUBE_TOKEN=your-token \
  sonarqube-exporter
```

### Dockerfile
- Multi-stage build with `golang:1.26.2` builder
- `CGO_ENABLED=0` for static binary
- `distroless/static:nonroot` as final image
- Runs as non-root user

## CI/CD

### GitHub Actions Workflows

**ci.yml**: Runs on push/PR to main
- Setup Go 1.26.2
- Cache Go modules
- Format check (`gofmt -s -l .`)
- `go vet ./...`
- Tests with race detector and coverage
- Build verification
- Docker image build (multi-platform, verification only)

**docker-release.yml**: Build and push Docker images
- Triggered on release publish
- Builds and pushes to GitHub Container Registry
- Multi-platform support (linux/amd64, linux/arm64)

## Important Implementation Details

### Cache Management
- `MetricsCache` struct holds all cached data
- Cache is swapped atomically under `RWMutex` lock
- Metric descriptors (`metricDescs`) cleared and rebuilt on each refresh
- First cache population closes `cacheReady` channel
- `/health` returns 503 in async mode until `cacheReady` is closed

### Graceful Shutdown
- 30-second timeout for server shutdown
- Async refresh goroutine stopped before server shutdown
- Context cancellation for refresh loop
- WaitGroup ensures refresh goroutine completes

### Server Configuration
- `ReadTimeout`: 15 seconds (protects against slow clients)
- `IdleTimeout`: 60 seconds (closes idle connections)
- `WriteTimeout`: 0 (disabled) in sync mode — allows long-running scrapes
- Prometheus registry is custom (not default) to avoid metric collisions

### Project Pagination
- Handles SonarQube instances with many projects
- 500 projects per page (`ps=500`)
- Continues until `paging.total` projects retrieved

### Branch & PR Collection
- Disabled by default (opt-in via flags)
- Branch/PR lists fetched in Phase 1
- Branch/PR measures fetched in Phase 2
- No pagination needed (endpoints return all at once)
- 404 responses treated as "no data available"

### Metric Types Handling

| SonarQube Type | Prometheus Value | Notes |
|---------------|------------------|-------|
| INT | float64(int) | Standard integer |
| FLOAT | float64 | Standard float |
| PERCENT | float64 | Percentage value |
| RATING | float64 | A=1.0, B=2.0, C=3.0, D=4.0, E=5.0 |
| MILLISEC | float64(int) | Milliseconds |
| WORK_DUR | float64(int) | Minutes |
| Other | Ignored | Non-numeric types excluded |

## Common Tasks

### Add a new metric type
1. Add to `numericTypes` map in `collector.go`
2. Add parsing case in `parseMetricValue()`

### Add a new API endpoint
1. Add method to `internal/sonarqube/client.go`
2. Add corresponding model structs in `internal/sonarqube/models.go`
3. Update collector to use new method

### Modify metric labels
1. Update `getLabelNames()` in `collector.go`
2. Update `exportMeasure()` label construction
3. Update descriptor constant labels in `getOrCreateMetricDesc()`

### Test changes
```bash
# Run all tests
make test

# Run with coverage
make coverage

# Test specific functionality
go test -v ./internal/metrics -run TestCollect_AsyncMode
```

## Troubleshooting

### "Metrics not appearing"
- Check if metric is numeric and not hidden in SonarQube
- Verify metric discovery: `make run` and check logs for "Found X numeric metrics"
- Check metric sanitization: `-` and `.` are replaced with `_`

### "High scrape latency"
- Switch to async mode with `-refresh-interval 60`
- Reduce number of projects/branches/PRs being collected
- Check SonarQube server performance

### "503 on /health in async mode"
- Normal until first cache refresh completes
- Check logs for refresh errors
- Increase refresh interval if SonarQube is slow

### "404 on branch/PR endpoints"
- Feature may not be available in your SonarQube version
- Requires Developer Edition for branch support
- Requires Developer Edition+ for PR support
- Collector handles this gracefully (returns empty slice)

## File Structure

```
sonarqube-prometheus-exporter/
├── AGENTS.md              # This file
├── CLAUDE.md              # Claude-specific instructions
├── .github/
│   ├── copilot-instructions.md  # Copilot-specific instructions
│   └── workflows/
│       ├── ci.yml         # CI pipeline
│       └── docker-release.yml
├── Makefile               # Build, test, lint commands
├── Dockerfile             # Multi-stage Docker build
├── go.mod                 # Go module definition
├── go.sum                 # Dependency checksums
├── cmd/
│   └── exporter/
│       └── main.go        # Application entry point
└── internal/
    ├── config/
    │   ├── config.go      # CLI flags & env var loading
    │   └── config_test.go # Config tests
    ├── metrics/
    │   ├── collector.go                 # Prometheus collector
    │   ├── collector_test.go            # Unit tests
    │   └── collector_integration_test.go # Integration tests
    └── sonarqube/
        ├── client.go      # SonarQube API client
        ├── client_test.go # Client tests
        └── models.go      # API response models
    └── server/
        ├── server.go      # HTTP server
        ├── handlers.go    # Request handlers
        └── server_test.go # Server tests
```
