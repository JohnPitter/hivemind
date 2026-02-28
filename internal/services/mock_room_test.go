package services

import (
	"context"
	"testing"

	"github.com/joaopedro/hivemind/internal/models"
)

func TestMockRoomService_Create(t *testing.T) {
	svc := NewMockRoomService()

	room, err := svc.Create(context.Background(), models.RoomConfig{
		ModelID:   "meta-llama/Llama-3-70B",
		ModelType: models.ModelTypeLLM,
		MaxPeers:  5,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if room.ID == "" {
		t.Error("room ID should not be empty")
	}
	if room.InviteCode == "" {
		t.Error("invite code should not be empty")
	}
	if room.ModelID != "meta-llama/Llama-3-70B" {
		t.Errorf("model ID = %q, want %q", room.ModelID, "meta-llama/Llama-3-70B")
	}
	if room.State != models.RoomStateActive {
		t.Errorf("state = %q, want %q", room.State, models.RoomStateActive)
	}
	if len(room.Peers) != 1 {
		t.Errorf("peers = %d, want 1", len(room.Peers))
	}
	if !room.Peers[0].IsHost {
		t.Error("first peer should be host")
	}
	if room.TotalLayers != 80 {
		t.Errorf("total layers = %d, want 80 for 70B model", room.TotalLayers)
	}
	if len(room.Peers[0].Layers) == 0 {
		t.Error("host should have layers assigned")
	}
}

func TestMockRoomService_CreateDuplicate(t *testing.T) {
	svc := NewMockRoomService()

	_, _ = svc.Create(context.Background(), models.RoomConfig{
		ModelID:   "test/model-7b",
		ModelType: models.ModelTypeLLM,
		MaxPeers:  3,
	})

	_, err := svc.Create(context.Background(), models.RoomConfig{
		ModelID:   "test/model-7b",
		ModelType: models.ModelTypeLLM,
		MaxPeers:  3,
	})

	if err != models.ErrAlreadyInRoom {
		t.Errorf("error = %v, want ErrAlreadyInRoom", err)
	}
}

func TestMockRoomService_Leave(t *testing.T) {
	svc := NewMockRoomService()

	_, _ = svc.Create(context.Background(), models.RoomConfig{
		ModelID: "test/model-7b",
	})

	if err := svc.Leave(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if svc.CurrentRoom() != nil {
		t.Error("room should be nil after leave")
	}
}

func TestMockRoomService_LeaveWithoutRoom(t *testing.T) {
	svc := NewMockRoomService()

	err := svc.Leave(context.Background())
	if err != models.ErrNotInRoom {
		t.Errorf("error = %v, want ErrNotInRoom", err)
	}
}

func TestMockRoomService_Status(t *testing.T) {
	svc := NewMockRoomService()

	_, _ = svc.Create(context.Background(), models.RoomConfig{
		ModelID:   "meta-llama/Llama-3-70B",
		ModelType: models.ModelTypeLLM,
		MaxPeers:  5,
	})

	status, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.TotalVRAM == 0 {
		t.Error("total VRAM should not be 0")
	}
	if status.Uptime == "" {
		t.Error("uptime should not be empty")
	}
}

func TestLayerAssignment(t *testing.T) {
	room := &models.Room{
		TotalLayers: 32,
		Peers: []models.Peer{
			{
				ID: "a",
				Resources: models.ResourceSpec{
					VRAMTotal: 8192,
					VRAMFree:  6144,
				},
			},
			{
				ID: "b",
				Resources: models.ResourceSpec{
					VRAMTotal: 16384,
					VRAMFree:  14336,
				},
			},
		},
	}

	assignLayers(room)

	totalAssigned := len(room.Peers[0].Layers) + len(room.Peers[1].Layers)
	if totalAssigned != 32 {
		t.Errorf("total assigned layers = %d, want 32", totalAssigned)
	}

	// Peer B has more VRAM, should have more layers
	if len(room.Peers[1].Layers) <= len(room.Peers[0].Layers) {
		t.Errorf("peer B (%d layers) should have more than peer A (%d layers)",
			len(room.Peers[1].Layers), len(room.Peers[0].Layers))
	}
}
