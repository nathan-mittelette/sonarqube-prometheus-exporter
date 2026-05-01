# SonarQube Prometheus Exporter

A lightweight Go application that exports SonarQube metrics in Prometheus format.

## Features

- Fetches metrics from SonarQube API
- Exposes metrics in Prometheus format via `/metrics` endpoint
- Supports configuration via CLI flags or environment variables
- Includes health check endpoint
- Comprehensive test coverage
- Cross-platform support
- **Async mode**: pre-fetches metrics in background at a configurable interval
- **Branch metrics**: exports metrics per branch with `sonarqube_branch_info` and `sonarqube_<metric>_branch`
- **Pull request metrics**: exports metrics per PR with `sonarqube_pull_request_info` and `sonarqube_<metric>_pull_request`

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/axopen/sonarqube-prometheus-exporter.git
cd sonarqube-prometheus-exporter

# Install dependencies
make install

# Build the binary
make build
```

### Using Go Install

```bash
go install github.com/axopen/sonarqube-prometheus-exporter/cmd/exporter@latest
```

## Configuration

The exporter can be configured using either command-line flags or environment variables.

### Command-Line Flags

```bash
./bin/sonarqube-exporter \
  -host 0.0.0.0 \
  -port 9090 \
  -sonarqube-url https://sonar.example.com \
  -sonarqube-token your-token-here \
  -refresh-interval 60 \
  -collect-branches \
  -collect-pull-requests
```

### Environment Variables

```bash
export EXPORTER_HOST=0.0.0.0
export EXPORTER_PORT=9090
export SONARQUBE_URL=https://sonar.example.com
export SONARQUBE_TOKEN=your-token-here
export REFRESH_INTERVAL=60
export COLLECT_BRANCHES=true
export COLLECT_PULL_REQUESTS=true

./bin/sonarqube-exporter
```

### Configuration Options

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `-host` | `EXPORTER_HOST` | `0.0.0.0` | Host to bind the exporter server |
| `-port` | `EXPORTER_PORT` | `9090` | Port to bind the exporter server |
| `-sonarqube-url` | `SONARQUBE_URL` | *required* | SonarQube server URL |
| `-sonarqube-token` | `SONARQUBE_TOKEN` | *required* | SonarQube authentication token |
| `-refresh-interval` | `REFRESH_INTERVAL` | `0` | Async refresh interval in seconds (`0` = sync mode) |
| `-collect-branches` | `COLLECT_BRANCHES` | `false` | Enable branch metrics collection |
| `-collect-pull-requests` | `COLLECT_PULL_REQUESTS` | `false` | Enable pull request metrics collection |

## Sync vs Async Mode

### Sync mode (default, `refresh-interval=0`)

Metrics are fetched on demand each time Prometheus scrapes `/metrics`. Each scrape blocks until all data is fetched from SonarQube. This can take a significant amount of time on large instances with many projects, branches, or PRs.

### Async mode (`-refresh-interval 60`)

A background goroutine pre-fetches all metrics at the configured interval (in seconds) and stores them in an in-memory cache. Prometheus scrapes read directly from the cache without blocking.

```bash
./bin/sonarqube-exporter -refresh-interval 60
```

**Behavior:**
- First refresh happens immediately on startup
- The `/health` endpoint returns `503` until the cache is populated after the first refresh
- Subsequent scrapes are served instantly from cache
- Up to 20 concurrent API calls are made to SonarQube during each refresh
- Graceful shutdown waits for the current refresh to complete

Async mode is recommended for production use, especially when collecting branches and pull requests.

## Usage

### Starting the Exporter

```bash
# Using make
make run

# Or directly
./bin/sonarqube-exporter
```

### Accessing Metrics

Once the exporter is running, you can access:

- **Metrics**: `http://localhost:9090/metrics`
- **Health Check**: `http://localhost:9090/health`
- **Home Page**: `http://localhost:9090/`

### Example Metrics Output

```
# HELP sonarqube_project_info Information about SonarQube projects
# TYPE sonarqube_project_info gauge
sonarqube_project_info{project_key="my-project",project_name="My Project",qualifier="TRK",visibility="private"} 1

# HELP sonarqube_bugs Bugs
# TYPE sonarqube_bugs gauge
sonarqube_bugs{domain="Reliability",project_key="my-project",project_name="My Project"} 5

# HELP sonarqube_code_smells Code Smells
# TYPE sonarqube_code_smells gauge
sonarqube_code_smells{domain="Maintainability",project_key="my-project",project_name="My Project"} 23

# HELP sonarqube_branch_info Information about SonarQube branches
# TYPE sonarqube_branch_info gauge
sonarqube_branch_info{branch_key="main",branch_name="main",is_main="true",project_key="my-project",project_name="My Project"} 1
sonarqube_branch_info{branch_key="feature/my-feature",branch_name="feature/my-feature",is_main="false",project_key="my-project",project_name="My Project"} 1

# HELP sonarqube_bugs_branch Bugs (branch)
# TYPE sonarqube_bugs_branch gauge
sonarqube_bugs_branch{branch_key="main",domain="Reliability",project_key="my-project",project_name="My Project",type="branch"} 3

# HELP sonarqube_pull_request_info Information about SonarQube pull requests
# TYPE sonarqube_pull_request_info gauge
sonarqube_pull_request_info{pr_branch="feature/my-feature",pr_key="42",pr_status="OK",pr_target="main",project_key="my-project",project_name="My Project"} 1

# HELP sonarqube_bugs_pull_request Bugs (pull request)
# TYPE sonarqube_bugs_pull_request gauge
sonarqube_bugs_pull_request{domain="Reliability",pr_key="42",project_key="my-project",project_name="My Project",type="pull_request"} 1
```

