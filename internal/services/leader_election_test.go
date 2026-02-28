package services

import (
	"fmt"
	"testing"

	"github.com/joaopedro/hivemind/internal/models"
)

func TestElectLeader_LowestIP(t *testing.T) {
	le := NewLeaderElection(LeaderElectionConfig{
		LocalPeerID: "peer-3",
		LocalIP:     "10.42.0.3",
	})

	peers := []models.Peer{
		{ID: "peer-3", IP: "10.42.0.3"},
		{ID: "peer-1", IP: "10.42.0.1"},
		{ID: "peer-2", IP: "10.42.0.2"},
	}

	leader := le.ElectLeader(peers)
	if leader != "peer-1" {
		t.Errorf("expected peer-1 (lowest IP 10.42.0.1), got %s", leader)
	}
}

func TestElectLeader_LocalBecomesLeader(t *testing.T) {
	becameLeader := false
	le := NewLeaderElection(LeaderElectionConfig{
		LocalPeerID: "peer-1",
		LocalIP:     "10.42.0.1",
		OnBecomeLeader: func() {
			becameLeader = true
		},
	})

	peers := []models.Peer{
		{ID: "peer-1", IP: "10.42.0.1"},
		{ID: "peer-2", IP: "10.42.0.2"},
		{ID: "peer-3", IP: "10.42.0.3"},
	}

	leader := le.ElectLeader(peers)

	if leader != "peer-1" {
		t.Errorf("expected peer-1, got %s", leader)
	}
	if !le.IsLeader() {
		t.Error("local peer should be leader")
	}
	if !becameLeader {
		t.Error("onBecomeLeader callback should have been called")
	}
}

func TestElectLeader_NotLocalLeader(t *testing.T) {
	le := NewLeaderElection(LeaderElectionConfig{
		LocalPeerID: "peer-3",
		LocalIP:     "10.42.0.3",
	})

	peers := []models.Peer{
		{ID: "peer-1", IP: "10.42.0.1"},
		{ID: "peer-3", IP: "10.42.0.3"},
	}

	le.ElectLeader(peers)

	if le.IsLeader() {
		t.Error("peer-3 should not be leader when peer-1 has lower IP")
	}
}

func TestElectLeader_EmptyPeers(t *testing.T) {
	le := NewLeaderElection(LeaderElectionConfig{
		LocalPeerID: "peer-1",
	})

	leader := le.ElectLeader(nil)
	if leader != "" {
		t.Errorf("expected empty leader for no peers, got %s", leader)
	}
}

func TestElectLeader_SinglePeer(t *testing.T) {
	le := NewLeaderElection(LeaderElectionConfig{
		LocalPeerID: "solo",
		LocalIP:     "10.42.0.1",
	})

	leader := le.ElectLeader([]models.Peer{
		{ID: "solo", IP: "10.42.0.1"},
	})

	if leader != "solo" {
		t.Errorf("expected solo peer as leader, got %s", leader)
	}
	if !le.IsLeader() {
		t.Error("solo peer should be leader")
	}
}

func TestCompareIPs(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int // -1, 0, or 1
	}{
		{"10.42.0.1", "10.42.0.2", -1},
		{"10.42.0.2", "10.42.0.1", 1},
		{"10.42.0.1", "10.42.0.1", 0},
		{"10.42.0.10", "10.42.0.9", 1},
		{"10.42.0.1", "10.42.1.1", -1},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			result := compareIPs(
				parseTestIP(t, tt.a),
				parseTestIP(t, tt.b),
			)
			if (tt.expected < 0 && result >= 0) ||
				(tt.expected > 0 && result <= 0) ||
				(tt.expected == 0 && result != 0) {
				t.Errorf("compareIPs(%s, %s) = %d, expected sign %d", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func parseTestIP(t *testing.T, s string) []byte {
	t.Helper()
	ip := make([]byte, 4)
	var a, b, c, d byte
	n, _ := fmt.Sscanf(s, "%d.%d.%d.%d", &a, &b, &c, &d)
	if n != 4 {
		t.Fatalf("invalid IP: %s", s)
	}
	ip[0], ip[1], ip[2], ip[3] = a, b, c, d
	return ip
}
