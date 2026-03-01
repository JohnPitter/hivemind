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

// MockRoomService provides realistic mock data for CLI/Web development.
type MockRoomService struct {
	mu           sync.RWMutex
	room         *models.Room
	startAt      time.Time
	pendingTimer *time.Timer
}

// NewMockRoomService creates a mock room service.
func NewMockRoomService() *MockRoomService {
	return &MockRoomService{}
}

func (s *MockRoomService) Create(_ context.Context, cfg models.RoomConfig) (*models.Room, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.room != nil {
		return nil, models.ErrAlreadyInRoom
	}

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

	s.startAt = time.Now()
	s.room = &models.Room{
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

	assignLayers(s.room)

	// Start pending timer if resources insufficient
	if state == models.RoomStatePending {
		s.pendingTimer = time.AfterFunc(pendingTimeout, func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.room != nil && s.room.State == models.RoomStatePending {
				s.room.State = models.RoomStateClosed
				s.room = nil
			}
		})
	}

	return s.room, nil
}

func (s *MockRoomService) Join(_ context.Context, inviteCode string, resources models.ResourceSpec) (*models.Room, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.room != nil {
		return nil, models.ErrAlreadyInRoom
	}

	// Use model from env var if set (real inference mode), otherwise default
	modelID := os.Getenv("HIVEMIND_MODEL_ID")
	if modelID == "" {
		modelID = "meta-llama/Llama-3-70B"
	}

	s.startAt = time.Now()
	s.room = &models.Room{
		ID:          generateID(8),
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

	// If room was pending, check if combined VRAM is now sufficient
	if s.room.State == models.RoomStatePending {
		var totalVRAM int64
		for _, p := range s.room.Peers {
			totalVRAM += p.Resources.TotalUsableVRAM()
		}
		modelReqs := catalog.Lookup(s.room.ModelID)
		if modelReqs != nil && totalVRAM >= modelReqs.MinVRAMMB {
			s.room.State = models.RoomStateActive
			if s.pendingTimer != nil {
				s.pendingTimer.Stop()
				s.pendingTimer = nil
			}
		}
	}

	assignLayers(s.room)

	return s.room, nil
}

func (s *MockRoomService) Leave(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.room == nil {
		return models.ErrNotInRoom
	}

	s.stopPendingTimer()
	s.room = nil
	return nil
}

func (s *MockRoomService) Stop(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.room == nil {
		return models.ErrNotInRoom
	}

	s.stopPendingTimer()
	s.room.State = models.RoomStateClosed
	s.room = nil
	return nil
}

func (s *MockRoomService) Status(_ context.Context) (*models.RoomStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.room == nil {
		return nil, models.ErrNotInRoom
	}

	var totalVRAM, usedVRAM int64
	for _, p := range s.room.Peers {
		totalVRAM += p.Resources.VRAMTotal
		usedVRAM += p.Resources.VRAMTotal - p.Resources.VRAMFree
	}

	uptime := time.Since(s.startAt).Round(time.Second).String()

	return &models.RoomStatus{
		Room:         *s.room,
		TotalVRAM:    totalVRAM,
		UsedVRAM:     usedVRAM,
		TokensPerSec: 12.4,
		Uptime:       uptime,
	}, nil
}

func (s *MockRoomService) CurrentRoom() *models.Room {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.room
}

// stopPendingTimer cancels the pending timer if active. Must be called with mu held.
func (s *MockRoomService) stopPendingTimer() {
	if s.pendingTimer != nil {
		s.pendingTimer.Stop()
		s.pendingTimer = nil
	}
}

func containsInsensitive(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
