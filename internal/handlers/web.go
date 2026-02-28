package handlers

import (
	"encoding/json"
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

// RegisterRoutes sets up HTTP routes on the provided mux.
func (h *WebHandler) RegisterRoutes(mux *http.ServeMux) {
	// API routes
	mux.HandleFunc("GET /api/room/status", h.handleRoomStatus)
	mux.HandleFunc("GET /api/health", h.handleHealth)
	mux.HandleFunc("GET /api/models", h.handleModels)

	// Static files (React SPA) — must be last
	fileServer := http.FileServer(http.FS(h.webFS))
	mux.Handle("/", spaHandler(fileServer, h.webFS))
}

func (h *WebHandler) handleRoomStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.roomSvc.Status(r.Context())
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *WebHandler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":          "ok",
		"worker_healthy":  true,
		"peers_connected": 3,
		"model_loaded":    true,
	})
}

func (h *WebHandler) handleModels(w http.ResponseWriter, r *http.Request) {
	models, err := h.infSvc.ListModels(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, models)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// spaHandler serves index.html for any path that doesn't match a static file.
func spaHandler(fileServer http.Handler, webFS fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			path = "index.html"
		} else {
			path = path[1:] // remove leading /
		}

		// Check if file exists
		if _, err := fs.Stat(webFS, path); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback — serve index.html
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
