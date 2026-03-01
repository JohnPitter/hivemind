package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/joaopedro/hivemind/internal/handlers"
	"github.com/joaopedro/hivemind/internal/services"
)

func TestPrometheusMetrics_ContentType(t *testing.T) {
	mc := services.NewMetricsCollector()
	h := handlers.NewPrometheusHandler(mc)

	req := httptest.NewRequest(http.MethodGet, "/metrics/prometheus", nil)
	rec := httptest.NewRecorder()

	h.Metrics(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	expected := "text/plain; version=0.0.4; charset=utf-8"
	if ct != expected {
		t.Errorf("expected Content-Type %q, got %q", expected, ct)
	}
}

func TestPrometheusMetrics_ContainsAllMetrics(t *testing.T) {
	mc := services.NewMetricsCollector()
	mc.RecordInferenceRequest(42.5, nil)
	mc.RecordTensorTransfer(1024, 256, nil)
	mc.RecordPeerEvent(true)

	h := handlers.NewPrometheusHandler(mc)

	req := httptest.NewRequest(http.MethodGet, "/metrics/prometheus", nil)
	rec := httptest.NewRecorder()

	h.Metrics(rec, req)

	body := rec.Body.String()

	requiredMetrics := []string{
		"hivemind_inference_requests_total",
		"hivemind_inference_errors_total",
		"hivemind_inference_avg_latency_ms",
		"hivemind_streaming_chunks_total",
		"hivemind_tensor_transfers_total",
		"hivemind_tensor_bytes_total",
		"hivemind_tensor_errors_total",
		"hivemind_compression_saved_bytes_total",
		"hivemind_peer_connections_total",
		"hivemind_peer_disconnections_total",
		"hivemind_health_checks_failed_total",
		"hivemind_latency_p50_ms",
		"hivemind_latency_p95_ms",
		"hivemind_latency_p99_ms",
	}

	for _, metric := range requiredMetrics {
		if !strings.Contains(body, metric) {
			t.Errorf("missing metric %q in output", metric)
		}
	}
}

func TestPrometheusMetrics_Format(t *testing.T) {
	mc := services.NewMetricsCollector()
	mc.RecordInferenceRequest(100.0, nil)

	h := handlers.NewPrometheusHandler(mc)

	req := httptest.NewRequest(http.MethodGet, "/metrics/prometheus", nil)
	rec := httptest.NewRecorder()

	h.Metrics(rec, req)

	body := rec.Body.String()

	// Verify HELP and TYPE lines exist for a counter
	if !strings.Contains(body, "# HELP hivemind_inference_requests_total") {
		t.Error("missing HELP line for hivemind_inference_requests_total")
	}
	if !strings.Contains(body, "# TYPE hivemind_inference_requests_total counter") {
		t.Error("missing TYPE line for hivemind_inference_requests_total")
	}
	if !strings.Contains(body, "hivemind_inference_requests_total 1") {
		t.Error("expected hivemind_inference_requests_total to be 1")
	}

	// Verify HELP and TYPE lines exist for a gauge
	if !strings.Contains(body, "# HELP hivemind_inference_avg_latency_ms") {
		t.Error("missing HELP line for hivemind_inference_avg_latency_ms")
	}
	if !strings.Contains(body, "# TYPE hivemind_inference_avg_latency_ms gauge") {
		t.Error("missing TYPE line for hivemind_inference_avg_latency_ms")
	}
}

func TestPrometheusMetrics_Values(t *testing.T) {
	mc := services.NewMetricsCollector()
	mc.RecordInferenceRequest(50.0, nil)
	mc.RecordInferenceRequest(150.0, nil)
	mc.RecordTensorTransfer(2048, 512, nil)
	mc.RecordPeerEvent(true)
	mc.RecordPeerEvent(false)

	h := handlers.NewPrometheusHandler(mc)

	req := httptest.NewRequest(http.MethodGet, "/metrics/prometheus", nil)
	rec := httptest.NewRecorder()

	h.Metrics(rec, req)

	body := rec.Body.String()

	// Check counter values
	if !strings.Contains(body, "hivemind_inference_requests_total 2") {
		t.Error("expected inference_requests_total to be 2")
	}
	if !strings.Contains(body, "hivemind_tensor_bytes_total 2048") {
		t.Error("expected tensor_bytes_total to be 2048")
	}
	if !strings.Contains(body, "hivemind_peer_connections_total 1") {
		t.Error("expected peer_connections_total to be 1")
	}
	if !strings.Contains(body, "hivemind_peer_disconnections_total 1") {
		t.Error("expected peer_disconnections_total to be 1")
	}
}

func TestPrometheusMetrics_NilCollector(t *testing.T) {
	h := handlers.NewPrometheusHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/metrics/prometheus", nil)
	rec := httptest.NewRecorder()

	h.Metrics(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if rec.Body.Len() != 0 {
		t.Errorf("expected empty body for nil collector, got %q", rec.Body.String())
	}
}
