package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/joaopedro/hivemind/internal/models"
)

// MockRoomService provides realistic mock data for CLI/Web development.
type MockRoomService struct {
	mu      sync.RWMutex
	room    *models.Room
	startAt time.Time
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

	s.startAt = time.Now()
	s.room = &models.Room{
		ID:          roomID,
		InviteCode:  inviteCode,
		ModelID:     cfg.ModelID,
		ModelType:   cfg.ModelType,
		State:       models.RoomStateActive,
		HostID:      "self",
		MaxPeers:    cfg.MaxPeers,
		TotalLayers: layersForModel(cfg.ModelID),
		CreatedAt:   time.Now(),
		Peers: []models.Peer{
			{
				ID:    "self",
				Name:  "you (host)",
				IP:    "10.0.0.1",
				State: models.PeerStateReady,
				Resources: models.ResourceSpec{
					GPUName:   "NVIDIA RTX 3060",
					VRAMTotal: 12288,
					VRAMFree:  10240,
					RAMTotal:  32768,
					RAMFree:   24576,
					CUDAAvail: true,
					Platform:  "Windows",
				},
				JoinedAt: time.Now(),
				IsHost:   true,
			},
		},
	}

	assignLayers(s.room)

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
		TotalLayers: layersForModel(modelID),
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

	assignLayers(s.room)

	return s.room, nil
}

func (s *MockRoomService) Leave(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.room == nil {
		return models.ErrNotInRoom
	}

	s.room = nil
	return nil
}

func (s *MockRoomService) Stop(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.room == nil {
		return models.ErrNotInRoom
	}

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

// generateID creates a random hex string of n bytes.
func generateID(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// layersForModel returns a realistic layer count based on model name.
func layersForModel(modelID string) int {
	switch {
	case containsInsensitive(modelID, "70b"):
		return 80
	case containsInsensitive(modelID, "13b"):
		return 40
	case containsInsensitive(modelID, "7b"):
		return 32
	case containsInsensitive(modelID, "3b"):
		return 26
	case containsInsensitive(modelID, "1.1b"), containsInsensitive(modelID, "tinyllama"):
		return 22
	default:
		return 32
	}
}

// assignLayers distributes model layers across peers by available VRAM.
func assignLayers(room *models.Room) {
	if len(room.Peers) == 0 || room.TotalLayers == 0 {
		return
	}

	var totalVRAM int64
	for _, p := range room.Peers {
		totalVRAM += p.Resources.TotalUsableVRAM()
	}

	if totalVRAM == 0 {
		// Equal distribution fallback
		perPeer := room.TotalLayers / len(room.Peers)
		offset := 0
		for i := range room.Peers {
			count := perPeer
			if i == len(room.Peers)-1 {
				count = room.TotalLayers - offset
			}
			room.Peers[i].Layers = makeRange(offset, offset+count)
			offset += count
		}
		return
	}

	// Proportional distribution by VRAM
	offset := 0
	for i := range room.Peers {
		peerVRAM := room.Peers[i].Resources.TotalUsableVRAM()
		proportion := float64(peerVRAM) / float64(totalVRAM)
		count := int(proportion * float64(room.TotalLayers))

		if count < 1 {
			count = 1
		}
		if i == len(room.Peers)-1 {
			count = room.TotalLayers - offset
		}
		if offset+count > room.TotalLayers {
			count = room.TotalLayers - offset
		}

		room.Peers[i].Layers = makeRange(offset, offset+count)
		offset += count
	}
}

func makeRange(start, end int) []int {
	r := make([]int, 0, end-start)
	for i := start; i < end; i++ {
		r = append(r, i)
	}
	return r
}

func containsInsensitive(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
