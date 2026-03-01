package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/joaopedro/hivemind/internal/services"
)

// PrometheusHandler exposes metrics in Prometheus text exposition format.
type PrometheusHandler struct {
	metrics *services.MetricsCollector
}

// NewPrometheusHandler creates a Prometheus metrics handler.
func NewPrometheusHandler(metrics *services.MetricsCollector) *PrometheusHandler {
	return &PrometheusHandler{metrics: metrics}
}

// Metrics handles GET /metrics/prometheus.
func (h *PrometheusHandler) Metrics(w http.ResponseWriter, _ *http.Request) {
	if h.metrics == nil {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		return
	}

	snap := h.metrics.Snapshot()

	var b strings.Builder

	writeCounter(&b, "hivemind_inference_requests_total", "Total number of inference requests.", snap.InferenceTotal)
	writeCounter(&b, "hivemind_inference_errors_total", "Total number of inference errors.", snap.InferenceErrors)
	writeGaugeFloat(&b, "hivemind_inference_avg_latency_ms", "Average inference latency in milliseconds.", snap.InferenceAvgMs)
	writeCounter(&b, "hivemind_streaming_chunks_total", "Total number of streaming chunks sent.", snap.StreamingChunks)
	writeCounter(&b, "hivemind_tensor_transfers_total", "Total number of tensor transfers.", snap.TensorTransfers)
	writeCounter(&b, "hivemind_tensor_bytes_total", "Total bytes transferred for tensors.", snap.TensorBytesTotal)
	writeCounter(&b, "hivemind_tensor_errors_total", "Total number of tensor transfer errors.", snap.TensorErrors)
	writeCounter(&b, "hivemind_compression_saved_bytes_total", "Total bytes saved by compression.", snap.CompressionSaved)
	writeCounter(&b, "hivemind_peer_connections_total", "Total number of peer connections.", snap.PeerConnects)
	writeCounter(&b, "hivemind_peer_disconnections_total", "Total number of peer disconnections.", snap.PeerDisconnects)
	writeCounter(&b, "hivemind_health_checks_failed_total", "Total number of failed health checks.", snap.HealthCheckFails)
	writeGaugeFloat(&b, "hivemind_latency_p50_ms", "50th percentile latency in milliseconds.", snap.P50LatencyMs)
	writeGaugeFloat(&b, "hivemind_latency_p95_ms", "95th percentile latency in milliseconds.", snap.P95LatencyMs)
	writeGaugeFloat(&b, "hivemind_latency_p99_ms", "99th percentile latency in milliseconds.", snap.P99LatencyMs)

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, b.String())
}

func writeCounter(b *strings.Builder, name, help string, value int64) {
	fmt.Fprintf(b, "# HELP %s %s\n", name, help)
	fmt.Fprintf(b, "# TYPE %s counter\n", name)
	fmt.Fprintf(b, "%s %d\n\n", name, value)
}

func writeGaugeFloat(b *strings.Builder, name, help string, value float64) {
	fmt.Fprintf(b, "# HELP %s %s\n", name, help)
	fmt.Fprintf(b, "# TYPE %s gauge\n", name)
	fmt.Fprintf(b, "%s %.6f\n\n", name, value)
}
