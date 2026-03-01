package handlers

import (
	"io/fs"
	"net/http"

	"github.com/joaopedro/hivemind/internal/services"
)

// WebHandler serves the embedded React dashboard and API endpoints.
type WebHandler struct {
	roomSvc services.RoomService
	infSvc  services.InferenceService
	webFS   fs.FS
}

// NewWebHandler creates a handler for the web dashboard.
func NewWebHandler(webFS fs.FS, roomSvc services.RoomService, infSvc services.InferenceService) *WebHandler {
	return &WebHandler{
		roomSvc: roomSvc,
		infSvc:  infSvc,
		webFS:   webFS,
	}
}

// RegisterRoutes sets up HTTP routes on the provided mux (used by standalone web command).
func (h *WebHandler) RegisterRoutes(mux *http.ServeMux) {
	// API routes
	mux.HandleFunc("GET /api/room/status", h.HandleRoomStatusJSON)
	mux.HandleFunc("GET /api/health", h.HandleHealthJSON)
	mux.HandleFunc("GET /api/models", h.HandleModelsJSON)

	// Static files (React SPA) — must be last
	fileServer := http.FileServer(http.FS(h.webFS))
	mux.Handle("/", SPAHandler(fileServer, h.webFS))
}

// HandleRoomStatusJSON serves room status as JSON (used by both standalone and chi-based server).
func (h *WebHandler) HandleRoomStatusJSON(w http.ResponseWriter, r *http.Request) {
	roomID := r.URL.Query().Get("room_id")
	if roomID == "" {
		roomID = h.roomSvc.ActiveRoomID()
	}
	status, err := h.roomSvc.Status(r.Context(), roomID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// HandleHealthJSON serves health status as JSON.
func (h *WebHandler) HandleHealthJSON(w http.ResponseWriter, _ *http.Request) {
	room := h.roomSvc.CurrentRoom()
	modelLoaded := room != nil
	peerCount := 0
	if room != nil {
		peerCount = len(room.Peers)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":          "ok",
		"worker_healthy":  modelLoaded,
		"peers_connected": peerCount,
		"model_loaded":    modelLoaded,
	})
}

// HandleModelsJSON serves model list as JSON.
func (h *WebHandler) HandleModelsJSON(w http.ResponseWriter, r *http.Request) {
	models, err := h.infSvc.ListModels(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, models)
}
