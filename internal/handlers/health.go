package handlers

import (
	"context"
	"net/http"

	"github.com/joaopedro/hivemind/internal/services"
)

// HealthHandler handles system health check endpoints.
type HealthHandler struct {
	roomSvc services.RoomService
}

// NewHealthHandler creates a health handler.
func NewHealthHandler(roomSvc services.RoomService) *HealthHandler {
	return &HealthHandler{roomSvc: roomSvc}
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
