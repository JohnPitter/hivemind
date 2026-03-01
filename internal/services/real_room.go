package services

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/joaopedro/hivemind/gen/peerpb"
	"github.com/joaopedro/hivemind/gen/workerpb"
	"github.com/joaopedro/hivemind/internal/catalog"
	"github.com/joaopedro/hivemind/internal/infra"
	"github.com/joaopedro/hivemind/internal/logger"
	"github.com/joaopedro/hivemind/internal/models"
)

const realPendingTimeout = 5 * time.Minute

// RealRoomConfig holds the configuration needed by RealRoomService.
type RealRoomConfig struct {
	LocalPeerID   string
	Endpoint      string
	WireGuardPort int
	GRPCPort      int
}

// RealRoomService orchestrates room lifecycle with real P2P networking:
// Signaling → WireGuard → PeerRegistry → PeerGRPCServer.
type RealRoomService struct {
	mu           sync.RWMutex
	room         *models.Room
	startAt      time.Time
	pendingTimer *time.Timer

	sigClient    *infra.SignalingClient
	wgManager    *infra.WireGuardManager
	peerRegistry *infra.PeerRegistry
	peerServer   *infra.PeerGRPCServer
	resilience   *ResilienceService

	cfg RealRoomConfig
}

// NewRealRoomService creates a real room service wired to actual infrastructure.
func NewRealRoomService(
	cfg RealRoomConfig,
	sigClient *infra.SignalingClient,
	wgManager *infra.WireGuardManager,
	peerRegistry *infra.PeerRegistry,
) *RealRoomService {
	return &RealRoomService{
		cfg:          cfg,
		sigClient:    sigClient,
		wgManager:    wgManager,
		peerRegistry: peerRegistry,
	}
}

func (s *RealRoomService) Create(ctx context.Context, cfg models.RoomConfig) (*models.Room, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.room != nil {
		return nil, models.ErrAlreadyInRoom
	}

	// 1. Initialize WireGuard (host gets mesh IP .1)
	meshIP := infra.AllocateMeshIP(0)
	if err := s.wgManager.Initialize(s.cfg.WireGuardPort, meshIP); err != nil {
		return nil, fmt.Errorf("wireguard init: %w", err)
	}

	// 2. Generate room identifiers
	roomID := generateID(8)
	inviteCode := generateID(6)

	// 3. Register room on signaling server
	sigReq := infra.SignalingCreateRequest{
		RoomID:     roomID,
		InviteCode: inviteCode,
		ModelID:    cfg.ModelID,
		HostID:     s.cfg.LocalPeerID,
		MaxPeers:   cfg.MaxPeers,
		PublicKey:  s.wgManager.PublicKey(),
		Endpoint:   s.cfg.Endpoint,
	}
	if err := s.sigClient.CreateRoom(ctx, sigReq); err != nil {
		return nil, fmt.Errorf("signaling create: %w", err)
	}

	// 4. Create PeerGRPCServer with callbacks
	peerSrv, err := infra.NewPeerGRPCServer(infra.PeerGRPCServerConfig{
		LocalPeerID: s.cfg.LocalPeerID,
		LocalWorker: func() workerpb.WorkerServiceClient { return nil }, // set later by inference service
		RoomToken:   inviteCode,
		OnHandshake: func(peerID string, resources *workerpb.ResourceUsage) {
			s.addPeerToRoom(peerID, resources)
		},
		GetRoomState: func() *peerpb.RoomState {
			return s.getRoomStateProto()
		},
	})
	if err != nil {
		return nil, fmt.Errorf("peer server: %w", err)
	}
	s.peerServer = peerSrv

	// 5. Start gRPC server for peer connections
	if err := s.peerServer.Start(s.cfg.GRPCPort); err != nil {
		return nil, fmt.Errorf("peer server start: %w", err)
	}

	// 6. Write WireGuard config
	if _, err := s.wgManager.WriteConfig(); err != nil {
		logger.Warn("failed to write wireguard config", "error", err)
	}

	// 7. Bring up WireGuard mesh (Linux only)
	s.bringUpMesh()

	// 8. Determine initial state
	hostResources := models.ResourceSpec{
		GPUName:   "Unknown GPU",
		VRAMTotal: 0,
		VRAMFree:  0,
		RAMTotal:  0,
		RAMFree:   0,
		Platform:  runtime.GOOS,
	}
	if cfg.Resources != nil {
		hostResources = *cfg.Resources
	}

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
		HostID:      s.cfg.LocalPeerID,
		MaxPeers:    cfg.MaxPeers,
		TotalLayers: catalog.LayersForModel(cfg.ModelID),
		CreatedAt:   time.Now(),
		Peers: []models.Peer{
			{
				ID:        s.cfg.LocalPeerID,
				Name:      "you (host)",
				IP:        meshIP,
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
		s.pendingTimer = time.AfterFunc(realPendingTimeout, func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.room != nil && s.room.State == models.RoomStatePending {
				s.room.State = models.RoomStateClosed
				s.cleanup(context.Background())
			}
		})
	}

	// Start resilience service for active rooms
	if state == models.RoomStateActive {
		s.startResilience()
	}

	logger.Info("room created",
		"room_id", roomID,
		"invite", inviteCode,
		"model", cfg.ModelID,
		"state", state,
	)

	return s.room, nil
}

