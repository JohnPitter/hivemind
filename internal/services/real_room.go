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

// roomContext holds per-room networking state.
type roomContext struct {
	room         *models.Room
	startAt      time.Time
	pendingTimer *time.Timer
	peerRegistry *infra.PeerRegistry
	peerServer   *infra.PeerGRPCServer
	resilience   *ResilienceService
}

// RealRoomService orchestrates room lifecycle with real P2P networking:
// Signaling → WireGuard → PeerRegistry → PeerGRPCServer.
// Supports multiple concurrent rooms.
type RealRoomService struct {
	mu    sync.RWMutex
	rooms map[string]*roomContext

	sigClient    *infra.SignalingClient
	wgManager    *infra.WireGuardManager
	peerRegistry *infra.PeerRegistry
	natTraversal *infra.NATTraversal

	cfg RealRoomConfig
}

// NewRealRoomService creates a real room service wired to actual infrastructure.
// natTraversal is optional — pass nil to disable STUN discovery.
func NewRealRoomService(
	cfg RealRoomConfig,
	sigClient *infra.SignalingClient,
	wgManager *infra.WireGuardManager,
	peerRegistry *infra.PeerRegistry,
	natTraversal *infra.NATTraversal,
) *RealRoomService {
	return &RealRoomService{
		rooms:        make(map[string]*roomContext),
		cfg:          cfg,
		sigClient:    sigClient,
		wgManager:    wgManager,
		peerRegistry: peerRegistry,
		natTraversal: natTraversal,
	}
}

