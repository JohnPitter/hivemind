package services

import (
	"context"
	"fmt"
	"time"

	"github.com/joaopedro/hivemind/gen/workerpb"
	"github.com/joaopedro/hivemind/internal/infra"
	"github.com/joaopedro/hivemind/internal/logger"
	"github.com/joaopedro/hivemind/internal/models"
)

// DiffusionStage represents a stage in the diffusion pipeline.
type DiffusionStage string

const (
	StageTextEncoder DiffusionStage = "text_encoder"
	StageUNet        DiffusionStage = "unet"
	StageVAEDecoder  DiffusionStage = "vae_decoder"
)

// StageAssignment maps a pipeline stage to a peer.
type StageAssignment struct {
	Stage  DiffusionStage
	PeerID string
	IsLocal bool
}

// DiffusionPipelineService coordinates distributed image generation
// by splitting the diffusion pipeline across peers.
// The pipeline has 3 stages: TextEncoder → UNet → VAE Decoder
// Each stage is assigned to a peer based on available VRAM.
type DiffusionPipelineService struct {
	localWorker  func() workerpb.WorkerServiceClient
	peerRegistry *infra.PeerRegistry
	roomSvc      RoomService
	localPeerID  string
}

// NewDiffusionPipelineService creates a distributed diffusion pipeline coordinator.
func NewDiffusionPipelineService(
	workerClient func() workerpb.WorkerServiceClient,
	peerRegistry *infra.PeerRegistry,
	roomSvc RoomService,
	localPeerID string,
) *DiffusionPipelineService {
	return &DiffusionPipelineService{
		localWorker:  workerClient,
		peerRegistry: peerRegistry,
		roomSvc:      roomSvc,
		localPeerID:  localPeerID,
	}
}

// AssignStages distributes diffusion pipeline stages across peers.
// The peer with the most VRAM gets the UNet (heaviest stage).
// Text encoder and VAE decoder go to peers with less VRAM.
func (dp *DiffusionPipelineService) AssignStages(peers []models.Peer) []StageAssignment {
	if len(peers) == 0 {
		return nil
	}

	stages := []DiffusionStage{StageTextEncoder, StageUNet, StageVAEDecoder}

	// Single peer handles everything
	if len(peers) == 1 {
		var assignments []StageAssignment
		for _, stage := range stages {
			assignments = append(assignments, StageAssignment{
				Stage:   stage,
				PeerID:  peers[0].ID,
				IsLocal: peers[0].ID == dp.localPeerID,
			})
		}
		return assignments
	}

	// Sort peers by VRAM (descending)
	type peerVRAM struct {
		peer models.Peer
		vram int64
	}
	sorted := make([]peerVRAM, len(peers))
	for i, p := range peers {
		sorted[i] = peerVRAM{peer: p, vram: p.Resources.TotalUsableVRAM()}
	}
	// Sort descending by VRAM
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].vram > sorted[i].vram {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	assignments := make([]StageAssignment, len(stages))

	// UNet (heaviest) goes to peer with most VRAM
	assignments[1] = StageAssignment{
		Stage:   StageUNet,
		PeerID:  sorted[0].peer.ID,
		IsLocal: sorted[0].peer.ID == dp.localPeerID,
	}

	// Text encoder goes to second peer (or first if only 2 peers)
	textEncoderPeer := sorted[0].peer
	if len(sorted) > 1 {
		textEncoderPeer = sorted[1].peer
	}
	assignments[0] = StageAssignment{
		Stage:   StageTextEncoder,
		PeerID:  textEncoderPeer.ID,
		IsLocal: textEncoderPeer.ID == dp.localPeerID,
	}

	// VAE decoder goes to third peer (or wraps around)
	vaeDecoderPeer := sorted[0].peer
	if len(sorted) > 2 {
		vaeDecoderPeer = sorted[2].peer
	} else if len(sorted) > 1 {
		vaeDecoderPeer = sorted[1].peer
	}
	assignments[2] = StageAssignment{
		Stage:   StageVAEDecoder,
		PeerID:  vaeDecoderPeer.ID,
		IsLocal: vaeDecoderPeer.ID == dp.localPeerID,
	}

	return assignments
}

// ExecuteDistributedImageGeneration runs the diffusion pipeline across peers.
// Stage 1: TextEncoder encodes the prompt
// Stage 2: UNet performs denoising steps
// Stage 3: VAE Decoder produces the final image
func (dp *DiffusionPipelineService) ExecuteDistributedImageGeneration(
	ctx context.Context,
	req models.ImageRequest,
	requestID string,
) ([]byte, error) {
	room := dp.roomSvc.CurrentRoom()
	if room == nil {
		return nil, models.ErrNotInRoom
	}

	stageAssignments := dp.AssignStages(room.Peers)
	if len(stageAssignments) == 0 {
		return nil, fmt.Errorf("no stage assignments available")
	}

	logger.Info("starting distributed image generation",
		"request_id", requestID,
		"stages", len(stageAssignments),
		"prompt_length", len(req.Prompt),
	)

	totalStart := time.Now()

	// For now, route the entire request to the UNet peer (most capable)
	// Full stage-by-stage pipeline would require tensor serialization between stages
	unetAssignment := stageAssignments[1]

	width, height := parseSize(req.Size)
	grpcReq := &workerpb.ImageRequest{
		RequestId:     requestID,
		Model:         req.Model,
		Prompt:        req.Prompt,
		Width:         int32(width),
		Height:        int32(height),
		Steps:         30,
		GuidanceScale: 7.5,
	}

	var imageData []byte

	if unetAssignment.IsLocal {
		client := dp.localWorker()
		if client == nil {
			return nil, models.ErrWorkerUnavail
		}

		resp, err := client.ImageGeneration(ctx, grpcReq)
		if err != nil {
			return nil, fmt.Errorf("local image generation failed: %w", err)
		}
		imageData = resp.ImageData
	} else {
		// In a full implementation, this would use the peer gRPC to forward
		// the image generation request. For now, fall back to local worker.
		client := dp.localWorker()
		if client == nil {
			return nil, models.ErrWorkerUnavail
		}

		resp, err := client.ImageGeneration(ctx, grpcReq)
		if err != nil {
			return nil, fmt.Errorf("image generation failed: %w", err)
		}
		imageData = resp.ImageData
	}

	logger.Info("distributed image generation completed",
		"request_id", requestID,
		"duration_ms", time.Since(totalStart).Milliseconds(),
		"image_size", len(imageData),
	)

	return imageData, nil
}
