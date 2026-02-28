package infra

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/joaopedro/hivemind/gen/peerpb"
	"github.com/joaopedro/hivemind/gen/workerpb"
	"github.com/joaopedro/hivemind/internal/logger"
)

// PeerNode represents a connected peer with its gRPC connection.
type PeerNode struct {
	ID        string
	IP        string
	PublicKey string
	Layers    []int32
	Resources *workerpb.ResourceUsage
	conn      *grpc.ClientConn
	client    peerpb.PeerServiceClient
	Latency   float64 // measured latency in ms
}

// Client returns the gRPC client for this peer.
func (p *PeerNode) Client() peerpb.PeerServiceClient {
	return p.client
}

// PeerRegistry manages connected peers and their gRPC connections.
type PeerRegistry struct {
	mu          sync.RWMutex
	peers       map[string]*PeerNode
	localPeerID string
	grpcPort    int
}

// NewPeerRegistry creates a peer registry for managing peer connections.
func NewPeerRegistry(localPeerID string, grpcPort int) *PeerRegistry {
	return &PeerRegistry{
		peers:       make(map[string]*PeerNode),
		localPeerID: localPeerID,
		grpcPort:    grpcPort,
	}
}

// AddPeer registers a peer and establishes a gRPC connection.
func (pr *PeerRegistry) AddPeer(ctx context.Context, id, ip, publicKey string) error {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	if _, exists := pr.peers[id]; exists {
		return nil // Already connected
	}

	addr := fmt.Sprintf("%s:%d", ip, pr.grpcPort)
	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to peer %s at %s: %w", id, addr, err)
	}

	client := peerpb.NewPeerServiceClient(conn)

	// Measure latency with a health check
	latency := pr.measureLatency(ctx, client)

	peer := &PeerNode{
		ID:        id,
		IP:        ip,
		PublicKey: publicKey,
		conn:      conn,
		client:    client,
		Latency:   latency,
	}

	pr.peers[id] = peer

	logger.Info("peer connected",
		"peer_id", id,
		"address", addr,
		"latency_ms", latency,
	)

	return nil
}

// RemovePeer disconnects and removes a peer.
func (pr *PeerRegistry) RemovePeer(id string) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	peer, exists := pr.peers[id]
	if !exists {
		return
	}

	if peer.conn != nil {
		peer.conn.Close()
	}

	delete(pr.peers, id)
	logger.Info("peer disconnected", "peer_id", id)
}

// GetPeer returns a peer by ID.
func (pr *PeerRegistry) GetPeer(id string) (*PeerNode, bool) {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	peer, ok := pr.peers[id]
	return peer, ok
}

// GetAllPeers returns all connected peers.
func (pr *PeerRegistry) GetAllPeers() []*PeerNode {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	peers := make([]*PeerNode, 0, len(pr.peers))
	for _, p := range pr.peers {
		peers = append(peers, p)
	}
	return peers
}

// PeerCount returns the number of connected peers.
func (pr *PeerRegistry) PeerCount() int {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	return len(pr.peers)
}

// ForwardToNextPeer sends a forward pass request to the peer responsible for the next layers.
func (pr *PeerRegistry) ForwardToNextPeer(ctx context.Context, peerID string, req *workerpb.ForwardRequest) (*workerpb.ForwardResponse, error) {
	pr.mu.RLock()
	peer, exists := pr.peers[peerID]
	pr.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("peer %s not found in registry", peerID)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := peer.client.ForwardTensor(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("forward pass to peer %s failed: %w", peerID, err)
	}

	return resp, nil
}

// Handshake performs a handshake with a peer, exchanging resources and room state.
func (pr *PeerRegistry) Handshake(ctx context.Context, peerID, roomToken string, resources *workerpb.ResourceUsage) (*peerpb.HandshakeResponse, error) {
	pr.mu.RLock()
	peer, exists := pr.peers[peerID]
	pr.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("peer %s not found", peerID)
	}

	req := &peerpb.HandshakeRequest{
		PeerId:    pr.localPeerID,
		RoomToken: roomToken,
		Resources: resources,
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := peer.client.Handshake(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("handshake with peer %s failed: %w", peerID, err)
	}

	return resp, nil
}

// UpdatePeerLayers updates the layer assignment for a peer.
func (pr *PeerRegistry) UpdatePeerLayers(peerID string, layers []int32) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	if peer, exists := pr.peers[peerID]; exists {
		peer.Layers = layers
	}
}

func (pr *PeerRegistry) measureLatency(ctx context.Context, client peerpb.PeerServiceClient) float64 {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := client.HealthCheck(ctx, &peerpb.Ping{
		Timestamp: time.Now().UnixMilli(),
	})
	if err != nil {
		return -1 // Unknown latency
	}

	return float64(time.Since(start).Milliseconds())
}

// Close disconnects all peers.
func (pr *PeerRegistry) Close() {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	for id, peer := range pr.peers {
		if peer.conn != nil {
			peer.conn.Close()
		}
		delete(pr.peers, id)
	}

	logger.Info("all peers disconnected")
}
