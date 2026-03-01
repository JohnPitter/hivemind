package services

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/joaopedro/hivemind/internal/catalog"
	"github.com/joaopedro/hivemind/internal/models"
)

const pendingTimeout = 5 * time.Minute

// mockRoomEntry holds per-room state in the mock service.
type mockRoomEntry struct {
	room         *models.Room
	startAt      time.Time
	pendingTimer *time.Timer
}

// MockRoomService provides realistic mock data for CLI/Web development.
type MockRoomService struct {
	mu    sync.RWMutex
	rooms map[string]*mockRoomEntry
}

// NewMockRoomService creates a mock room service.
func NewMockRoomService() *MockRoomService {
	return &MockRoomService{
		rooms: make(map[string]*mockRoomEntry),
	}
}

func (s *MockRoomService) Create(_ context.Context, cfg models.RoomConfig) (*models.Room, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	roomID := generateID(8)
	inviteCode := generateID(6)

	hostResources := models.ResourceSpec{
		GPUName:   "NVIDIA RTX 3060",
		VRAMTotal: 12288,
		VRAMFree:  10240,
		RAMTotal:  32768,
		RAMFree:   24576,
		CUDAAvail: true,
		Platform:  "Windows",
	}

	// Use resources from config if provided
	if cfg.Resources != nil {
		hostResources = *cfg.Resources
	}

	// Determine initial state based on resource check
	state := models.RoomStateActive
	hostVRAM := hostResources.TotalUsableVRAM()
	modelReqs := catalog.Lookup(cfg.ModelID)

	if modelReqs != nil && hostVRAM < modelReqs.MinVRAMMB {
		state = models.RoomStatePending
	}

	room := &models.Room{
		ID:          roomID,
		InviteCode:  inviteCode,
		ModelID:     cfg.ModelID,
		ModelType:   cfg.ModelType,
		State:       state,
		HostID:      "self",
		MaxPeers:    cfg.MaxPeers,
		TotalLayers: catalog.LayersForModel(cfg.ModelID),
		CreatedAt:   time.Now(),
		Peers: []models.Peer{
			{
				ID:        "self",
				Name:      "you (host)",
				IP:        "10.0.0.1",
				State:     models.PeerStateReady,
				Resources: hostResources,
				JoinedAt:  time.Now(),
				IsHost:    true,
			},
		},
	}

	assignLayers(room)

	entry := &mockRoomEntry{
		room:    room,
		startAt: time.Now(),
	}

	// Start pending timer if resources insufficient
	if state == models.RoomStatePending {
		entry.pendingTimer = time.AfterFunc(pendingTimeout, func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if e, ok := s.rooms[roomID]; ok && e.room.State == models.RoomStatePending {
				e.room.State = models.RoomStateClosed
				delete(s.rooms, roomID)
			}
		})
	}

	s.rooms[roomID] = entry

	return room, nil
}

func (s *MockRoomService) Join(_ context.Context, inviteCode string, resources models.ResourceSpec) (*models.Room, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Use model from env var if set (real inference mode), otherwise default
	modelID := os.Getenv("HIVEMIND_MODEL_ID")
	if modelID == "" {
		modelID = "meta-llama/Llama-3-70B"
	}

	roomID := generateID(8)

	room := &models.Room{
		ID:          roomID,
		InviteCode:  inviteCode,
		ModelID:     modelID,
		ModelType:   models.ModelTypeLLM,
		State:       models.RoomStateActive,
		HostID:      "host-abc",
		MaxPeers:    5,
		TotalLayers: catalog.LayersForModel(modelID),
		CreatedAt:   time.Now().Add(-10 * time.Minute),
		Peers: []models.Peer{
			{
				ID:    "host-abc",
				Name:  "host",
				IP:    "10.0.0.1",
				State: models.PeerStateReady,
				Resources: models.ResourceSpec{
					GPUName:   "NVIDIA RTX 4090",
					VRAMTotal: 24576,
					VRAMFree:  22528,
					CUDAAvail: true,
					Platform:  "Linux",
				},
				Latency:  45.2,
				JoinedAt: time.Now().Add(-10 * time.Minute),
				IsHost:   true,
			},
			{
				ID:    "peer-def",
				Name:  "peer-1",
				IP:    "10.0.0.2",
				State: models.PeerStateReady,
				Resources: models.ResourceSpec{
					GPUName:   "NVIDIA RTX 3080",
					VRAMTotal: 10240,
					VRAMFree:  8192,
					CUDAAvail: true,
					Platform:  "Windows",
				},
				Latency:  72.8,
				JoinedAt: time.Now().Add(-5 * time.Minute),
				IsHost:   false,
			},
			{
				ID:        "self",
				Name:      "you",
				IP:        "10.0.0.3",
				State:     models.PeerStateReady,
				Resources: resources,
				JoinedAt:  time.Now(),
				IsHost:    false,
			},
		},
	}

	// Check if combined VRAM is sufficient
	var totalVRAM int64
	for _, p := range room.Peers {
		totalVRAM += p.Resources.TotalUsableVRAM()
	}
	modelReqs := catalog.Lookup(room.ModelID)
	if modelReqs != nil && totalVRAM < modelReqs.MinVRAMMB {
		room.State = models.RoomStatePending
	}

	assignLayers(room)

	s.rooms[roomID] = &mockRoomEntry{
		room:    room,
		startAt: time.Now(),
	}

	return room, nil
}

func (s *MockRoomService) Leave(_ context.Context, roomID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.rooms[roomID]
	if !ok {
		return models.ErrNotInRoom
	}

	if entry.pendingTimer != nil {
		entry.pendingTimer.Stop()
	}
	delete(s.rooms, roomID)
	return nil
}

func (s *MockRoomService) Stop(_ context.Context, roomID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.rooms[roomID]
	if !ok {
		return models.ErrNotInRoom
	}

	if entry.pendingTimer != nil {
		entry.pendingTimer.Stop()
	}
	entry.room.State = models.RoomStateClosed
	delete(s.rooms, roomID)
	return nil
}

func (s *MockRoomService) Status(_ context.Context, roomID string) (*models.RoomStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.rooms[roomID]
	if !ok {
		return nil, models.ErrNotInRoom
	}

	var totalVRAM, usedVRAM int64
	for _, p := range entry.room.Peers {
		totalVRAM += p.Resources.VRAMTotal
		usedVRAM += p.Resources.VRAMTotal - p.Resources.VRAMFree
	}

	uptime := time.Since(entry.startAt).Round(time.Second).String()

	return &models.RoomStatus{
		Room:         *entry.room,
		TotalVRAM:    totalVRAM,
		UsedVRAM:     usedVRAM,
		TokensPerSec: 12.4,
		Uptime:       uptime,
	}, nil
}

func (s *MockRoomService) CurrentRoom() *models.Room {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, entry := range s.rooms {
		if entry.room.State == models.RoomStateActive || entry.room.State == models.RoomStatePending {
			return entry.room
		}
	}
	return nil
}

func (s *MockRoomService) GetRoom(roomID string) *models.Room {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if entry, ok := s.rooms[roomID]; ok {
		return entry.room
	}
	return nil
}

func (s *MockRoomService) ListRooms() []*models.Room {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rooms := make([]*models.Room, 0, len(s.rooms))
	for _, entry := range s.rooms {
		rooms = append(rooms, entry.room)
	}
	return rooms
}

func (s *MockRoomService) ActiveRoomID() string {
	room := s.CurrentRoom()
	if room != nil {
		return room.ID
	}
	return ""
}

func containsInsensitive(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
