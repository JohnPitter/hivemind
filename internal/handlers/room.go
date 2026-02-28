package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/joaopedro/hivemind/internal/models"
	"github.com/joaopedro/hivemind/internal/services"
)

// RoomHandler handles room lifecycle endpoints.
type RoomHandler struct {
	roomSvc services.RoomService
}

// NewRoomHandler creates a room handler.
func NewRoomHandler(roomSvc services.RoomService) *RoomHandler {
	return &RoomHandler{roomSvc: roomSvc}
}

// CreateRequest holds parameters for creating a room.
type CreateRequest struct {
	ModelID     string          `json:"model_id"`
	ModelType   models.ModelType `json:"model_type"`
	MaxPeers    int             `json:"max_peers"`
	AutoApprove bool            `json:"auto_approve"`
}

// JoinRequest holds parameters for joining a room.
type JoinRequest struct {
	InviteCode string             `json:"invite_code"`
	Resources  models.ResourceSpec `json:"resources"`
}

// Create handles POST /room/create.
func (h *RoomHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body: "+err.Error())
		return
	}

	if req.ModelID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "model_id is required")
		return
	}

	cfg := models.RoomConfig{
		ModelID:     req.ModelID,
		ModelType:   req.ModelType,
		MaxPeers:    req.MaxPeers,
		AutoApprove: req.AutoApprove,
	}

	room, err := h.roomSvc.Create(r.Context(), cfg)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, room)
}

// Join handles POST /room/join.
func (h *RoomHandler) Join(w http.ResponseWriter, r *http.Request) {
	var req JoinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body: "+err.Error())
		return
	}

	if req.InviteCode == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "invite_code is required")
		return
	}

	room, err := h.roomSvc.Join(r.Context(), req.InviteCode, req.Resources)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, room)
}

// Leave handles DELETE /room/leave.
func (h *RoomHandler) Leave(w http.ResponseWriter, r *http.Request) {
	if err := h.roomSvc.Leave(r.Context()); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "left"})
}

// Status handles GET /room/status.
func (h *RoomHandler) Status(w http.ResponseWriter, r *http.Request) {
	status, err := h.roomSvc.Status(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, status)
}
