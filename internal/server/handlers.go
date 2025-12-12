package server

import (
	"fmt"
	"net/http"
)

// healthHandler handles health check requests
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "OK")
}

// rootHandler handles requests to the root endpoint
func rootHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
    <title>SonarQube Prometheus Exporter</title>
</head>
<body>
    <h1>SonarQube Prometheus Exporter</h1>
    <p>Available endpoints:</p>
    <ul>
        <li><a href="/metrics">/metrics</a> - Prometheus metrics</li>
        <li><a href="/health">/health</a> - Health check</li>
    </ul>
</body>
</html>`)
}
