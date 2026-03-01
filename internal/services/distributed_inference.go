package services

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/joaopedro/hivemind/gen/workerpb"
	"github.com/joaopedro/hivemind/internal/infra"
	"github.com/joaopedro/hivemind/internal/logger"
	"github.com/joaopedro/hivemind/internal/models"
)

// DistributedInferenceService coordinates inference across multiple peers.
// It determines the forward pass order based on layer assignments and
// chains the tensor transfers through the peer network.
type DistributedInferenceService struct {
	localWorker  func() workerpb.WorkerServiceClient
	peerRegistry *infra.PeerRegistry
	roomSvc      RoomService
	localPeerID  string
	compressor   *infra.TensorCompressor

	// Metrics (atomics for lock-free reads)
	tensorTransfers  atomic.Int64
	bytesTransferred atomic.Int64
	totalForwardMs   atomic.Int64
	forwardPassCount atomic.Int64

	// Generation loop metrics
	tokensGenerated    atomic.Int64
	generationRequests atomic.Int64
	embedTotalMs       atomic.Int64
	embedCount         atomic.Int64
	sampleTotalMs      atomic.Int64
	sampleCount        atomic.Int64

	// Token latency tracking
	tokenLatencyMu   sync.Mutex
	tokenLatencies   []int64
	tokenLatencySum  int64

	// Compression ratio tracking
	compMu              sync.Mutex
	compressedTotal     int64
	uncompressedTotal   int64
}

// HasRemotePeers returns true if there are remote peers connected in the registry.
func (d *DistributedInferenceService) HasRemotePeers() bool {
	return d.peerRegistry != nil && d.peerRegistry.PeerCount() > 0
}

// NewDistributedInferenceService creates a distributed inference coordinator.
func NewDistributedInferenceService(
	workerClient func() workerpb.WorkerServiceClient,
	peerRegistry *infra.PeerRegistry,
	roomSvc RoomService,
	localPeerID string,
) *DistributedInferenceService {
	compressor, _ := infra.NewTensorCompressor()

	return &DistributedInferenceService{
		localWorker:  workerClient,
		peerRegistry: peerRegistry,
		roomSvc:      roomSvc,
		localPeerID:  localPeerID,
		compressor:   compressor,
	}
}

// LayerAssignment describes which peer handles which layers.
type LayerAssignment struct {
	PeerID     string
	StartLayer int
	EndLayer   int
	IsLocal    bool
}

// ComputeForwardOrder determines the order of peers for the forward pass
// based on their layer assignments. Returns an ordered list of assignments.
func (d *DistributedInferenceService) ComputeForwardOrder() []LayerAssignment {
	room := d.roomSvc.CurrentRoom()
	if room == nil {
		return nil
	}

	var assignments []LayerAssignment

	for _, peer := range room.Peers {
		if len(peer.Layers) == 0 {
			continue
		}

		start := peer.Layers[0]
		end := peer.Layers[len(peer.Layers)-1]

		assignments = append(assignments, LayerAssignment{
			PeerID:     peer.ID,
			StartLayer: start,
			EndLayer:   end,
			IsLocal:    peer.ID == d.localPeerID,
		})
	}

	// Sort by starting layer for correct forward pass order
	sort.Slice(assignments, func(i, j int) bool {
		return assignments[i].StartLayer < assignments[j].StartLayer
	})

	return assignments
}

