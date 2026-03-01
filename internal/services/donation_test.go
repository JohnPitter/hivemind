package services

import (
	"context"
	"testing"

	"github.com/joaopedro/hivemind/internal/catalog"
	"github.com/joaopedro/hivemind/internal/models"
)

// --- Scenario 1: User joins with 50% donation ---
// A user with a RTX 3060 (10GB free VRAM, 24GB free RAM)
// joins a room donating 50% of resources.
func TestUserJoins_50PercentDonation(t *testing.T) {
	svc := NewMockRoomService()

	resources := models.ResourceSpec{
		GPUName:     "NVIDIA RTX 3060",
		VRAMTotal:   12288,
		VRAMFree:    10240,
		RAMTotal:    32768,
		RAMFree:     24576,
		CUDAAvail:   true,
		Platform:    "Windows",
		DonationPct: 50,
	}

	room, err := svc.Join(context.Background(), "abc123", resources)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
		t.Fatal("self peer not found in room")
	}

	// Verify donation percentage was preserved
	if self.Resources.DonationPct != 50 {
		t.Errorf("DonationPct = %d, want 50", self.Resources.DonationPct)
	}

	// VRAM: (10240 - 512) * 50/100 = 4864 MB
	expectedVRAM := int64(4864)
	if got := self.Resources.TotalUsableVRAM(); got != expectedVRAM {
		t.Errorf("TotalUsableVRAM() = %d, want %d", got, expectedVRAM)
	}

	// RAM: (24576 - 1024) * 50/100 = 11776 MB
	expectedRAM := int64(11776)
	if got := self.Resources.TotalUsableRAM(); got != expectedRAM {
		t.Errorf("TotalUsableRAM() = %d, want %d", got, expectedRAM)
	}

	// Verify layers were assigned (should be fewer than full donation)
	if len(self.Layers) == 0 {
		t.Error("expected self to have layers assigned")
	}
}

// --- Scenario 2: User joins with 100% donation (all-in) ---
func TestUserJoins_100PercentDonation(t *testing.T) {
	svc := NewMockRoomService()

	resources := models.ResourceSpec{
		GPUName:     "NVIDIA RTX 4090",
		VRAMTotal:   24576,
		VRAMFree:    22528,
		RAMTotal:    65536,
		RAMFree:     57344,
		CUDAAvail:   true,
		Platform:    "Linux",
		DonationPct: 100,
	}

	room, err := svc.Join(context.Background(), "xyz789", resources)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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

	// VRAM: (22528 - 512) * 100/100 = 22016 MB
	expectedVRAM := int64(22016)
	if got := self.Resources.TotalUsableVRAM(); got != expectedVRAM {
		t.Errorf("TotalUsableVRAM() = %d, want %d", got, expectedVRAM)
	}

	// RAM: (57344 - 1024) * 100/100 = 56320 MB
	expectedRAM := int64(56320)
	if got := self.Resources.TotalUsableRAM(); got != expectedRAM {
		t.Errorf("TotalUsableRAM() = %d, want %d", got, expectedRAM)
	}
}

// --- Scenario 3: User joins with 25% donation (light) ---
func TestUserJoins_25PercentDonation(t *testing.T) {
	svc := NewMockRoomService()

	resources := models.ResourceSpec{
		GPUName:     "NVIDIA RTX 3080",
		VRAMTotal:   10240,
		VRAMFree:    8192,
		RAMTotal:    32768,
		RAMFree:     28672,
		CUDAAvail:   true,
		Platform:    "Windows",
		DonationPct: 25,
	}

	room, err := svc.Join(context.Background(), "light123", resources)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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

	// VRAM: (8192 - 512) * 25/100 = 1920 MB
	expectedVRAM := int64(1920)
	if got := self.Resources.TotalUsableVRAM(); got != expectedVRAM {
		t.Errorf("TotalUsableVRAM() = %d, want %d", got, expectedVRAM)
	}

	// RAM: (28672 - 1024) * 25/100 = 6912 MB
	expectedRAM := int64(6912)
	if got := self.Resources.TotalUsableRAM(); got != expectedRAM {
		t.Errorf("TotalUsableRAM() = %d, want %d", got, expectedRAM)
	}

	// With only 1920 MB usable, self should get fewer layers than other peers
	if len(self.Layers) == 0 {
		t.Error("expected self to have at least some layers")
	}
}

