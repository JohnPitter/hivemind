package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/joaopedro/hivemind/internal/models"
	"github.com/joaopedro/hivemind/internal/services"
)

// InferenceHandler handles OpenAI-compatible inference endpoints.
type InferenceHandler struct {
	infSvc services.InferenceService
}

// NewInferenceHandler creates an inference handler.
func NewInferenceHandler(infSvc services.InferenceService) *InferenceHandler {
	return &InferenceHandler{infSvc: infSvc}
}

// ChatCompletions handles POST /v1/chat/completions.
func (h *InferenceHandler) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	var req models.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body: "+err.Error())
		return
	}

	if len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "messages array is required and cannot be empty")
		return
	}

	if req.Stream {
		h.handleStream(w, r, req)
		return
	}

	resp, err := h.infSvc.ChatCompletion(r.Context(), req)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleStream sends SSE streaming response for chat completions.
func (h *InferenceHandler) handleStream(w http.ResponseWriter, r *http.Request, req models.ChatRequest) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming_error", "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch := make(chan models.ChatChunk, 100)
	errCh := make(chan error, 1)

	go func() {
		errCh <- h.infSvc.ChatCompletionStream(r.Context(), req, ch)
	}()

	for chunk := range ch {
		data, err := json.Marshal(chunk)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	if err := <-errCh; err != nil {
		fmt.Fprintf(w, "data: {\"error\":\"%s\"}\n\n", err.Error())
		flusher.Flush()
	}

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// ImageGeneration handles POST /v1/images/generations.
func (h *InferenceHandler) ImageGeneration(w http.ResponseWriter, r *http.Request) {
	var req models.ImageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body: "+err.Error())
		return
	}

	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "prompt is required")
		return
	}

	resp, err := h.infSvc.ImageGeneration(r.Context(), req)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// ListModels handles GET /v1/models.
func (h *InferenceHandler) ListModels(w http.ResponseWriter, r *http.Request) {
	modelList, err := h.infSvc.ListModels(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, modelList)
}
