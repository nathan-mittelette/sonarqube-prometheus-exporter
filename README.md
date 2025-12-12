# SonarQube Prometheus Exporter

A lightweight Go application that exports SonarQube metrics in Prometheus format.

## Features

- Fetches metrics from SonarQube API
- Exposes metrics in Prometheus format via `/metrics` endpoint
- Supports configuration via CLI flags or environment variables
- Includes health check endpoint
- Comprehensive test coverage
- Cross-platform support

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
  -sonarqube-token your-token-here
```

### Environment Variables

```bash
export EXPORTER_HOST=0.0.0.0
export EXPORTER_PORT=9090
export SONARQUBE_URL=https://sonar.example.com
export SONARQUBE_TOKEN=your-token-here

./bin/sonarqube-exporter
```

### Configuration Options

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `-host` | `EXPORTER_HOST` | `0.0.0.0` | Host to bind the exporter server |
| `-port` | `EXPORTER_PORT` | `9090` | Port to bind the exporter server |
| `-sonarqube-url` | `SONARQUBE_URL` | *required* | SonarQube server URL |
| `-sonarqube-token` | `SONARQUBE_TOKEN` | *required* | SonarQube authentication token |

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

Each metric is labeled with:
- `project_key`: The SonarQube project key
- `project_name`: The SonarQube project name
- `domain`: The metric domain (e.g., Reliability, Security)

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

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

Apache License 2.0

## Author

Created by nathan-mittelette
