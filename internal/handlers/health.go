package handlers

import (
	"context"
	"net/http"

	"github.com/joaopedro/hivemind/internal/services"
)

// HealthHandler handles system health check endpoints.
type HealthHandler struct {
	roomSvc services.RoomService
	metrics *services.MetricsCollector
}

// NewHealthHandler creates a health handler.
func NewHealthHandler(roomSvc services.RoomService) *HealthHandler {
	return &HealthHandler{
		roomSvc: roomSvc,
		metrics: services.NewMetricsCollector(),
	}
}

// SetMetrics injects an external metrics collector.
func (h *HealthHandler) SetMetrics(m *services.MetricsCollector) {
	h.metrics = m
}

// HealthResponse contains system health information.
type HealthResponse struct {
	Status         string `json:"status"`
	WorkerHealthy  bool   `json:"worker_healthy"`
	PeersConnected int    `json:"peers_connected"`
	ModelLoaded    bool   `json:"model_loaded"`
}

// Health handles GET /health.
func (h *HealthHandler) Health(w http.ResponseWriter, _ *http.Request) {
	resp := HealthResponse{
		Status:         "ok",
		WorkerHealthy:  true,
		PeersConnected: 0,
		ModelLoaded:    false,
	}

	// Try to get room status for peer count
	status, err := h.roomSvc.Status(context.Background())
	if err == nil {
		resp.PeersConnected = len(status.Room.Peers)
		resp.ModelLoaded = status.Room.ModelID != ""
	}

	writeJSON(w, http.StatusOK, resp)
}

// Metrics handles GET /metrics returning observability data.
func (h *HealthHandler) Metrics(w http.ResponseWriter, _ *http.Request) {
	if h.metrics == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "no metrics available"})
		return
	}

	writeJSON(w, http.StatusOK, h.metrics.Snapshot())
}
