# Build the manager binary
FROM golang:1.25 AS builder

WORKDIR /app
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY cmd/ cmd/
COPY internal/ internal/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o sonarqube-prometheus-exporter cmd/exporter/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /app
COPY --from=builder /app/sonarqube-prometheus-exporter .
USER nonroot:nonroot

ENTRYPOINT ["/app/sonarqube-prometheus-exporter"]