func (s *RealRoomService) Create(ctx context.Context, cfg models.RoomConfig) (*models.Room, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Initialize WireGuard (host gets mesh IP .1)
	meshIP := infra.AllocateMeshIP(0)
	if err := s.wgManager.Initialize(s.cfg.WireGuardPort, meshIP); err != nil {
		return nil, fmt.Errorf("wireguard init: %w", err)
	}

	// 2. Generate room identifiers
	roomID := generateID(8)
	inviteCode := generateID(6)

	// 3. Auto-detect external endpoint via STUN if not configured
	if s.natTraversal != nil && s.cfg.Endpoint == "" {
		endpoint, natType, err := s.natTraversal.DiscoverExternalEndpoint(ctx, s.cfg.WireGuardPort)
		if err == nil {
			s.cfg.Endpoint = endpoint
			logger.Info("STUN discovered endpoint",
				"component", "nat",
				"endpoint", endpoint,
				"nat_type", string(natType),
			)
		} else {
			logger.Warn("STUN discovery failed, using local IP",
				"component", "nat",
				"error", err,
			)
		}
		if natType == infra.NATTypeSymmetric {
			if s.natTraversal.HasTURN() {
				relayAddr, relayErr := s.natTraversal.AllocateRelay(ctx)
				if relayErr == nil {
					s.cfg.Endpoint = relayAddr
					logger.Info("TURN relay allocated for symmetric NAT",
						"component", "nat",
						"relay_addr", relayAddr,
					)
				} else {
					logger.Warn("TURN relay allocation failed",
						"component", "nat",
						"error", relayErr,
					)
				}
			} else {
				logger.Warn("symmetric NAT detected — no TURN configured, P2P may fail",
					"component", "nat",
				)
			}
		}
	}

	// 4. Register room on signaling server
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

	// 5. Create PeerGRPCServer with callbacks
	peerSrv, err := infra.NewPeerGRPCServer(infra.PeerGRPCServerConfig{
		LocalPeerID: s.cfg.LocalPeerID,
		LocalWorker: func() workerpb.WorkerServiceClient { return nil }, // set later by inference service
		RoomToken:   inviteCode,
		OnHandshake: func(peerID string, resources *workerpb.ResourceUsage) {
			s.addPeerToRoom(roomID, peerID, resources)
		},
		GetRoomState: func() *peerpb.RoomState {
			return s.getRoomStateProto(roomID)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("peer server: %w", err)
	}

	// 6. Start gRPC server for peer connections
	if err := peerSrv.Start(s.cfg.GRPCPort); err != nil {
		return nil, fmt.Errorf("peer server start: %w", err)
	}

	// 7. Write WireGuard config
	if _, err := s.wgManager.WriteConfig(); err != nil {
		logger.Warn("failed to write wireguard config", "error", err)
	}

	// 8. Bring up WireGuard mesh (Linux only)
	s.bringUpMesh()

	// 9. Determine initial state
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

	room := &models.Room{
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

	assignLayers(room)

	rc := &roomContext{
		room:         room,
		startAt:      time.Now(),
		peerRegistry: s.peerRegistry,
		peerServer:   peerSrv,
	}

	// Start pending timer if resources insufficient
	if state == models.RoomStatePending {
		rc.pendingTimer = time.AfterFunc(realPendingTimeout, func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if entry, ok := s.rooms[roomID]; ok && entry.room.State == models.RoomStatePending {
				entry.room.State = models.RoomStateClosed
				s.cleanupRoom(roomID)
			}
		})
	}

	s.rooms[roomID] = rc

	// Start resilience service for active rooms
	if state == models.RoomStateActive {
		s.startResilienceForRoom(rc)
	}

	logger.Info("room created",
		"room_id", roomID,
		"invite", inviteCode,
		"model", cfg.ModelID,
		"state", state,
	)

	return room, nil
}

func (s *RealRoomService) Join(ctx context.Context, inviteCode string, resources models.ResourceSpec) (*models.Room, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Initialize WireGuard first to get public key
	// Use a temporary mesh IP — will be overwritten by signaling response
	if err := s.wgManager.Initialize(s.cfg.WireGuardPort, "10.42.0.99/24"); err != nil {
		return nil, fmt.Errorf("wireguard init: %w", err)
	}

	// 2. Auto-detect external endpoint via STUN if not configured
	if s.natTraversal != nil && s.cfg.Endpoint == "" {
		endpoint, natType, err := s.natTraversal.DiscoverExternalEndpoint(ctx, s.cfg.WireGuardPort)
		if err == nil {
			s.cfg.Endpoint = endpoint
			logger.Info("STUN discovered endpoint",
				"component", "nat",
				"endpoint", endpoint,
				"nat_type", string(natType),
			)
		} else {
			logger.Warn("STUN discovery failed, using local IP",
				"component", "nat",
				"error", err,
			)
		}
		if natType == infra.NATTypeSymmetric {
			if s.natTraversal.HasTURN() {
				relayAddr, relayErr := s.natTraversal.AllocateRelay(ctx)
				if relayErr == nil {
					s.cfg.Endpoint = relayAddr
					logger.Info("TURN relay allocated for symmetric NAT",
						"component", "nat",
						"relay_addr", relayAddr,
					)
				} else {
					logger.Warn("TURN relay allocation failed",
						"component", "nat",
						"error", relayErr,
					)
				}
			} else {
				logger.Warn("symmetric NAT detected — no TURN configured, P2P may fail",
					"component", "nat",
				)
			}
		}
	}

	// 3. Join via signaling server (with correct public key)
	joinResp, err := s.sigClient.JoinRoom(ctx, infra.SignalingJoinRequest{
		InviteCode: inviteCode,
		PeerID:     s.cfg.LocalPeerID,
		PublicKey:  s.wgManager.PublicKey(),
		Endpoint:   s.cfg.Endpoint,
	})
	if err != nil {
		return nil, fmt.Errorf("signaling join: %w", err)
	}

	// 4. Add existing peers to WireGuard config
	for _, p := range joinResp.Peers {
		if err := s.wgManager.AddPeer(p.PublicKey, p.Endpoint, p.MeshIP); err != nil {
			logger.Warn("failed to add peer to wireguard", "peer", p.ID, "error", err)
		}
	}

	// 5. Write WireGuard config and bring up mesh
	if _, err := s.wgManager.WriteConfig(); err != nil {
		logger.Warn("failed to write wireguard config", "error", err)
	}
	s.bringUpMesh()

	// 6. Register peers in peer registry and perform handshakes
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

	// 7. Build room
	room := &models.Room{
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
		room.HostID = p.ID
		break
	}

	assignLayers(room)

	rc := &roomContext{
		room:         room,
		startAt:      time.Now(),
		peerRegistry: s.peerRegistry,
	}
	s.rooms[joinResp.RoomID] = rc

	// Start resilience service
	s.startResilienceForRoom(rc)

	logger.Info("joined room",
		"room_id", joinResp.RoomID,
		"mesh_ip", joinResp.MeshIP,
		"peers", len(roomPeers),
	)

	return room, nil
}

func (s *RealRoomService) Leave(ctx context.Context, roomID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rc, ok := s.rooms[roomID]
	if !ok {
		return models.ErrNotInRoom
	}

	// Notify signaling server
	if err := s.sigClient.LeaveRoom(ctx, rc.room.InviteCode, s.cfg.LocalPeerID); err != nil {
		logger.Warn("failed to notify signaling on leave", "error", err)
	}

	s.cleanupRoom(roomID)
	return nil
}

func (s *RealRoomService) Stop(ctx context.Context, roomID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rc, ok := s.rooms[roomID]
	if !ok {
		return models.ErrNotInRoom
	}

	rc.room.State = models.RoomStateClosed

	// Notify signaling server
	if err := s.sigClient.LeaveRoom(ctx, rc.room.InviteCode, s.cfg.LocalPeerID); err != nil {
		logger.Warn("failed to notify signaling on stop", "error", err)
	}

	s.cleanupRoom(roomID)
	return nil
}

func (s *RealRoomService) Status(_ context.Context, roomID string) (*models.RoomStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rc, ok := s.rooms[roomID]
	if !ok {
		return nil, models.ErrNotInRoom
	}

	var totalVRAM, usedVRAM int64
	for _, p := range rc.room.Peers {
		totalVRAM += p.Resources.VRAMTotal
		usedVRAM += p.Resources.VRAMTotal - p.Resources.VRAMFree
	}

	uptime := time.Since(rc.startAt).Round(time.Second).String()

	return &models.RoomStatus{
		Room:         *rc.room,
		TotalVRAM:    totalVRAM,
		UsedVRAM:     usedVRAM,
		TokensPerSec: 0, // populated by inference service
		Uptime:       uptime,
	}, nil
}

func (s *RealRoomService) CurrentRoom() *models.Room {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, rc := range s.rooms {
		if rc.room.State == models.RoomStateActive || rc.room.State == models.RoomStatePending {
			return rc.room
		}
	}
	return nil
}

func (s *RealRoomService) GetRoom(roomID string) *models.Room {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if rc, ok := s.rooms[roomID]; ok {
		return rc.room
	}
	return nil
}

func (s *RealRoomService) ListRooms() []*models.Room {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rooms := make([]*models.Room, 0, len(s.rooms))
	for _, rc := range s.rooms {
		rooms = append(rooms, rc.room)
	}
	return rooms
}

func (s *RealRoomService) ActiveRoomID() string {
	room := s.CurrentRoom()
	if room != nil {
		return room.ID
	}
	return ""
}

// addPeerToRoom is called when a remote peer completes handshake (host side).
func (s *RealRoomService) addPeerToRoom(roomID, peerID string, resources *workerpb.ResourceUsage) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rc, ok := s.rooms[roomID]
	if !ok {
		return
	}

	// Check if peer already exists
	for _, p := range rc.room.Peers {
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

	rc.room.Peers = append(rc.room.Peers, models.Peer{
		ID:        peerID,
		Name:      peerID,
		IP:        "",
		State:     models.PeerStateReady,
		Resources: res,
		JoinedAt:  time.Now(),
		IsHost:    false,
	})

	// Re-check if room can become active
	if rc.room.State == models.RoomStatePending {
		var totalVRAM int64
		for _, p := range rc.room.Peers {
			totalVRAM += p.Resources.TotalUsableVRAM()
		}
		modelReqs := catalog.Lookup(rc.room.ModelID)
		if modelReqs != nil && totalVRAM >= modelReqs.MinVRAMMB {
			rc.room.State = models.RoomStateActive
			if rc.pendingTimer != nil {
				rc.pendingTimer.Stop()
				rc.pendingTimer = nil
			}
			s.startResilienceForRoom(rc)
		}
	}

	assignLayers(rc.room)

	// Register peer in resilience monitoring
	if rc.resilience != nil {
		rc.resilience.RegisterPeer(peerID)
	}

	logger.Info("peer added to room via handshake",
		"room_id", roomID,
		"peer_id", peerID,
		"total_peers", len(rc.room.Peers),
	)
}

// getRoomStateProto returns the current room state for the PeerGRPCServer.
func (s *RealRoomService) getRoomStateProto(roomID string) *peerpb.RoomState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rc, ok := s.rooms[roomID]
	if !ok {
		return nil
	}

	assignments := make([]*peerpb.PeerAssignment, 0, len(rc.room.Peers))
	for _, p := range rc.room.Peers {
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
		ModelId:     rc.room.ModelID,
		TotalLayers: int32(rc.room.TotalLayers),
		Assignments: assignments,
	}
}

// cleanupRoom tears down networking resources for a specific room. Must be called with mu held.
func (s *RealRoomService) cleanupRoom(roomID string) {
	rc, ok := s.rooms[roomID]
	if !ok {
		return
	}

	if rc.pendingTimer != nil {
		rc.pendingTimer.Stop()
		rc.pendingTimer = nil
	}

	if rc.resilience != nil {
		rc.resilience.Stop()
		rc.resilience = nil
	}

	if rc.peerRegistry != nil {
		rc.peerRegistry.Close()
	}

	if rc.peerServer != nil {
		rc.peerServer.Stop()
		rc.peerServer = nil
	}

	delete(s.rooms, roomID)

	// Only tear down shared resources if no rooms remain
	if len(s.rooms) == 0 {
		// Deallocate TURN relay if active
		if s.natTraversal != nil {
			_ = s.natTraversal.DeallocateRelay(context.Background())
		}

		// Bring down WireGuard mesh (Linux only)
		s.bringDownMesh()
	}
}

// startResilienceForRoom initializes health monitoring for all remote peers in a room.
func (s *RealRoomService) startResilienceForRoom(rc *roomContext) {
	if rc.peerRegistry == nil {
		return
	}

	rc.resilience = NewResilienceService(s, rc.peerRegistry, s.cfg.LocalPeerID)
	rc.resilience.Start()

	// Register all remote peers for monitoring
	for _, p := range rc.room.Peers {
		if p.ID != s.cfg.LocalPeerID {
			rc.resilience.RegisterPeer(p.ID)
		}
	}
}

// bringUpMesh attempts to bring up the WireGuard interface (Linux only).
func (s *RealRoomService) bringUpMesh() {
	if runtime.GOOS != "linux" {
		logger.Info("skipping wg-quick up (non-linux)", "os", runtime.GOOS)
		return
	}
	logger.Info("wireguard mesh config written (wg-quick up skipped in container)")
}

// bringDownMesh tears down the WireGuard interface (Linux only).
func (s *RealRoomService) bringDownMesh() {
	if runtime.GOOS != "linux" {
		return
	}
	logger.Info("wireguard mesh torn down (wg-quick down skipped in container)")
}
