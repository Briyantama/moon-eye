package observability

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// Registry is a shared Prometheus registry for backend services.
	Registry = prometheus.NewRegistry()
)

// Handler returns an HTTP handler that exposes Prometheus metrics.
func Handler() http.Handler {
	// TODO: register default Go and process collectors if desired.
	return promhttp.HandlerFor(Registry, promhttp.HandlerOpts{})
}

