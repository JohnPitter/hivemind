package services

import (
	"testing"

	"github.com/joaopedro/hivemind/internal/models"
)

func TestAssignLayersByVRAM(t *testing.T) {
	peers := []models.Peer{
		{
			ID: "host",
			Resources: models.ResourceSpec{
				VRAMTotal: 24576, // 24GB
				VRAMFree:  20000,
			},
		},
		{
			ID: "peer-1",
			Resources: models.ResourceSpec{
				VRAMTotal: 12288, // 12GB
				VRAMFree:  10000,
			},
		},
		{
			ID: "peer-2",
			Resources: models.ResourceSpec{
				VRAMTotal: 8192, // 8GB
				VRAMFree:  6000,
			},
		},
	}

	totalLayers := 80
	assignments := AssignLayersByVRAM(peers, totalLayers)

	if assignments == nil {
		t.Fatal("assignments should not be nil")
	}

	// All layers should be assigned
	totalAssigned := 0
	for _, layers := range assignments {
		totalAssigned += len(layers)
	}

	if totalAssigned != totalLayers {
		t.Errorf("expected %d total layers assigned, got %d", totalLayers, totalAssigned)
	}

	// Host (most VRAM) should get the most layers
	hostLayers := len(assignments["host"])
	peer1Layers := len(assignments["peer-1"])
	peer2Layers := len(assignments["peer-2"])

	if hostLayers <= peer1Layers {
		t.Errorf("host (%d layers) should have more than peer-1 (%d layers)", hostLayers, peer1Layers)
	}

	if peer1Layers <= peer2Layers {
		t.Errorf("peer-1 (%d layers) should have more than peer-2 (%d layers)", peer1Layers, peer2Layers)
	}

	// Layers should be contiguous
	for _, layers := range assignments {
		for i := 1; i < len(layers); i++ {
			if layers[i] != layers[i-1]+1 {
				t.Errorf("layers should be contiguous: got %d after %d", layers[i], layers[i-1])
			}
		}
	}
}

func TestAssignLayersByVRAM_EqualFallback(t *testing.T) {
	peers := []models.Peer{
		{ID: "peer-1", Resources: models.ResourceSpec{VRAMTotal: 0, VRAMFree: 0}},
		{ID: "peer-2", Resources: models.ResourceSpec{VRAMTotal: 0, VRAMFree: 0}},
	}

	assignments := AssignLayersByVRAM(peers, 10)

	if assignments == nil {
		t.Fatal("assignments should not be nil for zero-VRAM peers (equal fallback)")
	}

	// Both should get 5 layers each
	if len(assignments["peer-1"]) != 5 {
		t.Errorf("expected 5 layers for peer-1, got %d", len(assignments["peer-1"]))
	}
	if len(assignments["peer-2"]) != 5 {
		t.Errorf("expected 5 layers for peer-2, got %d", len(assignments["peer-2"]))
	}
}

func TestAssignLayersByVRAM_SinglePeer(t *testing.T) {
	peers := []models.Peer{
		{ID: "solo", Resources: models.ResourceSpec{VRAMTotal: 24576, VRAMFree: 20000}},
	}

	assignments := AssignLayersByVRAM(peers, 40)

	if len(assignments["solo"]) != 40 {
		t.Errorf("single peer should get all 40 layers, got %d", len(assignments["solo"]))
	}
}

func TestAssignLayersByVRAM_Empty(t *testing.T) {
	assignments := AssignLayersByVRAM(nil, 40)
	if assignments != nil {
		t.Error("nil peers should return nil assignments")
	}

	assignments = AssignLayersByVRAM([]models.Peer{{ID: "p1"}}, 0)
	if assignments != nil {
		t.Error("zero layers should return nil assignments")
	}
}
