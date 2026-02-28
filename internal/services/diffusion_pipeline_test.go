package services

import (
	"testing"

	"github.com/joaopedro/hivemind/internal/models"
)

func TestAssignStages_SinglePeer(t *testing.T) {
	dp := &DiffusionPipelineService{
		localPeerID: "solo",
	}

	peers := []models.Peer{
		{ID: "solo", Resources: models.ResourceSpec{VRAMFree: 24000}},
	}

	assignments := dp.AssignStages(peers)

	if len(assignments) != 3 {
		t.Fatalf("expected 3 stage assignments, got %d", len(assignments))
	}

	// Single peer should get all stages
	for _, a := range assignments {
		if a.PeerID != "solo" {
			t.Errorf("expected all stages assigned to 'solo', got %s for stage %s", a.PeerID, a.Stage)
		}
		if !a.IsLocal {
			t.Error("single peer should be local")
		}
	}

	if assignments[0].Stage != StageTextEncoder {
		t.Errorf("expected first stage to be text_encoder, got %s", assignments[0].Stage)
	}
	if assignments[1].Stage != StageUNet {
		t.Errorf("expected second stage to be unet, got %s", assignments[1].Stage)
	}
	if assignments[2].Stage != StageVAEDecoder {
		t.Errorf("expected third stage to be vae_decoder, got %s", assignments[2].Stage)
	}
}

func TestAssignStages_TwoPeers(t *testing.T) {
	dp := &DiffusionPipelineService{
		localPeerID: "big",
	}

	peers := []models.Peer{
		{ID: "small", Resources: models.ResourceSpec{VRAMFree: 8000}},
		{ID: "big", Resources: models.ResourceSpec{VRAMFree: 24000}},
	}

	assignments := dp.AssignStages(peers)

	if len(assignments) != 3 {
		t.Fatalf("expected 3 stage assignments, got %d", len(assignments))
	}

	// UNet (heaviest) should go to peer with most VRAM
	if assignments[1].PeerID != "big" {
		t.Errorf("UNet should be assigned to 'big' (most VRAM), got %s", assignments[1].PeerID)
	}

	// Text encoder should go to second peer
	if assignments[0].PeerID != "small" {
		t.Errorf("TextEncoder should be assigned to 'small', got %s", assignments[0].PeerID)
	}
}

func TestAssignStages_ThreePeers(t *testing.T) {
	dp := &DiffusionPipelineService{
		localPeerID: "big",
	}

	peers := []models.Peer{
		{ID: "small", Resources: models.ResourceSpec{VRAMFree: 6000}},
		{ID: "big", Resources: models.ResourceSpec{VRAMFree: 24000}},
		{ID: "medium", Resources: models.ResourceSpec{VRAMFree: 12000}},
	}

	assignments := dp.AssignStages(peers)

	if len(assignments) != 3 {
		t.Fatalf("expected 3 stage assignments, got %d", len(assignments))
	}

	// UNet -> big (most VRAM)
	if assignments[1].PeerID != "big" {
		t.Errorf("UNet should be on 'big', got %s", assignments[1].PeerID)
	}

	// TextEncoder -> medium (second most VRAM)
	if assignments[0].PeerID != "medium" {
		t.Errorf("TextEncoder should be on 'medium', got %s", assignments[0].PeerID)
	}

	// VAE -> small (third)
	if assignments[2].PeerID != "small" {
		t.Errorf("VAE should be on 'small', got %s", assignments[2].PeerID)
	}
}

func TestAssignStages_Empty(t *testing.T) {
	dp := &DiffusionPipelineService{
		localPeerID: "test",
	}

	assignments := dp.AssignStages(nil)
	if assignments != nil {
		t.Error("expected nil assignments for no peers")
	}
}