// ExecuteDistributedForwardPass chains the forward pass through all peers in order.
// Each peer processes its assigned layers and passes the intermediate tensor to the next.
// Remote transfers use zstd compression and SHA-256 checksums for integrity.
// When useCache is true, peers reuse KV cache from previous steps (avoids O(n²) recomputation).
func (d *DistributedInferenceService) ExecuteDistributedForwardPass(
	ctx context.Context,
	tensorData []byte,
	requestID string,
	useCache bool,
	cacheSeqLen int32,
) ([]byte, float64, int32, error) {
	order := d.ComputeForwardOrder()
	if len(order) == 0 {
		return nil, 0, 0, fmt.Errorf("no layer assignments found")
	}

	logger.Info("starting distributed forward pass",
		"request_id", requestID,
		"peer_count", len(order),
		"input_size", len(tensorData),
		"use_cache", useCache,
		"cache_seq_len", cacheSeqLen,
	)

	totalDuration := float64(0)
	currentData := tensorData
	currentSeqLen := cacheSeqLen

	for i, assignment := range order {
		start := time.Now()

		// Compute checksum for integrity
		checksum := sha256.Sum256(currentData)

		// Compress for remote transfers
		sendData := currentData
		compressed := false
		if !assignment.IsLocal && d.compressor != nil {
			var ratio float64
			sendData, compressed, ratio = d.compressor.CompressIfBeneficial(currentData)
			if compressed {
				d.trackCompression(len(currentData), len(sendData))
				logger.Info("tensor compressed for transfer",
					"original_size", len(currentData),
					"compressed_size", len(sendData),
					"ratio", fmt.Sprintf("%.2f", ratio),
				)
			}
		}

		req := &workerpb.ForwardRequest{
			RequestId:  requestID,
			TensorData: sendData,
			Meta: &workerpb.TensorMeta{
				FromLayer: int32(assignment.StartLayer),
				ToLayer:   int32(assignment.EndLayer),
			},
			Compressed:  compressed,
			Checksum:    checksum[:],
			UseCache:    useCache,
			CacheSeqLen: currentSeqLen,
		}

		var resp *workerpb.ForwardResponse
		var err error

		if assignment.IsLocal {
			// Process locally via worker gRPC
			client := d.localWorker()
			if client == nil {
				return nil, 0, 0, models.ErrWorkerUnavail
			}

			resp, err = client.ForwardPass(ctx, req)
		} else {
			// Forward to remote peer
			resp, err = d.peerRegistry.ForwardToNextPeer(ctx, assignment.PeerID, req)

			// Track transfer metrics
			d.tensorTransfers.Add(1)
			d.bytesTransferred.Add(int64(len(sendData)))
		}

		if err != nil {
			return nil, 0, 0, fmt.Errorf("forward pass failed at peer %s (layers %d-%d): %w",
				assignment.PeerID, assignment.StartLayer, assignment.EndLayer, err)
		}

		duration := float64(time.Since(start).Milliseconds())
		totalDuration += duration
		d.totalForwardMs.Add(int64(duration))
		d.forwardPassCount.Add(1)

		// Update sequence length from response
		if resp.CacheSeqLen > 0 {
			currentSeqLen = resp.CacheSeqLen
		}

		// Decompress response if needed
		outputData := resp.TensorData
		if resp.Compressed && d.compressor != nil {
			outputData, err = d.compressor.Decompress(resp.TensorData)
			if err != nil {
				return nil, 0, 0, fmt.Errorf("failed to decompress response from peer %s: %w",
					assignment.PeerID, err)
			}
		}

		logger.Info("forward pass step completed",
			"step", i+1,
			"total_steps", len(order),
			"peer_id", assignment.PeerID,
			"layers", fmt.Sprintf("%d-%d", assignment.StartLayer, assignment.EndLayer),
			"duration_ms", duration,
			"output_size", len(outputData),
		)

		currentData = outputData
	}

	logger.Info("distributed forward pass completed",
		"request_id", requestID,
		"total_duration_ms", totalDuration,
		"output_size", len(currentData),
		"cache_seq_len", currentSeqLen,
	)

	return currentData, totalDuration, currentSeqLen, nil
}

