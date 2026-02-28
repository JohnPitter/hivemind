package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/joaopedro/hivemind/internal/handlers"
	"github.com/joaopedro/hivemind/internal/models"
	"github.com/joaopedro/hivemind/internal/services"
)

func setupInferenceHandler() *handlers.InferenceHandler {
	roomSvc := services.NewMockRoomService()
	// Create a room so inference works
	roomSvc.Create(nil, models.RoomConfig{
		ModelID:   "test-model",
		ModelType: models.ModelTypeLLM,
	})
	infSvc := services.NewMockInferenceService(roomSvc)
	return handlers.NewInferenceHandler(infSvc)
}

func TestChatCompletions_NonStreaming(t *testing.T) {
	h := setupInferenceHandler()

	body := models.ChatRequest{
		Model: "test-model",
		Messages: []models.ChatMessage{
			{Role: "user", Content: "Hello"},
		},
		Temperature: 0.7,
		MaxTokens:   100,
		Stream:      false,
	}

	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ChatCompletions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp models.ChatResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Object != "chat.completion" {
		t.Errorf("expected object 'chat.completion', got %q", resp.Object)
	}

	if len(resp.Choices) == 0 {
		t.Error("expected at least one choice")
	}

	if resp.Choices[0].Message.Role != "assistant" {
		t.Errorf("expected role 'assistant', got %q", resp.Choices[0].Message.Role)
	}
}

func TestChatCompletions_EmptyMessages(t *testing.T) {
	h := setupInferenceHandler()

	body := models.ChatRequest{
		Model:    "test-model",
		Messages: []models.ChatMessage{},
	}

	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ChatCompletions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestChatCompletions_InvalidJSON(t *testing.T) {
	h := setupInferenceHandler()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ChatCompletions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestChatCompletions_Streaming(t *testing.T) {
	h := setupInferenceHandler()

	body := models.ChatRequest{
		Model: "test-model",
		Messages: []models.ChatMessage{
			{Role: "user", Content: "Hello streaming"},
		},
		Temperature: 0.7,
		MaxTokens:   100,
		Stream:      true,
	}

	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ChatCompletions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "text/event-stream" {
		t.Errorf("expected Content-Type 'text/event-stream', got %q", contentType)
	}

	respBody := rec.Body.String()
	if len(respBody) == 0 {
		t.Error("expected non-empty streaming response")
	}

	// Should contain SSE data lines
	if !bytes.Contains(rec.Body.Bytes(), []byte("data: ")) {
		t.Error("expected SSE data lines in response")
	}

	// Should end with [DONE]
	if !bytes.Contains(rec.Body.Bytes(), []byte("[DONE]")) {
		t.Error("expected [DONE] at end of stream")
	}
}

func TestListModels(t *testing.T) {
	h := setupInferenceHandler()

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()

	h.ListModels(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp models.ModelList
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if resp.Object != "list" {
		t.Errorf("expected object 'list', got %q", resp.Object)
	}
}
