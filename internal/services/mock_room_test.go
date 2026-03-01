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
	// 70B requires 40960MB but default host has ~9728MB usable → pending
	if room.State != models.RoomStatePending {
		t.Errorf("state = %q, want %q (host VRAM insufficient for 70B)", room.State, models.RoomStatePending)
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

func TestMockRoomService_Create_SufficientResources(t *testing.T) {
	svc := NewMockRoomService()

	room, err := svc.Create(context.Background(), models.RoomConfig{
		ModelID:   "TinyLlama/TinyLlama-1.1B",
		ModelType: models.ModelTypeLLM,
		MaxPeers:  3,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// TinyLlama requires 2048MB, default host has ~9728MB → active
	if room.State != models.RoomStateActive {
		t.Errorf("state = %q, want %q (host VRAM sufficient for TinyLlama)", room.State, models.RoomStateActive)
	}
}

func TestMockRoomService_MultiRoom(t *testing.T) {
	svc := NewMockRoomService()

	room1, err := svc.Create(context.Background(), models.RoomConfig{
		ModelID:   "TinyLlama/TinyLlama-1.1B",
		ModelType: models.ModelTypeLLM,
		MaxPeers:  3,
	})
	if err != nil {
		t.Fatalf("create room 1: %v", err)
	}

	room2, err := svc.Create(context.Background(), models.RoomConfig{
		ModelID:   "meta-llama/Llama-3-70B",
		ModelType: models.ModelTypeLLM,
		MaxPeers:  5,
	})
	if err != nil {
		t.Fatalf("create room 2: %v", err)
	}

	if room1.ID == room2.ID {
		t.Error("rooms should have different IDs")
	}

	rooms := svc.ListRooms()
	if len(rooms) != 2 {
		t.Errorf("ListRooms = %d, want 2", len(rooms))
	}

	got1 := svc.GetRoom(room1.ID)
	if got1 == nil || got1.ID != room1.ID {
		t.Error("GetRoom should return room 1")
	}

	got2 := svc.GetRoom(room2.ID)
	if got2 == nil || got2.ID != room2.ID {
		t.Error("GetRoom should return room 2")
	}
}

func TestMockRoomService_Leave(t *testing.T) {
	svc := NewMockRoomService()

	room, _ := svc.Create(context.Background(), models.RoomConfig{
		ModelID: "TinyLlama/TinyLlama-1.1B",
	})

	if err := svc.Leave(context.Background(), room.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if svc.GetRoom(room.ID) != nil {
		t.Error("room should be nil after leave")
	}
}

func TestMockRoomService_LeaveWithoutRoom(t *testing.T) {
	svc := NewMockRoomService()

	err := svc.Leave(context.Background(), "nonexistent")
	if err != models.ErrNotInRoom {
		t.Errorf("error = %v, want ErrNotInRoom", err)
	}
}

func TestMockRoomService_Status(t *testing.T) {
	svc := NewMockRoomService()

	room, _ := svc.Create(context.Background(), models.RoomConfig{
		ModelID:   "meta-llama/Llama-3-70B",
		ModelType: models.ModelTypeLLM,
		MaxPeers:  5,
	})

	status, err := svc.Status(context.Background(), room.ID)
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

func TestMockRoomService_ActiveRoomID(t *testing.T) {
	svc := NewMockRoomService()

	if id := svc.ActiveRoomID(); id != "" {
		t.Errorf("ActiveRoomID = %q, want empty", id)
	}

	room, _ := svc.Create(context.Background(), models.RoomConfig{
		ModelID:   "TinyLlama/TinyLlama-1.1B",
		ModelType: models.ModelTypeLLM,
		MaxPeers:  3,
	})

	if id := svc.ActiveRoomID(); id != room.ID {
		t.Errorf("ActiveRoomID = %q, want %q", id, room.ID)
	}
}
