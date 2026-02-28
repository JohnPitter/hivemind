package handlers

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"

	"github.com/joaopedro/hivemind/internal/models"
)

// APIError represents a standard error response following OpenAI error format.
type APIError struct {
	Error APIErrorDetail `json:"error"`
}

// APIErrorDetail holds the error details.
type APIErrorDetail struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError writes a standard error response.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, APIError{
		Error: APIErrorDetail{
			Message: message,
			Code:    code,
		},
	})
}

// handleServiceError maps domain errors to HTTP status codes.
func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, models.ErrNotInRoom):
		writeError(w, http.StatusNotFound, "not_in_room", err.Error())
	case errors.Is(err, models.ErrRoomNotFound):
		writeError(w, http.StatusNotFound, "room_not_found", err.Error())
	case errors.Is(err, models.ErrRoomFull):
		writeError(w, http.StatusConflict, "room_full", err.Error())
	case errors.Is(err, models.ErrRoomClosed):
		writeError(w, http.StatusGone, "room_closed", err.Error())
	case errors.Is(err, models.ErrAlreadyInRoom):
		writeError(w, http.StatusConflict, "already_in_room", err.Error())
	case errors.Is(err, models.ErrInvalidInvite):
		writeError(w, http.StatusBadRequest, "invalid_invite", err.Error())
	case errors.Is(err, models.ErrModelNotLoaded):
		writeError(w, http.StatusServiceUnavailable, "model_not_loaded", err.Error())
	case errors.Is(err, models.ErrWorkerUnavail):
		writeError(w, http.StatusServiceUnavailable, "worker_unavailable", err.Error())
	case errors.Is(err, models.ErrInferenceTimeout):
		writeError(w, http.StatusGatewayTimeout, "inference_timeout", err.Error())
	case errors.Is(err, models.ErrNotHost):
		writeError(w, http.StatusForbidden, "not_host", err.Error())
	case errors.Is(err, models.ErrInsufficientVRAM):
		writeError(w, http.StatusUnprocessableEntity, "insufficient_vram", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "an unexpected error occurred")
	}
}

// SPAHandler serves index.html for any path that doesn't match a static file.
func SPAHandler(fileServer http.Handler, webFS fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			path = "index.html"
		} else if len(path) > 1 {
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
