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

**Prometheus Integration**: The `metrics.Collector` implements the Prometheus `Collector` interface. When Prometheus scrapes `/metrics`, it calls `Collect()` which:
1. Fetches all available metrics from SonarQube API
2. Filters for numeric metric types (INT, FLOAT, PERCENT, RATING, MILLISEC, WORK_DUR)
3. Fetches all projects with pagination support
4. For each project, fetches all numeric measures
5. Exports each measure as a Prometheus gauge with labels

**Dynamic Metric Discovery**: Rather than hardcoding metrics, the collector dynamically discovers all available metrics from SonarQube's `/api/metrics/search` endpoint and filters for numeric types. This makes it compatible with different SonarQube versions and plugins.

**Metric Naming**: SonarQube metric keys are sanitized for Prometheus:
- Prefix with `sonarqube_`
- Replace `-` and `.` with `_`
- Convert to lowercase
- Example: `new_violations` → `sonarqube_new_violations`

**Labels**: All metrics include:
- `project_key` - SonarQube project key
- `project_name` - Human-readable project name
- `domain` - Metric domain (e.g., "Reliability", "Security", "Maintainability")

### SonarQube API Endpoints Used
- `/api/metrics/search` - Get all available metric definitions (ps=500)
- `/api/components/search_projects` - Get all projects (paginated, ps=500)
- `/api/measures/component` - Get measures for a specific project and metric keys

### Configuration
Config supports both CLI flags and environment variables (env vars take precedence as defaults):
- `-host` / `EXPORTER_HOST` (default: 0.0.0.0)
- `-port` / `EXPORTER_PORT` (default: 9090)
- `-sonarqube-url` / `SONARQUBE_URL` (required)
- `-sonarqube-token` / `SONARQUBE_TOKEN` (required)

### Testing Strategy
- Unit tests mock the SonarQube client
- Integration tests (`collector_integration_test.go`) require actual SonarQube instance
- Use `LoadWithFlagSet()` in config package for testable flag parsing
- HTTP client has 30s timeout

## Important Implementation Details

- The collector uses `sync.RWMutex` to safely cache metric descriptors across scrapes
- Project pagination handles large SonarQube instances (500 projects per page)
- Empty metric values are returned as 0.0
- Hidden metrics are filtered out
- Server implements graceful shutdown with 30s timeout
- All HTTP responses include proper error handling with status codes

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