func (s *RealRoomService) Join(ctx context.Context, inviteCode string, resources models.ResourceSpec) (*models.Room, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.room != nil {
		return nil, models.ErrAlreadyInRoom
	}

	// 1. Initialize WireGuard first to get public key
	// Use a temporary mesh IP — will be overwritten by signaling response
	if err := s.wgManager.Initialize(s.cfg.WireGuardPort, "10.42.0.99/24"); err != nil {
		return nil, fmt.Errorf("wireguard init: %w", err)
	}

	// 2. Join via signaling server (with correct public key)
	joinResp, err := s.sigClient.JoinRoom(ctx, infra.SignalingJoinRequest{
		InviteCode: inviteCode,
		PeerID:     s.cfg.LocalPeerID,
		PublicKey:  s.wgManager.PublicKey(),
		Endpoint:   s.cfg.Endpoint,
	})
	if err != nil {
		return nil, fmt.Errorf("signaling join: %w", err)
	}

	// 3. Add existing peers to WireGuard config
	for _, p := range joinResp.Peers {
		if err := s.wgManager.AddPeer(p.PublicKey, p.Endpoint, p.MeshIP); err != nil {
			logger.Warn("failed to add peer to wireguard", "peer", p.ID, "error", err)
		}
	}

	// 4. Write WireGuard config and bring up mesh
	if _, err := s.wgManager.WriteConfig(); err != nil {
		logger.Warn("failed to write wireguard config", "error", err)
	}
	s.bringUpMesh()

	// 5. Register peers in peer registry and perform handshakes
	roomPeers := make([]models.Peer, 0, len(joinResp.Peers)+1)
	for _, p := range joinResp.Peers {
		// Extract IP from mesh IP (remove CIDR suffix)
		ip := p.MeshIP
		if idx := len(ip) - 3; idx > 0 && ip[idx] == '/' {
			ip = ip[:idx]
		}

		if err := s.peerRegistry.AddPeer(ctx, p.ID, ip, p.PublicKey); err != nil {
			logger.Warn("failed to connect to peer", "peer", p.ID, "error", err)
			continue
		}

		// Handshake with peer
		hsResp, err := s.peerRegistry.Handshake(ctx, p.ID, inviteCode, &workerpb.ResourceUsage{
			GpuName:     resources.GPUName,
			VramTotalMb: resources.VRAMTotal,
			VramUsedMb:  resources.VRAMTotal - resources.VRAMFree,
			RamTotalMb:  resources.RAMTotal,
			RamUsedMb:   resources.RAMTotal - resources.RAMFree,
		})
		if err != nil {
			logger.Warn("handshake failed", "peer", p.ID, "error", err)
		}
		_ = hsResp // handshake response used for state sync in future

		peerNode, _ := s.peerRegistry.GetPeer(p.ID)
		latency := float64(0)
		if peerNode != nil {
			latency = peerNode.Latency
		}

		roomPeers = append(roomPeers, models.Peer{
			ID:    p.ID,
			Name:  p.ID,
			IP:    ip,
			State: models.PeerStateReady,
			Resources: models.ResourceSpec{
				GPUName: "Remote GPU",
			},
			Latency:  latency,
			JoinedAt: p.JoinedAt,
			IsHost:   true, // existing peers include the host
		})
	}

	// Add ourselves
	roomPeers = append(roomPeers, models.Peer{
		ID:        s.cfg.LocalPeerID,
		Name:      "you",
		IP:        joinResp.MeshIP,
		State:     models.PeerStateReady,
		Resources: resources,
		JoinedAt:  time.Now(),
		IsHost:    false,
	})

	// 6. Build room
	s.startAt = time.Now()
	s.room = &models.Room{
		ID:          joinResp.RoomID,
		InviteCode:  inviteCode,
		ModelID:     joinResp.ModelID,
		ModelType:   models.ModelTypeLLM,
		State:       models.RoomStateActive,
		HostID:      "", // will be set by signaling/handshake context
		MaxPeers:    10,
		TotalLayers: catalog.LayersForModel(joinResp.ModelID),
		CreatedAt:   time.Now(),
		Peers:       roomPeers,
	}

	// Detect host from peers
	for _, p := range joinResp.Peers {
		// First peer is typically the host
		s.room.HostID = p.ID
		break
	}

	assignLayers(s.room)

	// Start resilience service
	s.startResilience()

	logger.Info("joined room",
		"room_id", joinResp.RoomID,
		"mesh_ip", joinResp.MeshIP,
		"peers", len(roomPeers),
	)

	return s.room, nil
}