// --- Scenario 4: Backwards compatibility — DonationPct=0 defaults to 100% ---
func TestUserJoins_ZeroDonation_DefaultsTo100(t *testing.T) {
	resources := models.ResourceSpec{
		GPUName:   "NVIDIA RTX 3060",
		VRAMTotal: 12288,
		VRAMFree:  10240,
		RAMTotal:  32768,
		RAMFree:   24576,
		CUDAAvail: true,
		Platform:  "Windows",
		// DonationPct is 0 (zero value)
	}

	// Should behave exactly like 100%
	expectedVRAM := int64(10240 - 512) // 9728
	if got := resources.TotalUsableVRAM(); got != expectedVRAM {
		t.Errorf("TotalUsableVRAM() with 0 donation = %d, want %d (100%% default)", got, expectedVRAM)
	}

	expectedRAM := int64(24576 - 1024) // 23552
	if got := resources.TotalUsableRAM(); got != expectedRAM {
		t.Errorf("TotalUsableRAM() with 0 donation = %d, want %d (100%% default)", got, expectedRAM)
	}
}

// --- Scenario 5: Donation affects layer distribution proportionally ---
// Two peers with same hardware but different donation percentages
// should get proportional layers.
func TestLayerDistribution_DifferentDonations(t *testing.T) {
	room := &models.Room{
		TotalLayers: 32,
		Peers: []models.Peer{
			{
				ID: "generous",
				Resources: models.ResourceSpec{
					VRAMTotal:   12288,
					VRAMFree:    10240,
					DonationPct: 100,
				},
			},
			{
				ID: "light",
				Resources: models.ResourceSpec{
					VRAMTotal:   12288,
					VRAMFree:    10240,
					DonationPct: 25,
				},
			},
		},
	}

	assignLayers(room)

	generousLayers := len(room.Peers[0].Layers)
	lightLayers := len(room.Peers[1].Layers)
	total := generousLayers + lightLayers

	if total != 32 {
		t.Errorf("total layers = %d, want 32", total)
	}

	// Generous peer donates 100% → (10240-512)*100/100 = 9728
	// Light peer donates 25% → (10240-512)*25/100 = 2432
	// Generous should get ~80% of layers (9728 / (9728+2432) ≈ 0.8)
	if generousLayers <= lightLayers {
		t.Errorf("generous peer (%d layers) should have more than light peer (%d layers)",
			generousLayers, lightLayers)
	}

	// Generous should have roughly 4x the layers of light
	if generousLayers < lightLayers*3 {
		t.Errorf("generous peer (%d layers) should have roughly 4x light peer (%d layers)",
			generousLayers, lightLayers)
	}
}