## Development

### Prerequisites

- Go 1.20 or higher
- Make (optional, but recommended)

### Building

```bash
# Build for current platform
make build

# Build for all platforms
make build-all
```

### Testing

```bash
# Run tests
make test

# Run tests with coverage
make coverage
```

### Code Quality

```bash
# Format code
make fmt

# Run static analysis
make vet

# Run linter (requires golangci-lint)
make lint
```

## Project Structure

```
.
├── cmd/
│   └── exporter/          # Application entry point
│       └── main.go
├── internal/
│   ├── config/            # Configuration management
│   │   ├── config.go
│   │   └── config_test.go
│   ├── sonarqube/         # SonarQube API client
│   │   ├── client.go
│   │   └── client_test.go
│   ├── metrics/           # Prometheus metrics collector
│   │   ├── collector.go
│   │   └── collector_test.go
│   └── server/            # HTTP server
│       ├── server.go
│       └── server_test.go
├── Makefile               # Build automation
├── go.mod
├── go.sum
└── README.md
```

## Metrics Collected

The exporter collects all numeric metrics available in your SonarQube instance, including:

- **Reliability**: bugs, vulnerabilities, reliability rating
- **Security**: security hotspots, security rating, vulnerabilities
- **Maintainability**: code smells, technical debt, maintainability rating
- **Coverage**: line coverage, branch coverage
- **Duplications**: duplicated lines, duplicated blocks
- **Size**: lines of code, files, classes, functions
- **Complexity**: cyclomatic complexity, cognitive complexity
- And many more...

Supported SonarQube metric types: `INT`, `FLOAT`, `PERCENT`, `RATING`, `MILLISEC`, `WORK_DUR`. Hidden metrics are excluded.

### Project metrics labels

| Label | Description |
|-------|-------------|
| `project_key` | SonarQube project key |
| `project_name` | Human-readable project name |
| `domain` | Metric domain (e.g., Reliability, Security) |

### Branch metrics labels (`sonarqube_<metric>_branch`)

| Label | Description |
|-------|-------------|
| `project_key` | SonarQube project key |
| `project_name` | Human-readable project name |
| `branch_key` | Branch identifier |
| `domain` | Metric domain |
| `type` | Always `branch` |

The `sonarqube_branch_info` gauge also includes `branch_name` and `is_main` (`"true"` / `"false"`).

### Pull request metrics labels (`sonarqube_<metric>_pull_request`)

| Label | Description |
|-------|-------------|
| `project_key` | SonarQube project key |
| `project_name` | Human-readable project name |
| `pr_key` | Pull request ID |
| `domain` | Metric domain |
| `type` | Always `pull_request` |

The `sonarqube_pull_request_info` gauge also includes `pr_branch` (source branch), `pr_target` (target branch), and `pr_status` (quality gate status: `OK`, `ERROR`, etc.).

## Docker Support

You can also run the exporter using Docker:

```bash
# Build Docker image
docker build -t sonarqube-exporter .

# Run container
docker run -d \
  -p 9090:9090 \
  -e SONARQUBE_URL=https://sonar.example.com \
  -e SONARQUBE_TOKEN=your-token-here \
  -e REFRESH_INTERVAL=60 \
  -e COLLECT_BRANCHES=true \
  -e COLLECT_PULL_REQUESTS=true \
  sonarqube-exporter
```

### Docker Compose Example

```yaml
version: '3'
services:
  sonarqube-exporter:
    image: sonarqube-exporter
    ports:
      - "9090:9090"
    environment:
      - SONARQUBE_URL=https://sonar.example.com
      - SONARQUBE_TOKEN=your-token-here
      - REFRESH_INTERVAL=60
      - COLLECT_BRANCHES=true
      - COLLECT_PULL_REQUESTS=true
    restart: unless-stopped
```

## Troubleshooting

### Connection Issues

If you're experiencing connection issues to SonarQube:

1. Verify the SonarQube URL is correct
2. Check that the token has the necessary permissions
3. Ensure network connectivity to the SonarQube server

### No Metrics Available

If no metrics are being exported:

1. Check the exporter logs for errors
2. Verify that your SonarQube instance has projects analyzed
3. Ensure the token has permission to view project metrics

### Health Check Returns 503

In async mode (`-refresh-interval`), the `/health` endpoint returns `503` until the first cache refresh completes. Wait a few seconds after startup for the cache to be populated.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

Apache License 2.0

## Author

Created by nathan-mittelette
