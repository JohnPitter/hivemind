package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/joaopedro/hivemind/internal/logger"
)

// SignalingServer is a lightweight matchmaking/signaling server for room discovery
// and WireGuard key exchange. It holds NO data at rest — rooms exist only in memory
// while participants are connected. Zero trust: it never sees model data or inference.
type SignalingServer struct {
	mu    sync.RWMutex
	rooms map[string]*SignalingRoom
	port  int
}

// SignalingRoom holds the in-memory state of a room on the signaling server.
type SignalingRoom struct {
	ID         string                    `json:"id"`
	InviteCode string                    `json:"invite_code"`
	ModelID    string                    `json:"model_id"`
	HostID     string                    `json:"host_id"`
	MaxPeers   int                       `json:"max_peers"`
	Peers      map[string]*SignalingPeer `json:"peers"`
	CreatedAt  time.Time                 `json:"created_at"`
}

// SignalingPeer holds peer connection info for key exchange.
type SignalingPeer struct {
	ID        string `json:"id"`
	PublicKey string `json:"public_key"`
	Endpoint  string `json:"endpoint"`  // public IP:port for WireGuard
	MeshIP    string `json:"mesh_ip"`   // allocated mesh IP (e.g., 10.42.0.1/24)
	JoinedAt  time.Time `json:"joined_at"`
}

// SignalingCreateRequest is sent by the host to create a room.
type SignalingCreateRequest struct {
	RoomID     string `json:"room_id"`
	InviteCode string `json:"invite_code"`
	ModelID    string `json:"model_id"`
	HostID     string `json:"host_id"`
	MaxPeers   int    `json:"max_peers"`
	PublicKey  string `json:"public_key"`
	Endpoint   string `json:"endpoint"`
}

// SignalingJoinRequest is sent by a peer to join a room.
type SignalingJoinRequest struct {
	InviteCode string `json:"invite_code"`
	PeerID     string `json:"peer_id"`
	PublicKey  string `json:"public_key"`
	Endpoint   string `json:"endpoint"`
}

// SignalingJoinResponse returns room info and all peer connection details.
type SignalingJoinResponse struct {
	RoomID  string           `json:"room_id"`
	ModelID string           `json:"model_id"`
	MeshIP  string           `json:"mesh_ip"` // assigned mesh IP for the joining peer
	Peers   []SignalingPeer  `json:"peers"`   // all existing peers for WG config
}

// NewSignalingServer creates a new signaling server.
func NewSignalingServer(port int) *SignalingServer {
	return &SignalingServer{
		rooms: make(map[string]*SignalingRoom),
		port:  port,
	}
}

// Start runs the signaling server.
func (s *SignalingServer) Start(_ context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /signal/create", s.HandleCreate)
	mux.HandleFunc("POST /signal/join", s.HandleJoin)
	mux.HandleFunc("GET /signal/peers", s.HandlePeers)
	mux.HandleFunc("DELETE /signal/leave", s.HandleLeave)
	mux.HandleFunc("GET /signal/health", s.HandleHealth)

	addr := fmt.Sprintf(":%d", s.port)
	logger.Info("signaling server starting", "address", addr)

	return http.ListenAndServe(addr, mux)
}

func (s *SignalingServer) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var req SignalingCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.rooms[req.InviteCode]; exists {
		http.Error(w, `{"error":"room already exists"}`, http.StatusConflict)
		return
	}

	room := &SignalingRoom{
		ID:         req.RoomID,
		InviteCode: req.InviteCode,
		ModelID:    req.ModelID,
		HostID:     req.HostID,
		MaxPeers:   req.MaxPeers,
		Peers:      make(map[string]*SignalingPeer),
		CreatedAt:  time.Now(),
	}

	// Host is the first peer (gets mesh IP .1)
	room.Peers[req.HostID] = &SignalingPeer{
		ID:        req.HostID,
		PublicKey: req.PublicKey,
		Endpoint:  req.Endpoint,
		MeshIP:    AllocateMeshIP(0),
		JoinedAt:  time.Now(),
	}

	s.rooms[req.InviteCode] = room

	logger.Info("room created on signaling server",
		"room_id", req.RoomID,
		"invite", req.InviteCode,
		"model", req.ModelID,
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "created",
		"room_id": req.RoomID,
		"mesh_ip": AllocateMeshIP(0),
	})
}

func (s *SignalingServer) HandleJoin(w http.ResponseWriter, r *http.Request) {
	var req SignalingJoinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	room, exists := s.rooms[req.InviteCode]
	if !exists {
		http.Error(w, `{"error":"room not found"}`, http.StatusNotFound)
		return
	}

	if len(room.Peers) >= room.MaxPeers {
		http.Error(w, `{"error":"room full"}`, http.StatusConflict)
		return
	}

	if _, already := room.Peers[req.PeerID]; already {
		http.Error(w, `{"error":"already in room"}`, http.StatusConflict)
		return
	}

	// Allocate next mesh IP
	meshIP := AllocateMeshIP(len(room.Peers))

	newPeer := &SignalingPeer{
		ID:        req.PeerID,
		PublicKey: req.PublicKey,
		Endpoint:  req.Endpoint,
		MeshIP:    meshIP,
		JoinedAt:  time.Now(),
	}

	// Collect existing peers before adding new one (for WG config)
	var existingPeers []SignalingPeer
	for _, p := range room.Peers {
		existingPeers = append(existingPeers, *p)
	}

	room.Peers[req.PeerID] = newPeer

	logger.Info("peer joined room via signaling",
		"room_id", room.ID,
		"peer_id", req.PeerID,
		"mesh_ip", meshIP,
		"total_peers", len(room.Peers),
	)

	resp := SignalingJoinResponse{
		RoomID:  room.ID,
		ModelID: room.ModelID,
		MeshIP:  meshIP,
		Peers:   existingPeers,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *SignalingServer) HandlePeers(w http.ResponseWriter, r *http.Request) {
	inviteCode := r.URL.Query().Get("invite")
	if inviteCode == "" {
		http.Error(w, `{"error":"invite code required"}`, http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	room, exists := s.rooms[inviteCode]
	if !exists {
		http.Error(w, `{"error":"room not found"}`, http.StatusNotFound)
		return
	}

	var peers []SignalingPeer
	for _, p := range room.Peers {
		peers = append(peers, *p)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(peers)
}

func (s *SignalingServer) HandleLeave(w http.ResponseWriter, r *http.Request) {
	inviteCode := r.URL.Query().Get("invite")
	peerID := r.URL.Query().Get("peer_id")

	if inviteCode == "" || peerID == "" {
		http.Error(w, `{"error":"invite and peer_id required"}`, http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	room, exists := s.rooms[inviteCode]
	if !exists {
		w.WriteHeader(http.StatusOK) // Idempotent
		return
	}

	delete(room.Peers, peerID)

	// If room is empty, clean it up
	if len(room.Peers) == 0 {
		delete(s.rooms, inviteCode)
		logger.Info("room cleaned up (empty)", "room_id", room.ID)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "left"})
}

func (s *SignalingServer) HandleHealth(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	roomCount := len(s.rooms)
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "ok",
		"rooms":  roomCount,
	})
}
