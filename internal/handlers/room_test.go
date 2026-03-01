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

func setupRoomHandler() (*handlers.RoomHandler, services.RoomService) {
	roomSvc := services.NewMockRoomService()
	return handlers.NewRoomHandler(roomSvc), roomSvc
}

func TestRoomCreate(t *testing.T) {
	h, _ := setupRoomHandler()

	body := handlers.CreateRequest{
		ModelID:   "meta-llama/Llama-3-70B",
		ModelType: models.ModelTypeLLM,
		MaxPeers:  6,
	}

	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/room/create", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp models.CreateRoomResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if resp.Room == nil {
		t.Fatal("expected non-nil room")
	}

	if resp.Room.ModelID != "meta-llama/Llama-3-70B" {
		t.Errorf("expected model ID 'meta-llama/Llama-3-70B', got %q", resp.Room.ModelID)
	}

	if resp.Room.InviteCode == "" {
		t.Error("expected non-empty invite code")
	}

	if resp.ResourceCheck == nil {
		t.Fatal("expected non-nil resource check for cataloged model")
	}
}

func TestRoomCreate_MissingModel(t *testing.T) {
	h, _ := setupRoomHandler()

	body := handlers.CreateRequest{}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/room/create", bytes.NewReader(bodyBytes))
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestRoomCreate_InsufficientResources_Pending(t *testing.T) {
	h, _ := setupRoomHandler()

	// Create with 70B model and small GPU — should go pending
	body := handlers.CreateRequest{
		ModelID:  "meta-llama/Llama-3-70B",
		MaxPeers: 5,
		Resources: &models.ResourceSpec{
			GPUName:   "NVIDIA RTX 3060",
			VRAMTotal: 12288,
			VRAMFree:  10240,
			CUDAAvail: true,
			Platform:  "Linux",
		},
	}

	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/room/create", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp models.CreateRoomResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if resp.Room.State != models.RoomStatePending {
		t.Errorf("expected pending state, got %q", resp.Room.State)
	}

	if resp.PendingTimeout != 300 {
		t.Errorf("expected pending timeout 300, got %d", resp.PendingTimeout)
	}

	if resp.ResourceCheck == nil {
		t.Fatal("expected resource check")
	}

	if resp.ResourceCheck.Sufficient {
		t.Error("expected insufficient resources")
	}

	if resp.ResourceCheck.DeficitMB <= 0 {
		t.Error("expected positive deficit")
	}

	if resp.ResourceCheck.SuggestedModelID == "" {
		t.Error("expected a suggested model")
	}
}

func TestRoomCreate_AutoFillModelType(t *testing.T) {
	h, _ := setupRoomHandler()

	// Don't provide ModelType — should auto-fill from catalog
	body := handlers.CreateRequest{
		ModelID:  "stabilityai/stable-diffusion-xl",
		MaxPeers: 3,
	}

	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/room/create", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp models.CreateRoomResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if resp.Room.ModelType != models.ModelTypeDiffusion {
		t.Errorf("expected auto-filled model type 'diffusion', got %q", resp.Room.ModelType)
	}
}

func TestRoomStatus_NotInRoom(t *testing.T) {
	h, _ := setupRoomHandler()

	req := httptest.NewRequest(http.MethodGet, "/room/status", nil)
	rec := httptest.NewRecorder()

	h.Status(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRoomStatus_InRoom(t *testing.T) {
	h, roomSvc := setupRoomHandler()

	// Create a room first
	roomSvc.Create(nil, models.RoomConfig{
		ModelID:   "test-model",
		ModelType: models.ModelTypeLLM,
	})

	req := httptest.NewRequest(http.MethodGet, "/room/status", nil)
	rec := httptest.NewRecorder()

	h.Status(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var status models.RoomStatus
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if status.Room.ModelID != "test-model" {
		t.Errorf("expected model 'test-model', got %q", status.Room.ModelID)
	}
}

func TestRoomLeave_NotInRoom(t *testing.T) {
	h, _ := setupRoomHandler()

	req := httptest.NewRequest(http.MethodDelete, "/room/leave", nil)
	rec := httptest.NewRecorder()

	h.Leave(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}
