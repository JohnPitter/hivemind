package services

import (
	"sync"
	"sync/atomic"
	"time"
)

// MetricsCollector tracks system-wide metrics for observability.
// All metrics are lock-free atomics for high-performance concurrent access.
type MetricsCollector struct {
	// Inference metrics
	InferenceRequests   atomic.Int64
	InferenceErrors     atomic.Int64
	InferenceLatencySum atomic.Int64 // sum of latency in ms for averaging
	StreamingChunks     atomic.Int64

	// Tensor transfer metrics
	TensorTransfers   atomic.Int64
	TensorBytes       atomic.Int64
	TensorErrors      atomic.Int64
	CompressionSaved  atomic.Int64 // bytes saved by compression

	// Peer metrics
	PeerConnections    atomic.Int64
	PeerDisconnections atomic.Int64
	HealthChecksFailed atomic.Int64

	// System uptime
	startTime time.Time

	// Request duration histogram (simplified)
	mu        sync.Mutex
	durations []float64
}

// NewMetricsCollector creates a metrics collector.
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		startTime: time.Now(),
		durations: make([]float64, 0, 1000),
	}
}

// RecordInferenceRequest records an inference request with its duration.
func (mc *MetricsCollector) RecordInferenceRequest(durationMs float64, err error) {
	mc.InferenceRequests.Add(1)
	mc.InferenceLatencySum.Add(int64(durationMs))

	if err != nil {
		mc.InferenceErrors.Add(1)
	}

	mc.mu.Lock()
	mc.durations = append(mc.durations, durationMs)
	// Keep last 1000 durations
	if len(mc.durations) > 1000 {
		mc.durations = mc.durations[len(mc.durations)-1000:]
	}
	mc.mu.Unlock()
}

// RecordTensorTransfer records a tensor transfer.
func (mc *MetricsCollector) RecordTensorTransfer(bytes int64, compressionSaved int64, err error) {
	mc.TensorTransfers.Add(1)
	mc.TensorBytes.Add(bytes)
	mc.CompressionSaved.Add(compressionSaved)

	if err != nil {
		mc.TensorErrors.Add(1)
	}
}

// RecordPeerEvent records a peer connection or disconnection.
func (mc *MetricsCollector) RecordPeerEvent(connected bool) {
	if connected {
		mc.PeerConnections.Add(1)
	} else {
		mc.PeerDisconnections.Add(1)
	}
}

// MetricsSnapshot represents a point-in-time snapshot of all metrics.
type MetricsSnapshot struct {
	Uptime string `json:"uptime"`

	// Inference
	InferenceTotal    int64   `json:"inference_total"`
	InferenceErrors   int64   `json:"inference_errors"`
	InferenceAvgMs    float64 `json:"inference_avg_ms"`
	StreamingChunks   int64   `json:"streaming_chunks"`

	// Tensor
	TensorTransfers   int64  `json:"tensor_transfers"`
	TensorBytesTotal  int64  `json:"tensor_bytes_total"`
	TensorErrors      int64  `json:"tensor_errors"`
	CompressionSaved  int64  `json:"compression_saved_bytes"`

	// Peers
	PeerConnects      int64   `json:"peer_connects"`
	PeerDisconnects   int64   `json:"peer_disconnects"`
	HealthCheckFails  int64   `json:"health_check_fails"`

	// Latency distribution
	P50LatencyMs     float64 `json:"p50_latency_ms"`
	P95LatencyMs     float64 `json:"p95_latency_ms"`
	P99LatencyMs     float64 `json:"p99_latency_ms"`
}

// Snapshot returns a point-in-time snapshot of all metrics.
func (mc *MetricsCollector) Snapshot() MetricsSnapshot {
	total := mc.InferenceRequests.Load()
	latencySum := mc.InferenceLatencySum.Load()

	avgMs := float64(0)
	if total > 0 {
		avgMs = float64(latencySum) / float64(total)
	}

	mc.mu.Lock()
	durationsCopy := make([]float64, len(mc.durations))
	copy(durationsCopy, mc.durations)
	mc.mu.Unlock()

	p50, p95, p99 := computePercentiles(durationsCopy)

	return MetricsSnapshot{
		Uptime:           time.Since(mc.startTime).Round(time.Second).String(),
		InferenceTotal:   total,
		InferenceErrors:  mc.InferenceErrors.Load(),
		InferenceAvgMs:   avgMs,
		StreamingChunks:  mc.StreamingChunks.Load(),
		TensorTransfers:  mc.TensorTransfers.Load(),
		TensorBytesTotal: mc.TensorBytes.Load(),
		TensorErrors:     mc.TensorErrors.Load(),
		CompressionSaved: mc.CompressionSaved.Load(),
		PeerConnects:     mc.PeerConnections.Load(),
		PeerDisconnects:  mc.PeerDisconnections.Load(),
		HealthCheckFails: mc.HealthChecksFailed.Load(),
		P50LatencyMs:     p50,
		P95LatencyMs:     p95,
		P99LatencyMs:     p99,
	}
}

func computePercentiles(durations []float64) (p50, p95, p99 float64) {
	n := len(durations)
	if n == 0 {
		return 0, 0, 0
	}

	// Sort for percentile calculation
	sorted := make([]float64, n)
	copy(sorted, durations)
	sortFloat64s(sorted)

	p50 = sorted[n*50/100]
	p95 = sorted[min(n*95/100, n-1)]
	p99 = sorted[min(n*99/100, n-1)]

	return p50, p95, p99
}

func sortFloat64s(a []float64) {
	// Simple insertion sort (fine for ≤1000 elements)
	for i := 1; i < len(a); i++ {
		key := a[i]
		j := i - 1
		for j >= 0 && a[j] > key {
			a[j+1] = a[j]
			j--
		}
		a[j+1] = key
	}
}
