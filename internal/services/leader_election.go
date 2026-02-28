package services

import (
	"net"
	"sort"
	"sync"

	"github.com/joaopedro/hivemind/internal/infra"
	"github.com/joaopedro/hivemind/internal/logger"
	"github.com/joaopedro/hivemind/internal/models"
)

// LeaderElection implements leader election based on lowest-IP strategy.
// When the host dies, the peer with the lowest mesh IP becomes the new host.
// State is replicated every 30s via gRPC SyncState so the new leader has
// a recent snapshot of the room state.
type LeaderElection struct {
	mu          sync.RWMutex
	localPeerID string
	localIP     string
	isLeader    bool
	roomSvc     RoomService
	registry    *infra.PeerRegistry
	onBecomeLeader func()
}

// LeaderElectionConfig holds configuration for leader election.
type LeaderElectionConfig struct {
	LocalPeerID    string
	LocalIP        string
	RoomSvc        RoomService
	Registry       *infra.PeerRegistry
	OnBecomeLeader func()
}

// NewLeaderElection creates a leader election coordinator.
func NewLeaderElection(cfg LeaderElectionConfig) *LeaderElection {
	return &LeaderElection{
		localPeerID:    cfg.LocalPeerID,
		localIP:        cfg.LocalIP,
		roomSvc:        cfg.RoomSvc,
		registry:       cfg.Registry,
		onBecomeLeader: cfg.OnBecomeLeader,
	}
}

// ElectLeader performs leader election among alive peers.
// Returns the peer ID of the new leader (lowest IP).
func (le *LeaderElection) ElectLeader(alivePeers []models.Peer) string {
	if len(alivePeers) == 0 {
		return ""
	}

	// Sort by IP address (string comparison works for 10.42.0.X format)
	sorted := make([]models.Peer, len(alivePeers))
	copy(sorted, alivePeers)

	sort.Slice(sorted, func(i, j int) bool {
		ipI := net.ParseIP(sorted[i].IP)
		ipJ := net.ParseIP(sorted[j].IP)
		if ipI == nil || ipJ == nil {
			return sorted[i].IP < sorted[j].IP
		}
		return compareIPs(ipI, ipJ) < 0
	})

	newLeader := sorted[0].ID

	le.mu.Lock()
	wasLeader := le.isLeader
	le.isLeader = newLeader == le.localPeerID
	le.mu.Unlock()

	if le.isLeader && !wasLeader {
		logger.Info("this node elected as new leader",
			"peer_id", le.localPeerID,
			"ip", le.localIP,
			"alive_peers", len(alivePeers),
		)

		if le.onBecomeLeader != nil {
			le.onBecomeLeader()
		}
	}

	return newLeader
}

// HandleHostDeath is called when the current host is detected as dead.
// It triggers leader election among remaining alive peers.
func (le *LeaderElection) HandleHostDeath(deadHostID string) string {
	room := le.roomSvc.CurrentRoom()
	if room == nil {
		return ""
	}

	var alivePeers []models.Peer
	for _, p := range room.Peers {
		if p.ID != deadHostID && p.State != models.PeerStateOffline {
			alivePeers = append(alivePeers, p)
		}
	}

	if len(alivePeers) == 0 {
		logger.Error("no alive peers for leader election")
		return ""
	}

	newLeader := le.ElectLeader(alivePeers)

	logger.Info("leader election completed",
		"dead_host", deadHostID,
		"new_leader", newLeader,
		"alive_peers", len(alivePeers),
	)

	return newLeader
}

// IsLeader returns whether this node is the current leader.
func (le *LeaderElection) IsLeader() bool {
	le.mu.RLock()
	defer le.mu.RUnlock()
	return le.isLeader
}

// SetLeader explicitly sets this node as leader (used on initial room creation).
func (le *LeaderElection) SetLeader(isLeader bool) {
	le.mu.Lock()
	defer le.mu.Unlock()
	le.isLeader = isLeader
}

// compareIPs compares two IP addresses numerically.
// Returns negative if a < b, 0 if equal, positive if a > b.
func compareIPs(a, b net.IP) int {
	aBytes := a.To4()
	bBytes := b.To4()

	if aBytes == nil || bBytes == nil {
		// Fallback to string comparison for IPv6
		return len(a.String()) - len(b.String())
	}

	for i := range aBytes {
		if aBytes[i] < bBytes[i] {
			return -1
		}
		if aBytes[i] > bBytes[i] {
			return 1
		}
	}
	return 0
}
