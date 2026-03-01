package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/joaopedro/hivemind/internal/handlers"
	"github.com/joaopedro/hivemind/internal/models"
)

// --- Scenario: User creates room via API with donation percentage ---
func TestRoomCreate_WithDonation_Active(t *testing.T) {
	h, _ := setupRoomHandler()

	body := handlers.CreateRequest{
		ModelID:   "TinyLlama/TinyLlama-1.1B",
		ModelType: models.ModelTypeLLM,
		MaxPeers:  5,
		Resources: &models.ResourceSpec{
			GPUName:     "NVIDIA RTX 3060",
			VRAMTotal:   12288,
			VRAMFree:    10240,
			RAMTotal:    32768,
			RAMFree:     24576,
			CUDAAvail:   true,
			Platform:    "Windows",
			DonationPct: 50,
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

	// TinyLlama needs 2048 MB. With 50% donation: (10240-512)*50/100 = 4864 MB → active
	if resp.Room.State != models.RoomStateActive {
		t.Errorf("expected active state, got %q", resp.Room.State)
	}

	if resp.Room.Peers[0].Resources.DonationPct != 50 {
		t.Errorf("expected DonationPct=50 in peer resources, got %d",
			resp.Room.Peers[0].Resources.DonationPct)
	}

	// Verify resource check reflects donated amount
	if resp.ResourceCheck == nil {
		t.Fatal("expected resource check")
	}
	if !resp.ResourceCheck.Sufficient {
		t.Error("expected sufficient resources for TinyLlama with 50% donation")
	}
	if resp.ResourceCheck.TotalVRAMMB != 4864 {
		t.Errorf("expected TotalVRAMMB=4864, got %d", resp.ResourceCheck.TotalVRAMMB)
	}
}

// --- Scenario: Low donation makes Gemma 2 27B go pending via API ---
func TestRoomCreate_WithLowDonation_Pending(t *testing.T) {
	h, _ := setupRoomHandler()

	body := handlers.CreateRequest{
		ModelID:  "google/gemma-2-27b",
		MaxPeers: 5,
		Resources: &models.ResourceSpec{
			GPUName:     "NVIDIA RTX 3060",
			VRAMTotal:   12288,
			VRAMFree:    10240,
			CUDAAvail:   true,
			Platform:    "Linux",
			DonationPct: 25,
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
		t.Errorf("expected pending timeout 300s, got %d", resp.PendingTimeout)
	}

	if resp.ResourceCheck == nil {
		t.Fatal("expected resource check")
	}

	if resp.ResourceCheck.Sufficient {
		t.Error("expected insufficient resources with 25% donation")
	}

	// Deficit: 18432 - 2432 = 15,1000 MB
	if resp.ResourceCheck.DeficitMB <= 0 {
		t.Error("expected positive deficit")
	}

	// Should suggest a smaller model
	if resp.ResourceCheck.SuggestedModelID == "" {
		t.Error("expected a suggested model that fits")
	}
}

// --- Scenario: User joins via API with donation and gets assigned layers ---
func TestRoomJoin_WithDonation(t *testing.T) {
	h, _ := setupRoomHandler()

	body := handlers.JoinRequest{
		InviteCode: "test-invite-123",
		Resources: models.ResourceSpec{
			GPUName:     "NVIDIA RTX 3070",
			VRAMTotal:   8192,
			VRAMFree:    6144,
			RAMTotal:    16384,
			RAMFree:     12288,
			CUDAAvail:   true,
			Platform:    "Windows",
			DonationPct: 75,
		},
	}

	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/room/join", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Join(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var room models.Room
	if err := json.NewDecoder(rec.Body).Decode(&room); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	// Find self peer
	var self *models.Peer
	for i := range room.Peers {
		if room.Peers[i].ID == "self" {
			self = &room.Peers[i]
			break
		}
	}

	if self == nil {
		t.Fatal("self peer not found")
	}

	if self.Resources.DonationPct != 75 {
		t.Errorf("expected DonationPct=75, got %d", self.Resources.DonationPct)
	}

	// VRAM: (6144-512)*75/100 = 4224 MB
	expectedVRAM := int64(4224)
	if got := self.Resources.TotalUsableVRAM(); got != expectedVRAM {
		t.Errorf("TotalUsableVRAM = %d, want %d", got, expectedVRAM)
	}

	// Should have layers assigned
	if len(self.Layers) == 0 {
		t.Error("expected self to have layers assigned")
	}
}

// --- Scenario: Create with new Code model type auto-fills from catalog ---
func TestRoomCreate_CodeModel_AutoFillType(t *testing.T) {
	h, _ := setupRoomHandler()

	body := handlers.CreateRequest{
		ModelID:  "Qwen/Qwen2.5-Coder-7B",
		MaxPeers: 3,
		// ModelType omitted — should auto-fill to "code"
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

	if resp.Room.ModelType != models.ModelTypeCode {
		t.Errorf("expected auto-filled model type 'code', got %q", resp.Room.ModelType)
	}
}

// --- Scenario: Create with Embedding model auto-fills from catalog ---
func TestRoomCreate_EmbeddingModel_AutoFillType(t *testing.T) {
	h, _ := setupRoomHandler()

	body := handlers.CreateRequest{
		ModelID:  "BAAI/bge-large-en-v1.5",
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

	if resp.Room.ModelType != models.ModelTypeEmbedding {
		t.Errorf("expected auto-filled model type 'embedding', got %q", resp.Room.ModelType)
	}

	// BGE Large needs 1024 MB, default host has ~9728 MB → active
	if resp.Room.State != models.RoomStateActive {
		t.Errorf("expected active state for BGE Large, got %q", resp.Room.State)
	}
}

// --- Scenario: Create with Multimodal model auto-fills from catalog ---
func TestRoomCreate_MultimodalModel_AutoFillType(t *testing.T) {
	h, _ := setupRoomHandler()

	body := handlers.CreateRequest{
		ModelID:  "liuhaotian/LLaVA-13B",
		MaxPeers: 5,
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

	if resp.Room.ModelType != models.ModelTypeMultimodal {
		t.Errorf("expected auto-filled model type 'multimodal', got %q", resp.Room.ModelType)
	}
}

// --- Scenario: Full user flow — create room → check status → leave ---
func TestFullFlow_CreateStatusLeave_WithDonation(t *testing.T) {
	h, _ := setupRoomHandler()

	// Step 1: Create room with code model and 75% donation
	createBody := handlers.CreateRequest{
		ModelID:  "Qwen/Qwen2.5-Coder-7B",
		MaxPeers: 4,
		Resources: &models.ResourceSpec{
			GPUName:     "NVIDIA RTX 4070",
			VRAMTotal:   12288,
			VRAMFree:    10240,
			RAMTotal:    32768,
			RAMFree:     28672,
			CUDAAvail:   true,
			Platform:    "Linux",
			DonationPct: 75,
		},
	}

	bodyBytes, _ := json.Marshal(createBody)
	req := httptest.NewRequest(http.MethodPost, "/room/create", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var createResp models.CreateRoomResponse
	json.NewDecoder(rec.Body).Decode(&createResp)

	if createResp.Room.State != models.RoomStateActive {
		t.Fatalf("expected active, got %q", createResp.Room.State)
	}

	// Step 2: Check status
	req = httptest.NewRequest(http.MethodGet, "/room/status", nil)
	rec = httptest.NewRecorder()

	h.Status(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var status models.RoomStatus
	json.NewDecoder(rec.Body).Decode(&status)

	if status.Room.ModelID != "Qwen/Qwen2.5-Coder-7B" {
		t.Errorf("status model = %q, want Qwen/Qwen2.5-Coder-7B", status.Room.ModelID)
	}

	if status.TotalVRAM == 0 {
		t.Error("expected non-zero total VRAM in status")
	}

	// Step 3: Leave
	req = httptest.NewRequest(http.MethodDelete, "/room/leave", nil)
	rec = httptest.NewRecorder()

	h.Leave(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("leave: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Step 4: Status should 404 after leaving
	req = httptest.NewRequest(http.MethodGet, "/room/status", nil)
	rec = httptest.NewRecorder()

	h.Status(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status after leave: expected 404, got %d", rec.Code)
	}
}

// --- Scenario: Catalog endpoint returns all 20 models with new types ---
func TestListCatalog_ReturnsExpandedCatalog(t *testing.T) {
	h := handlers.NewCatalogHandler()

	req := httptest.NewRequest(http.MethodGet, "/v1/models/catalog", nil)
	rec := httptest.NewRecorder()

	h.ListCatalog(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Models []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"models"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if len(resp.Models) != 20 {
		t.Errorf("expected 20 models, got %d", len(resp.Models))
	}

	// Verify all expected types are present
	typeCounts := make(map[string]int)
	for _, m := range resp.Models {
		typeCounts[m.Type]++
	}

	expectedTypes := map[string]int{
		"llm":        5,
		"code":       8,
		"diffusion":  2,
		"multimodal": 3,
		"embedding":  2,
	}

	for typ, want := range expectedTypes {
		if got := typeCounts[typ]; got != want {
			t.Errorf("type %q: expected %d models, got %d", typ, want, got)
		}
	}
}

// --- Scenario: Catalog suggests DeepSeek 236B for very high VRAM ---
func TestListCatalog_SuggestsLargestModel(t *testing.T) {
	h := handlers.NewCatalogHandler()

	req := httptest.NewRequest(http.MethodGet, "/v1/models/catalog?vram_mb=200000", nil)
	rec := httptest.NewRecorder()

	h.ListCatalog(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Suggested *struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"suggested"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if resp.Suggested == nil {
		t.Fatal("expected a suggestion for 200GB VRAM")
	}

	if resp.Suggested.ID != "deepseek-ai/DeepSeek-Coder-V2-236B" {
		t.Errorf("expected DeepSeek Coder V2 236B suggestion, got %q", resp.Suggested.ID)
	}
}