func (s *RealRoomService) Leave(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.room == nil {
		return models.ErrNotInRoom
	}

	// Notify signaling server
	if err := s.sigClient.LeaveRoom(ctx, s.room.InviteCode, s.cfg.LocalPeerID); err != nil {
		logger.Warn("failed to notify signaling on leave", "error", err)
	}

	s.cleanup(ctx)
	return nil
}

func (s *RealRoomService) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.room == nil {
		return models.ErrNotInRoom
	}

	s.room.State = models.RoomStateClosed

	// Notify signaling server
	if err := s.sigClient.LeaveRoom(ctx, s.room.InviteCode, s.cfg.LocalPeerID); err != nil {
		logger.Warn("failed to notify signaling on stop", "error", err)
	}

	s.cleanup(ctx)
	return nil
}

func (s *RealRoomService) Status(_ context.Context) (*models.RoomStatus, error) {
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
		TokensPerSec: 0, // populated by inference service
		Uptime:       uptime,
	}, nil
}

func (s *RealRoomService) CurrentRoom() *models.Room {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.room
}

// addPeerToRoom is called when a remote peer completes handshake (host side).
func (s *RealRoomService) addPeerToRoom(peerID string, resources *workerpb.ResourceUsage) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.room == nil {
		return
	}

	// Check if peer already exists
	for _, p := range s.room.Peers {
		if p.ID == peerID {
			return
		}
	}

	res := models.ResourceSpec{}
	if resources != nil {
		res = models.ResourceSpec{
			GPUName:   resources.GpuName,
			VRAMTotal: resources.VramTotalMb,
			VRAMFree:  resources.VramTotalMb - resources.VramUsedMb,
			RAMTotal:  resources.RamTotalMb,
			RAMFree:   resources.RamTotalMb - resources.RamUsedMb,
		}
	}

	s.room.Peers = append(s.room.Peers, models.Peer{
		ID:        peerID,
		Name:      peerID,
		IP:        "",
		State:     models.PeerStateReady,
		Resources: res,
		JoinedAt:  time.Now(),
		IsHost:    false,
	})

	// Re-check if room can become active
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
			s.startResilience()
		}
	}

	assignLayers(s.room)

	// Register peer in resilience monitoring
	if s.resilience != nil {
		s.resilience.RegisterPeer(peerID)
	}

	logger.Info("peer added to room via handshake",
		"peer_id", peerID,
		"total_peers", len(s.room.Peers),
	)
}

// getRoomStateProto returns the current room state for the PeerGRPCServer.
func (s *RealRoomService) getRoomStateProto() *peerpb.RoomState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.room == nil {
		return nil
	}

	assignments := make([]*peerpb.PeerAssignment, 0, len(s.room.Peers))
	for _, p := range s.room.Peers {
		layers := make([]int32, len(p.Layers))
		for i, l := range p.Layers {
			layers[i] = int32(l)
		}
		assignments = append(assignments, &peerpb.PeerAssignment{
			PeerId: p.ID,
			PeerIp: p.IP,
			Layers: layers,
		})
	}

	return &peerpb.RoomState{
		ModelId:     s.room.ModelID,
		TotalLayers: int32(s.room.TotalLayers),
		Assignments: assignments,
	}
}

// cleanup tears down all networking resources. Must be called with mu held.
func (s *RealRoomService) cleanup(_ context.Context) {
	if s.pendingTimer != nil {
		s.pendingTimer.Stop()
		s.pendingTimer = nil
	}

	if s.resilience != nil {
		s.resilience.Stop()
		s.resilience = nil
	}

	if s.peerRegistry != nil {
		s.peerRegistry.Close()
	}

	if s.peerServer != nil {
		s.peerServer.Stop()
		s.peerServer = nil
	}

	// Bring down WireGuard mesh (Linux only)
	s.bringDownMesh()

	s.room = nil
}

// startResilience initializes health monitoring for all remote peers.
func (s *RealRoomService) startResilience() {
	if s.peerRegistry == nil {
		return
	}

	s.resilience = NewResilienceService(s, s.peerRegistry, s.cfg.LocalPeerID)
	s.resilience.Start()

	// Register all remote peers for monitoring
	for _, p := range s.room.Peers {
		if p.ID != s.cfg.LocalPeerID {
			s.resilience.RegisterPeer(p.ID)
		}
	}
}

// bringUpMesh attempts to bring up the WireGuard interface (Linux only).
func (s *RealRoomService) bringUpMesh() {
	if runtime.GOOS != "linux" {
		logger.Info("skipping wg-quick up (non-linux)", "os", runtime.GOOS)
		return
	}
	// In production Linux environments, this would call:
	// exec.Command("wg-quick", "up", "hm0")
	// Skipped for now — Docker networking handles routing between containers
	logger.Info("wireguard mesh config written (wg-quick up skipped in container)")
}

// bringDownMesh tears down the WireGuard interface (Linux only).
func (s *RealRoomService) bringDownMesh() {
	if runtime.GOOS != "linux" {
		return
	}
	logger.Info("wireguard mesh torn down (wg-quick down skipped in container)")
}