// --- Scenario 6: Host creates room with donation-aware resources ---
func TestHostCreates_WithDonationPct(t *testing.T) {
	svc := NewMockRoomService()

	room, err := svc.Create(context.Background(), models.RoomConfig{
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
			DonationPct: 75,
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With 75% donation: (10240-512)*75/100 = 7296 MB
	// TinyLlama needs 2048 MB → should be active
	if room.State != models.RoomStateActive {
		t.Errorf("state = %q, want active (7296 MB usable > 2048 MB required)", room.State)
	}

	hostVRAM := room.Peers[0].Resources.TotalUsableVRAM()
	if hostVRAM != 7296 {
		t.Errorf("host usable VRAM = %d, want 7296", hostVRAM)
	}
}

// --- Scenario 7: Host creates room with low donation → pending ---
func TestHostCreates_LowDonation_GoesPending(t *testing.T) {
	svc := NewMockRoomService()

	// Gemma 2 27B needs 18432 MB
	// With 25% donation: (10240-512)*25/100 = 2432 MB → pending
	room, err := svc.Create(context.Background(), models.RoomConfig{
		ModelID:   "google/gemma-2-27b",
		ModelType: models.ModelTypeLLM,
		MaxPeers:  5,
		Resources: &models.ResourceSpec{
			GPUName:     "NVIDIA RTX 3060",
			VRAMTotal:   12288,
			VRAMFree:    10240,
			CUDAAvail:   true,
			Platform:    "Linux",
			DonationPct: 25,
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if room.State != models.RoomStatePending {
		t.Errorf("state = %q, want pending (2432 MB usable < 18432 MB required)", room.State)
	}
}

// --- Scenario 8: Edge case — VRAM too low even before donation ---
func TestDonation_VRAMBelowReserve(t *testing.T) {
	resources := models.ResourceSpec{
		VRAMFree:    400, // Below 512 MB reserve
		RAMFree:     800, // Below 1024 MB reserve
		DonationPct: 100,
	}

	if got := resources.TotalUsableVRAM(); got != 0 {
		t.Errorf("TotalUsableVRAM() = %d, want 0 (below reserve)", got)
	}
	if got := resources.TotalUsableRAM(); got != 0 {
		t.Errorf("TotalUsableRAM() = %d, want 0 (below reserve)", got)
	}
}

// --- Scenario 9: User creates with new model types (code, multimodal) ---
func TestHostCreates_CodeModel(t *testing.T) {
	svc := NewMockRoomService()

	room, err := svc.Create(context.Background(), models.RoomConfig{
		ModelID:   "Qwen/Qwen2.5-Coder-7B",
		ModelType: models.ModelTypeCode,
		MaxPeers:  3,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Default host has ~9728 MB. Qwen2.5-Coder 7B needs 5120 MB → active
	if room.State != models.RoomStateActive {
		t.Errorf("state = %q, want active for Qwen2.5-Coder 7B", room.State)
	}

	if room.ModelType != models.ModelTypeCode {
		t.Errorf("model type = %q, want 'code'", room.ModelType)
	}

	if room.TotalLayers != 32 {
		t.Errorf("total layers = %d, want 32 for Qwen2.5-Coder 7B", room.TotalLayers)
	}
}

func TestHostCreates_EmbeddingModel(t *testing.T) {
	svc := NewMockRoomService()

	room, err := svc.Create(context.Background(), models.RoomConfig{
		ModelID:   "nomic-ai/nomic-embed-text-v1.5",
		ModelType: models.ModelTypeEmbedding,
		MaxPeers:  3,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Nomic Embed needs 512 MB, host has ~9728 MB → active
	if room.State != models.RoomStateActive {
		t.Errorf("state = %q, want active for Nomic Embed", room.State)
	}

	if room.TotalLayers != 12 {
		t.Errorf("total layers = %d, want 12 for Nomic Embed", room.TotalLayers)
	}
}

func TestHostCreates_MultimodalModel_Pending(t *testing.T) {
	svc := NewMockRoomService()

	// Qwen2-VL 72B needs 45056 MB — default host has ~9728 MB → pending
	room, err := svc.Create(context.Background(), models.RoomConfig{
		ModelID:   "Qwen/Qwen2-VL-72B",
		ModelType: models.ModelTypeMultimodal,
		MaxPeers:  8,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if room.State != models.RoomStatePending {
		t.Errorf("state = %q, want pending for 72B multimodal on RTX 3060", room.State)
	}
}

// --- Scenario 10: Catalog suggests correct model for each resource tier ---
func TestCatalogSuggestion_ByDonationLevel(t *testing.T) {
	base := models.ResourceSpec{
		VRAMFree: 10240,
	}

	tests := []struct {
		donationPct int
		wantModelID string
		desc        string
	}{
		{100, "stabilityai/stable-diffusion-xl", "100%: 9728 MB → SDXL (8192)"},
		{75, "meta-llama/Llama-3-8B", "75%: 7296 MB → Llama 3 8B (6144)"},
		{50, "TinyLlama/TinyLlama-1.1B", "50%: 4864 MB → TinyLlama (5120 > 4864, next fit is 2048)"},
		{25, "TinyLlama/TinyLlama-1.1B", "25%: 2432 MB → TinyLlama (2048)"},
	}

	for _, tt := range tests {
		base.DonationPct = tt.donationPct
		usable := base.TotalUsableVRAM()
		suggested := catalog.SuggestLargestFitting(usable)

		if suggested == nil {
			t.Errorf("%s: got nil suggestion for %d MB usable", tt.desc, usable)
			continue
		}
		if suggested.ID != tt.wantModelID {
			t.Errorf("%s: usable=%d MB, got %s, want %s",
				tt.desc, usable, suggested.ID, tt.wantModelID)
		}
	}
}
