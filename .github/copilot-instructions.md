# Copilot Instructions

## Build, test, and lint commands

```bash
# Build
make build
make build-all

# Tests
make test
make coverage

# Run a single test or subset
go test -v ./internal/config -run TestLoad
go test -v ./internal/metrics -run TestCollect_WithBranches

# Formatting and static analysis
make fmt
make vet
make lint

# CI parity checks
gofmt -s -l .
go test -v -race -coverprofile=coverage.out ./...
go build -v ./...
```

## High-level architecture

- `cmd/exporter/main.go` is the composition root: it loads config, creates the SonarQube client, builds the Prometheus collector, starts the HTTP server, and stops async refresh before graceful shutdown.
- `internal/config` owns all runtime configuration. CLI flags are defined with environment-variable defaults, so environment variables effectively provide the default values for flags.
- `internal/sonarqube` is a thin typed wrapper around the SonarQube REST API. It paginates `/api/components/search_projects`, uses a 30-second HTTP timeout, and treats 404 from branch/PR listing endpoints as "feature unavailable / no data" by returning an empty slice.
- `internal/metrics` is the core of the application. The collector discovers metric definitions from `/api/metrics/search`, keeps only numeric non-hidden metrics, then fetches project measures and optionally branch/PR data before exporting Prometheus metrics.
- The collector supports two execution modes:
  - **Sync mode** (`refresh-interval=0`): each `/metrics` scrape refreshes data on demand.
  - **Async mode** (`refresh-interval>0`): a background refresh loop populates a cache and scrapes read from that cache.
- Async refresh is intentionally two-phase: fetch project measures plus branch/PR lists first, then fetch branch/PR measures once the full task list is known. Concurrency is capped with `maxConcurrentFetches = 20`.
- `internal/server` builds a dedicated Prometheus registry and exposes `/`, `/health`, and `/metrics`. In async mode, `/health` returns `503` until the collector’s `CacheReady()` channel closes.

## Key conventions

- Preserve the current metric naming contract. SonarQube metric keys are sanitized by replacing `-` and `.` with `_`, lowercasing, and prefixing with `sonarqube_`. Branch and PR metrics use suffixes (`_branch`, `_pull_request`) rather than prefixes to avoid collisions with SonarQube metric keys such as `branch_coverage`.
- Preserve backward compatibility for project metrics. Project metrics keep their existing label shape, while branch/PR metrics add a constant `type` label and use dedicated info metrics (`sonarqube_branch_info`, `sonarqube_pull_request_info`).
- Empty SonarQube measure values are exported as `0`, and only SonarQube metric types `INT`, `FLOAT`, `PERCENT`, `RATING`, `MILLISEC`, and `WORK_DUR` are exported.
- When refreshing cached data, metric descriptors and cache contents are swapped together under the same lock. If metric definitions change, descriptors must be rebuilt; otherwise Prometheus can reject metrics with stale help text.
- In sync mode, the HTTP server intentionally leaves `WriteTimeout` unset because large scrapes can take a long time before any response is written.
- Tests favor `httptest.NewServer` with mocked SonarQube JSON responses over real SonarQube instances. For config parsing tests, use a fresh `flag.FlagSet` and `LoadWithFlagSet(...)` instead of `flag.CommandLine`.