// GetStats returns current distributed inference statistics.
func (d *DistributedInferenceService) GetStats() *models.DistributedStats {
	room := d.roomSvc.CurrentRoom()
	if room == nil {
		return nil
	}

	peerCount := len(room.Peers)
	isDistributed := peerCount > 1

	// Average latency from peer registry
	avgLatency := float64(0)
	if d.peerRegistry != nil {
		peers := d.peerRegistry.GetAllPeers()
		totalLatency := float64(0)
		validPeers := 0
		for _, p := range peers {
			if p.Latency > 0 {
				totalLatency += p.Latency
				validPeers++
			}
		}
		if validPeers > 0 {
			avgLatency = totalLatency / float64(validPeers)
		}
	}

	// Average forward pass duration
	forwardPassAvg := float64(0)
	passCount := d.forwardPassCount.Load()
	if passCount > 0 {
		forwardPassAvg = float64(d.totalForwardMs.Load()) / float64(passCount)
	}

	// Compression ratio
	compressionRatio := 1.0
	d.compMu.Lock()
	if d.uncompressedTotal > 0 {
		compressionRatio = float64(d.compressedTotal) / float64(d.uncompressedTotal)
	}
	d.compMu.Unlock()

	// Generation metrics
	tokensGen := d.tokensGenerated.Load()
	genRequests := d.generationRequests.Load()

	// Tokens per second
	tokensPerSec := float64(0)
	avgTokenLatency := float64(0)
	d.tokenLatencyMu.Lock()
	if len(d.tokenLatencies) > 0 && d.tokenLatencySum > 0 {
		avgTokenLatency = float64(d.tokenLatencySum) / float64(len(d.tokenLatencies))
		if avgTokenLatency > 0 {
			tokensPerSec = 1000.0 / avgTokenLatency
		}
	}
	d.tokenLatencyMu.Unlock()

	// Embed and sample averages
	embedAvg := float64(0)
	if ec := d.embedCount.Load(); ec > 0 {
		embedAvg = float64(d.embedTotalMs.Load()) / float64(ec)
	}
	sampleAvg := float64(0)
	if sc := d.sampleCount.Load(); sc > 0 {
		sampleAvg = float64(d.sampleTotalMs.Load()) / float64(sc)
	}

	return &models.DistributedStats{
		PeerCount:          peerCount,
		TotalLayers:        room.TotalLayers,
		AvgLatencyMs:       avgLatency,
		TensorTransfers:    d.tensorTransfers.Load(),
		BytesTransferred:   d.bytesTransferred.Load(),
		CompressionRatio:   compressionRatio,
		ForwardPassAvgMs:   forwardPassAvg,
		IsDistributed:      isDistributed,
		TokensGenerated:    tokensGen,
		TokensPerSecond:    tokensPerSec,
		AvgTokenLatencyMs:  avgTokenLatency,
		EmbedAvgMs:         embedAvg,
		SampleAvgMs:        sampleAvg,
		GenerationRequests: genRequests,
	}
}

// recordTokenLatency tracks individual token generation latencies (ring buffer of last 1000).
func (d *DistributedInferenceService) recordTokenLatency(ms int64) {
	d.tokenLatencyMu.Lock()
	defer d.tokenLatencyMu.Unlock()

	const maxLatencies = 1000
	d.tokenLatencies = append(d.tokenLatencies, ms)
	d.tokenLatencySum += ms

	if len(d.tokenLatencies) > maxLatencies {
		// Remove oldest entry
		d.tokenLatencySum -= d.tokenLatencies[0]
		d.tokenLatencies = d.tokenLatencies[1:]
	}
}

func (d *DistributedInferenceService) trackCompression(original, compressed int) {
	d.compMu.Lock()
	d.uncompressedTotal += int64(original)
	d.compressedTotal += int64(compressed)
	d.compMu.Unlock()
}

// Close releases resources held by the distributed inference service.
func (d *DistributedInferenceService) Close() {
	if d.compressor != nil {
		d.compressor.Close()
	}
}

// AssignLayersByVRAM distributes model layers across peers proportional to their available VRAM.
// This is the core algorithm that determines how a model is split across the network.
func AssignLayersByVRAM(peers []models.Peer, totalLayers int) map[string][]int {
	if len(peers) == 0 || totalLayers == 0 {
		return nil
	}

	// Calculate total usable VRAM across all peers
	totalVRAM := int64(0)
	for _, p := range peers {
		totalVRAM += p.Resources.TotalUsableVRAM()
	}

	if totalVRAM == 0 {
		// Equal distribution fallback
		return assignLayersEqual(peers, totalLayers)
	}

	assignments := make(map[string][]int)
	currentLayer := 0

	for i, p := range peers {
		var layerCount int
		if i == len(peers)-1 {
			// Last peer gets remaining layers
			layerCount = totalLayers - currentLayer
		} else {
			// Proportional allocation
			proportion := float64(p.Resources.TotalUsableVRAM()) / float64(totalVRAM)
			layerCount = int(proportion * float64(totalLayers))
			if layerCount == 0 {
				layerCount = 1 // At least one layer
			}
		}

		layers := make([]int, layerCount)
		for j := range layerCount {
			layers[j] = currentLayer + j
		}

		assignments[p.ID] = layers
		currentLayer += layerCount
	}

	return assignments
}

func assignLayersEqual(peers []models.Peer, totalLayers int) map[string][]int {
	assignments := make(map[string][]int)
	layersPerPeer := totalLayers / len(peers)
	remainder := totalLayers % len(peers)
	currentLayer := 0

	for i, p := range peers {
		count := layersPerPeer
		if i < remainder {
			count++
		}

		layers := make([]int, count)
		for j := range count {
			layers[j] = currentLayer + j
		}

		assignments[p.ID] = layers
		currentLayer += count
	}

	return assignments
}
